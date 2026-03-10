package extproc

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"
	"time"

	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	extprocv3 "github.com/envoyproxy/go-control-plane/envoy/service/ext_proc/v3"
	typev3 "github.com/envoyproxy/go-control-plane/envoy/type/v3"
	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/agentgateway/budget-management/internal/budget"
	"github.com/agentgateway/budget-management/internal/cel"
	"github.com/agentgateway/budget-management/internal/config"
	"github.com/agentgateway/budget-management/internal/metrics"
)

// Server implements the ext_proc service for AgentGateway.
type Server struct {
	extprocv3.UnimplementedExternalProcessorServer
	budgetSvc    *budget.Service
	celEvaluator *cel.Evaluator
	cfg          *config.Config

	// Track request state across the stream
	requestStates sync.Map // map[string]*requestState
}

// requestState holds state for a single request.
type requestState struct {
	RequestID       string
	ModelID         string
	MatchedBudgets  []budgetInfo
	RateLimitedAt   *uuid.UUID
	EstimatedCost   float64
	StartTime       time.Time
	StreamingBuffer strings.Builder
	EvalContext     *cel.EvalContext // Stored for deferred budget check
	BudgetChecked   bool             // Whether budget check has been performed
	RequestBody     []byte           // Buffer request body for model extraction
}

// budgetInfo stores budget details for metrics recording.
type budgetInfo struct {
	EntityType string
	Name       string
	Period     string
}

// NewServer creates a new ext_proc server.
func NewServer(budgetSvc *budget.Service, celEvaluator *cel.Evaluator, cfg *config.Config) *Server {
	return &Server{
		budgetSvc:    budgetSvc,
		celEvaluator: celEvaluator,
		cfg:          cfg,
	}
}

// Register registers the server with a gRPC server.
func (s *Server) Register(grpcServer *grpc.Server) {
	extprocv3.RegisterExternalProcessorServer(grpcServer, s)
}

// Process handles the bidirectional stream from AgentGateway.
func (s *Server) Process(stream extprocv3.ExternalProcessor_ProcessServer) error {
	log.Debug().Msg("ext-proc: new connection from agentgateway")
	ctx := stream.Context()

	// Track state for this stream
	var requestID string
	var state *requestState

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		req, err := stream.Recv()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return status.Errorf(codes.Internal, "failed to receive request: %v", err)
		}

		var resp *extprocv3.ProcessingResponse

		switch r := req.Request.(type) {
		case *extprocv3.ProcessingRequest_RequestHeaders:
			log.Debug().Msg("ext-proc: received request headers")

			// Extract or generate request ID
			requestID = s.getRequestID(r.RequestHeaders)
			if requestID == "" {
				requestID = uuid.New().String()
			}

			// Build CEL evaluation context from headers (store for deferred budget check)
			evalCtx := s.buildEvalContext(r.RequestHeaders, req)

			// Extract model from headers (may get from body later)
			// TODO(metadataContext): get model from req.MetadataContext once supported by kgateway CRDs
			modelID := s.extractModelFromHeaders(r.RequestHeaders)

			// Store request state - defer budget check until we have the model from body
			state = &requestState{
				RequestID:     requestID,
				ModelID:       modelID,
				StartTime:     time.Now(),
				EvalContext:   evalCtx,
				BudgetChecked: false,
			}
			s.requestStates.Store(requestID, state)

			// If we already have a model from headers, do budget check now
			if modelID != "" {
				resp = s.doBudgetCheckAndRespond(ctx, state, true)
			} else {
				// Defer budget check - continue to body phase to extract model
				log.Debug().Str("request_id", requestID).Msg("deferring budget check to body phase")
				resp = &extprocv3.ProcessingResponse{
					Response: &extprocv3.ProcessingResponse_RequestHeaders{
						RequestHeaders: &extprocv3.HeadersResponse{
							Response: &extprocv3.CommonResponse{
								Status: extprocv3.CommonResponse_CONTINUE,
								HeaderMutation: &extprocv3.HeaderMutation{
									SetHeaders: []*corev3.HeaderValueOption{
										{
											Header: &corev3.HeaderValue{
												Key:      "x-budget-request-id",
												RawValue: []byte(requestID),
											},
										},
									},
								},
							},
						},
					},
				}
			}

		case *extprocv3.ProcessingRequest_RequestBody:
			log.Debug().Bool("end_of_stream", r.RequestBody.EndOfStream).Int("body_len", len(r.RequestBody.Body)).Msg("ext-proc: received request body")

			// Buffer body chunks for model extraction
			if state != nil {
				state.RequestBody = append(state.RequestBody, r.RequestBody.Body...)
			}

			// On end of stream, extract model and do deferred budget check
			// TODO(metadataContext): remove body buffering once model available in headers via metadata
			if r.RequestBody.EndOfStream && state != nil && !state.BudgetChecked {
				// Extract model from buffered body
				if state.ModelID == "" && len(state.RequestBody) > 0 {
					state.ModelID = s.extractModelFromBody(state.RequestBody)
					log.Debug().Str("request_id", requestID).Str("model", state.ModelID).Msg("extracted model from body")
				}

				// Now do the budget check with the model
				budgetResp := s.doBudgetCheckAndRespond(ctx, state, false)
				if budgetResp != nil {
					// Budget check failed - return immediate response
					resp = budgetResp
					break
				}
			}

			// Use StreamedResponse to forward body (Solo.io pattern)
			resp = &extprocv3.ProcessingResponse{
				Response: &extprocv3.ProcessingResponse_RequestBody{
					RequestBody: &extprocv3.BodyResponse{
						Response: &extprocv3.CommonResponse{
							BodyMutation: &extprocv3.BodyMutation{
								Mutation: &extprocv3.BodyMutation_StreamedResponse{
									StreamedResponse: &extprocv3.StreamedBodyResponse{
										Body:        r.RequestBody.Body,
										EndOfStream: r.RequestBody.EndOfStream,
									},
								},
							},
						},
					},
				},
			}

		case *extprocv3.ProcessingRequest_ResponseHeaders:
			log.Debug().Msg("ext-proc: received response headers")
			resp = &extprocv3.ProcessingResponse{
				Response: &extprocv3.ProcessingResponse_ResponseHeaders{
					ResponseHeaders: &extprocv3.HeadersResponse{
						Response: &extprocv3.CommonResponse{
							Status: extprocv3.CommonResponse_CONTINUE,
						},
					},
				},
			}

		case *extprocv3.ProcessingRequest_ResponseBody:
			log.Debug().Bool("end_of_stream", r.ResponseBody.EndOfStream).Int("body_len", len(r.ResponseBody.Body)).Msg("ext-proc: received response body")

			// Buffer response body for cost calculation
			if state != nil {
				state.StreamingBuffer.Write(r.ResponseBody.Body)
			}

			// On end of stream, calculate costs and add headers
			var headerMutation *extprocv3.HeaderMutation
			if r.ResponseBody.EndOfStream && state != nil {
				responseBody := []byte(state.StreamingBuffer.String())

				// Parse token usage from response
				inputTokens, outputTokens := s.parseTokenUsage(responseBody)

				// Extract model from response if not already set
				if state.ModelID == "" {
					state.ModelID = s.extractModelFromResponse(responseBody)
				}

				// Decrement budgets
				result, err := s.budgetSvc.DecrementBudgets(ctx, requestID, state.ModelID, inputTokens, outputTokens, state.RateLimitedAt)
				if err != nil {
					log.Error().Err(err).Str("request_id", requestID).Msg("failed to decrement budgets")
				}

				// Record cost and token metrics for each matched budget
				if result != nil && result.ActualCost > 0 {
					for _, b := range state.MatchedBudgets {
						metrics.RecordCostCharged(b.EntityType, b.Name, state.ModelID, result.ActualCost)
						metrics.RecordTokens(b.EntityType, b.Name, state.ModelID, inputTokens, outputTokens)
					}
					metrics.RecordExtProc("response_body", "success", time.Since(state.StartTime))
				}

				// Build response headers with cost info
				if result != nil {
					headerMutation = &extprocv3.HeaderMutation{
						SetHeaders: []*corev3.HeaderValueOption{
							{
								Header: &corev3.HeaderValue{
									Key:      "x-budget-cost-usd",
									RawValue: []byte(fmt.Sprintf("%.6f", result.ActualCost)),
								},
							},
							{
								Header: &corev3.HeaderValue{
									Key:      "x-budget-remaining-usd",
									RawValue: []byte(fmt.Sprintf("%.6f", result.RemainingBudget)),
								},
							},
						},
					}
				}

				// Clean up state
				s.requestStates.Delete(requestID)
			}

			// Use StreamedResponse to forward body (Solo.io pattern)
			resp = &extprocv3.ProcessingResponse{
				Response: &extprocv3.ProcessingResponse_ResponseBody{
					ResponseBody: &extprocv3.BodyResponse{
						Response: &extprocv3.CommonResponse{
							HeaderMutation: headerMutation,
							BodyMutation: &extprocv3.BodyMutation{
								Mutation: &extprocv3.BodyMutation_StreamedResponse{
									StreamedResponse: &extprocv3.StreamedBodyResponse{
										Body:        r.ResponseBody.Body,
										EndOfStream: r.ResponseBody.EndOfStream,
									},
								},
							},
						},
					},
				},
			}

		default:
			log.Warn().Str("type", fmt.Sprintf("%T", req.Request)).Msg("ext-proc: unknown request type")
			continue
		}

		log.Debug().Msg("ext-proc: sending response")
		if err := stream.Send(resp); err != nil {
			log.Error().Err(err).Msg("ext-proc: failed to send response")
			return status.Errorf(codes.Internal, "failed to send response: %v", err)
		}
	}
}

// doBudgetCheckAndRespond performs the budget check and returns an immediate response if denied.
// If isHeadersPhase is true, it returns a headers response; otherwise returns an immediate response for body phase.
// Returns nil if the budget check passed and the request should continue.
func (s *Server) doBudgetCheckAndRespond(ctx context.Context, state *requestState, isHeadersPhase bool) *extprocv3.ProcessingResponse {
	if state == nil || state.EvalContext == nil {
		return nil
	}

	checkStart := time.Now()
	result, err := s.budgetSvc.CheckBudget(ctx, state.EvalContext, state.ModelID)
	checkDuration := time.Since(checkStart)

	if err != nil {
		log.Error().Err(err).Str("request_id", state.RequestID).Msg("failed to check budget")
		return s.immediateResponse(429, "Budget check failed", 0)
	}

	state.BudgetChecked = true

	if !result.Allowed {
		// Record request-level metric (once per request)
		metrics.RecordBudgetRequest(false)
		// Record per-budget metrics
		for _, b := range result.MatchedBudgets {
			metrics.RecordBudgetCheck(string(b.EntityType), b.Name, false, checkDuration)
			metrics.RecordRateLimited(string(b.EntityType), b.Name)
			metrics.UpdateBudgetUsage(string(b.EntityType), b.Name, string(b.Period),
				b.CurrentUsageUSD, b.CalculateRemaining(), b.BudgetAmountUSD)
		}
		// Return 429 Too Many Requests
		retryAfter := int(result.RetryAfter.Seconds())
		if retryAfter < 1 {
			retryAfter = 3600
		}
		log.Info().Str("request_id", state.RequestID).Str("model", state.ModelID).Float64("estimated_cost", result.EstimatedCost).Float64("remaining", result.RemainingBudget).Msg("request rate limited")
		return s.immediateResponse(429, "Budget exceeded", retryAfter)
	}

	// Budget check passed
	metrics.RecordBudgetRequest(true)
	for _, b := range result.MatchedBudgets {
		metrics.RecordBudgetCheck(string(b.EntityType), b.Name, true, checkDuration)
		metrics.UpdateBudgetUsage(string(b.EntityType), b.Name, string(b.Period),
			b.CurrentUsageUSD, b.CalculateRemaining(), b.BudgetAmountUSD)
	}

	// Update state with matched budgets for later metrics
	state.MatchedBudgets = make([]budgetInfo, 0, len(result.MatchedBudgets))
	for _, b := range result.MatchedBudgets {
		state.MatchedBudgets = append(state.MatchedBudgets, budgetInfo{
			EntityType: string(b.EntityType),
			Name:       b.Name,
			Period:     string(b.Period),
		})
	}
	state.RateLimitedAt = result.RateLimitedAt
	state.EstimatedCost = result.EstimatedCost

	// Create reservation if we have matching budgets
	if len(result.MatchedBudgets) > 0 {
		if err := s.budgetSvc.CreateReservation(ctx, state.RequestID, result.MatchedBudgets, result.EstimatedCost); err != nil {
			log.Warn().Err(err).Str("request_id", state.RequestID).Msg("failed to create reservation")
		}
	}

	// If this was called during headers phase and passed, return the continue response
	if isHeadersPhase {
		return &extprocv3.ProcessingResponse{
			Response: &extprocv3.ProcessingResponse_RequestHeaders{
				RequestHeaders: &extprocv3.HeadersResponse{
					Response: &extprocv3.CommonResponse{
						Status: extprocv3.CommonResponse_CONTINUE,
						HeaderMutation: &extprocv3.HeaderMutation{
							SetHeaders: []*corev3.HeaderValueOption{
								{
									Header: &corev3.HeaderValue{
										Key:      "x-budget-request-id",
										RawValue: []byte(state.RequestID),
									},
								},
							},
						},
					},
				},
			},
		}
	}

	// Budget passed during body phase - return nil to continue normal processing
	return nil
}

// handleRequestHeaders processes incoming request headers.
func (s *Server) handleRequestHeaders(ctx context.Context, headers *extprocv3.HttpHeaders, req *extprocv3.ProcessingRequest) (*extprocv3.ProcessingResponse, error) {
	// Extract request ID or generate one
	requestID := s.getRequestID(headers)
	if requestID == "" {
		requestID = uuid.New().String()
	}

	log.Debug().Str("request_id", requestID).Msg("ext-proc: processing request headers")

	// Store minimal request state for tracking
	state := &requestState{
		RequestID: requestID,
		ModelID:   s.extractModelFromHeaders(headers),
		StartTime: time.Now(),
	}
	s.requestStates.Store(requestID, state)

	// Do budget check asynchronously to avoid blocking
	go func() {
		evalCtx := s.buildEvalContext(headers, req)
		result, err := s.budgetSvc.CheckBudget(context.Background(), evalCtx, state.ModelID)
		if err != nil {
			log.Error().Err(err).Str("request_id", requestID).Msg("failed to check budget")
			return
		}

		// Update state with budget info
		if st, ok := s.requestStates.Load(requestID); ok {
			rs := st.(*requestState)
			rs.RateLimitedAt = result.RateLimitedAt
			rs.EstimatedCost = result.EstimatedCost
		}

		// Create reservation if we have matching budgets
		if len(result.MatchedBudgets) > 0 {
			if err := s.budgetSvc.CreateReservation(context.Background(), requestID, result.MatchedBudgets, result.EstimatedCost); err != nil {
				log.Warn().Err(err).Str("request_id", requestID).Msg("failed to create reservation")
			}
		}
	}()

	// Return immediately with CONTINUE
	return &extprocv3.ProcessingResponse{
		Response: &extprocv3.ProcessingResponse_RequestHeaders{
			RequestHeaders: &extprocv3.HeadersResponse{
				Response: &extprocv3.CommonResponse{
					Status: extprocv3.CommonResponse_CONTINUE,
					HeaderMutation: &extprocv3.HeaderMutation{
						SetHeaders: []*corev3.HeaderValueOption{
							{
								Header: &corev3.HeaderValue{
									Key:      "x-budget-request-id",
									RawValue: []byte(requestID),
								},
							},
						},
					},
				},
			},
		},
	}, nil
}

// handleRequestBody processes request body.
func (s *Server) handleRequestBody(ctx context.Context, body *extprocv3.HttpBody, req *extprocv3.ProcessingRequest) (*extprocv3.ProcessingResponse, error) {
	// Parse the body to extract model if not already set
	requestID := s.getRequestIDFromContext(req)

	if state, ok := s.requestStates.Load(requestID); ok {
		rs := state.(*requestState)
		if rs.ModelID == "" && body.EndOfStream {
			// Try to extract model from body only on final chunk
			rs.ModelID = s.extractModelFromBody(body.Body)
		}
	}

	// Just CONTINUE to pass body through unchanged
	return &extprocv3.ProcessingResponse{
		Response: &extprocv3.ProcessingResponse_RequestBody{
			RequestBody: &extprocv3.BodyResponse{
				Response: &extprocv3.CommonResponse{
					Status: extprocv3.CommonResponse_CONTINUE,
				},
			},
		},
	}, nil
}

// handleResponseHeaders processes response headers.
func (s *Server) handleResponseHeaders(ctx context.Context, headers *extprocv3.HttpHeaders, req *extprocv3.ProcessingRequest) (*extprocv3.ProcessingResponse, error) {
	return &extprocv3.ProcessingResponse{
		Response: &extprocv3.ProcessingResponse_ResponseHeaders{
			ResponseHeaders: &extprocv3.HeadersResponse{
				Response: &extprocv3.CommonResponse{
					Status: extprocv3.CommonResponse_CONTINUE,
				},
			},
		},
	}, nil
}

// handleResponseBody processes response body.
func (s *Server) handleResponseBody(ctx context.Context, body *extprocv3.HttpBody, req *extprocv3.ProcessingRequest) (*extprocv3.ProcessingResponse, error) {
	requestID := s.getRequestIDFromContext(req)

	// Check if this is end of stream
	if !body.EndOfStream {
		// Buffer streaming response
		if state, ok := s.requestStates.Load(requestID); ok {
			rs := state.(*requestState)
			rs.StreamingBuffer.Write(body.Body)
		}

		return &extprocv3.ProcessingResponse{
			Response: &extprocv3.ProcessingResponse_ResponseBody{
				ResponseBody: &extprocv3.BodyResponse{
					Response: &extprocv3.CommonResponse{
						Status: extprocv3.CommonResponse_CONTINUE,
					},
				},
			},
		}, nil
	}

	// End of stream - process the complete response
	var responseBody []byte
	if state, ok := s.requestStates.Load(requestID); ok {
		rs := state.(*requestState)
		rs.StreamingBuffer.Write(body.Body)
		responseBody = []byte(rs.StreamingBuffer.String())
	} else {
		responseBody = body.Body
	}

	// Parse token usage from response
	inputTokens, outputTokens := s.parseTokenUsage(responseBody)

	// Get request state
	var modelID string
	var rateLimitedAt *uuid.UUID

	if state, ok := s.requestStates.Load(requestID); ok {
		rs := state.(*requestState)
		modelID = rs.ModelID
		rateLimitedAt = rs.RateLimitedAt
		s.requestStates.Delete(requestID)
	}

	// Extract model from response if not already set (most LLM providers include it)
	if modelID == "" {
		modelID = s.extractModelFromResponse(responseBody)
	}

	// Decrement budgets
	result, err := s.budgetSvc.DecrementBudgets(ctx, requestID, modelID, inputTokens, outputTokens, rateLimitedAt)
	if err != nil {
		log.Error().Err(err).Str("request_id", requestID).Msg("failed to decrement budgets")
	}

	// Build response headers
	var setHeaders []*corev3.HeaderValueOption

	if result != nil {
		setHeaders = append(setHeaders,
			&corev3.HeaderValueOption{
				Header: &corev3.HeaderValue{
					Key:      "x-budget-cost-usd",
					RawValue: []byte(fmt.Sprintf("%.6f", result.ActualCost)),
				},
			},
			&corev3.HeaderValueOption{
				Header: &corev3.HeaderValue{
					Key:      "x-budget-remaining-usd",
					RawValue: []byte(fmt.Sprintf("%.6f", result.RemainingBudget)),
				},
			},
		)
	}

	return &extprocv3.ProcessingResponse{
		Response: &extprocv3.ProcessingResponse_ResponseBody{
			ResponseBody: &extprocv3.BodyResponse{
				Response: &extprocv3.CommonResponse{
					Status: extprocv3.CommonResponse_CONTINUE,
					HeaderMutation: &extprocv3.HeaderMutation{
						SetHeaders: setHeaders,
					},
				},
			},
		},
	}, nil
}

// buildEvalContext builds a CEL evaluation context from request information.
func (s *Server) buildEvalContext(headers *extprocv3.HttpHeaders, req *extprocv3.ProcessingRequest) *cel.EvalContext {
	ctx := &cel.EvalContext{
		Request: cel.RequestContext{
			Headers: make(map[string]string),
		},
		JWT: cel.JWTContext{
			Claims: make(map[string]interface{}),
		},
		APIKey: cel.APIKeyContext{
			Metadata: make(map[string]interface{}),
		},
		LLM:      cel.LLMContext{},
		Source:   cel.SourceContext{},
		Metadata: make(map[string]interface{}),
	}

	// Extract headers
	if headers != nil && headers.Headers != nil {
		for _, h := range headers.Headers.Headers {
			key := strings.ToLower(h.Key)
			value := getHeaderValue(h)
			ctx.Request.Headers[key] = value

			switch key {
			case ":path":
				ctx.Request.Path = value
			case ":method":
				ctx.Request.Method = value
			case ":authority", "host":
				ctx.Request.Host = value
			case s.cfg.OrgIDHeader:
				ctx.Metadata["org_id"] = value
			case s.cfg.TeamIDHeader:
				ctx.Metadata["team_id"] = value
			case s.cfg.UserIDHeader:
				ctx.Metadata["user_id"] = value
			}
		}
	}

	// Extract metadata from processing request
	// TODO(metadataContext): extract model ID from metadata once kgateway CRDs support it
	if req.MetadataContext != nil && req.MetadataContext.FilterMetadata != nil {
		for ns, md := range req.MetadataContext.FilterMetadata {
			if md.Fields != nil {
				for k, v := range md.Fields {
					ctx.Metadata[ns+"."+k] = v.AsInterface()
				}
			}
		}
	}

	return ctx
}

// getRequestID extracts request ID from headers.
func (s *Server) getRequestID(headers *extprocv3.HttpHeaders) string {
	if headers == nil || headers.Headers == nil {
		return ""
	}

	for _, h := range headers.Headers.Headers {
		key := strings.ToLower(h.Key)
		if key == "x-request-id" || key == "x-budget-request-id" {
			return getHeaderValue(h)
		}
	}

	return ""
}

// getRequestIDFromContext extracts request ID from processing request context.
func (s *Server) getRequestIDFromContext(req *extprocv3.ProcessingRequest) string {
	// Try to get from attributes
	if req.Attributes != nil {
		for k, v := range req.Attributes {
			if strings.HasSuffix(k, "request_id") {
				if v.Fields != nil {
					for _, f := range v.Fields {
						if str, ok := f.AsInterface().(string); ok {
							return str
						}
					}
				}
			}
		}
	}

	// Generate a new one if not found
	return uuid.New().String()
}

// getHeaderValue extracts the value from a header, preferring RawValue over Value.
// This matches the agentgateway enterprise ext_proc API behavior.
func getHeaderValue(h *corev3.HeaderValue) string {
	if len(h.RawValue) > 0 {
		return string(h.RawValue)
	}
	return h.Value
}

// extractModelFromHeaders tries to extract the model from request headers.
func (s *Server) extractModelFromHeaders(headers *extprocv3.HttpHeaders) string {
	if headers == nil || headers.Headers == nil {
		return ""
	}

	for _, h := range headers.Headers.Headers {
		key := strings.ToLower(h.Key)
		if key == "x-model" || key == "x-llm-model" {
			return getHeaderValue(h)
		}
	}

	return ""
}

// extractModelFromBody extracts the model from the request body.
func (s *Server) extractModelFromBody(body []byte) string {
	var req struct {
		Model string `json:"model"`
	}

	if err := json.Unmarshal(body, &req); err != nil {
		return ""
	}

	return req.Model
}

// extractModelFromResponse extracts the model from the LLM response.
// Most providers include the model in the response (e.g., OpenAI, Anthropic).
func (s *Server) extractModelFromResponse(body []byte) string {
	var resp struct {
		Model string `json:"model"`
	}

	if err := json.Unmarshal(body, &resp); err == nil && resp.Model != "" {
		return resp.Model
	}

	return ""
}

// parseTokenUsage parses token usage from the LLM response.
func (s *Server) parseTokenUsage(body []byte) (inputTokens, outputTokens int64) {
	// Try OpenAI format
	var openaiResp struct {
		Usage struct {
			PromptTokens     int64 `json:"prompt_tokens"`
			CompletionTokens int64 `json:"completion_tokens"`
		} `json:"usage"`
	}

	if err := json.Unmarshal(body, &openaiResp); err == nil {
		if openaiResp.Usage.PromptTokens > 0 || openaiResp.Usage.CompletionTokens > 0 {
			return openaiResp.Usage.PromptTokens, openaiResp.Usage.CompletionTokens
		}
	}

	// Try Anthropic format
	var anthropicResp struct {
		Usage struct {
			InputTokens  int64 `json:"input_tokens"`
			OutputTokens int64 `json:"output_tokens"`
		} `json:"usage"`
	}

	if err := json.Unmarshal(body, &anthropicResp); err == nil {
		if anthropicResp.Usage.InputTokens > 0 || anthropicResp.Usage.OutputTokens > 0 {
			return anthropicResp.Usage.InputTokens, anthropicResp.Usage.OutputTokens
		}
	}

	// Return defaults if parsing fails
	return 0, 0
}

// immediateResponse creates an immediate response to reject a request.
// This follows the agentgateway enterprise ext_proc API pattern.
func (s *Server) immediateResponse(statusCode int, message string, retryAfter int) *extprocv3.ProcessingResponse {
	var headers []*corev3.HeaderValueOption

	headers = append(headers, &corev3.HeaderValueOption{
		Header: &corev3.HeaderValue{
			Key:      "content-type",
			RawValue: []byte("application/json"),
		},
	})

	if retryAfter > 0 {
		headers = append(headers, &corev3.HeaderValueOption{
			Header: &corev3.HeaderValue{
				Key:      "retry-after",
				RawValue: []byte(strconv.Itoa(retryAfter)),
			},
		})
	}

	body := fmt.Sprintf(`{"error":{"message":"%s","type":"rate_limit_error","code":"budget_exceeded"}}`, message)

	return &extprocv3.ProcessingResponse{
		Response: &extprocv3.ProcessingResponse_ImmediateResponse{
			ImmediateResponse: &extprocv3.ImmediateResponse{
				Status: &typev3.HttpStatus{
					Code: typev3.StatusCode(statusCode),
				},
				Headers: &extprocv3.HeaderMutation{
					SetHeaders: headers,
				},
				Body:    []byte(body),
				Details: "budget-rate-limit: " + message,
			},
		},
	}
}
