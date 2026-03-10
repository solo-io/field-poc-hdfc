package budget

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"sync"
	"time"

	"github.com/agentgateway/budget-management/internal/cel"
	"github.com/agentgateway/budget-management/internal/config"
	"github.com/agentgateway/budget-management/internal/db"
	"github.com/agentgateway/budget-management/internal/metrics"
	"github.com/agentgateway/budget-management/internal/models"
	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
)

// modelVersionSuffixRegex matches version suffixes like -2024-07-18, -20240229, -v1, etc.
var modelVersionSuffixRegex = regexp.MustCompile(`-(\d{4}-\d{2}-\d{2}|\d{8}|v\d+)$`)

var (
	ErrBudgetExceeded   = errors.New("budget exceeded")
	ErrNoMatchingBudget = errors.New("no matching budget found")
)

// CheckResult represents the result of a budget check.
type CheckResult struct {
	Allowed          bool
	MatchedBudgets   []models.BudgetDefinition
	RateLimitedAt    *uuid.UUID // ID of the budget that caused rate limiting
	FallbackBudgetID *uuid.UUID // ID of the budget used as fallback (if fallback occurred)
	EstimatedCost    float64
	RemainingBudget  float64
	RetryAfter       time.Duration
}

// DecrementResult represents the result of a budget decrement.
type DecrementResult struct {
	ActualCost      float64
	RemainingBudget float64
	BudgetsCharged  []uuid.UUID
}

// Service provides budget management operations.
type Service struct {
	repo         *db.Repository
	celEvaluator *cel.Evaluator
	cfg          *config.Config

	// Caches
	modelCostCache *modelCostCache
	budgetCache    *budgetCache
}

// modelCostCache caches model costs.
type modelCostCache struct {
	sync.RWMutex
	costs     map[string]*models.ModelCost
	expiresAt time.Time
	ttl       time.Duration
}

// budgetCache caches budget definitions.
type budgetCache struct {
	sync.RWMutex
	budgets   []models.BudgetDefinition
	expiresAt time.Time
	ttl       time.Duration
}

// NewService creates a new budget service.
func NewService(repo *db.Repository, celEvaluator *cel.Evaluator, cfg *config.Config) *Service {
	return &Service{
		repo:         repo,
		celEvaluator: celEvaluator,
		cfg:          cfg,
		modelCostCache: &modelCostCache{
			costs: make(map[string]*models.ModelCost),
			ttl:   cfg.ModelCostCacheTTL,
		},
		budgetCache: &budgetCache{
			ttl: cfg.BudgetCacheTTL,
		},
	}
}

// GetModelCost returns the cost configuration for a model.
// It tries exact match first, then falls back to matching without version suffix.
func (s *Service) GetModelCost(ctx context.Context, modelID string) (*models.ModelCost, error) {
	// Check cache for exact match
	s.modelCostCache.RLock()
	if time.Now().Before(s.modelCostCache.expiresAt) {
		if cost, ok := s.modelCostCache.costs[modelID]; ok {
			s.modelCostCache.RUnlock()
			return cost, nil
		}
	}
	s.modelCostCache.RUnlock()

	// Try exact match in database
	cost, err := s.repo.GetModelCostByID(ctx, modelID)
	if err != nil && errors.Is(err, db.ErrNotFound) {
		// Try stripping version suffix (e.g., gpt-4o-mini-2024-07-18 -> gpt-4o-mini)
		baseModelID := modelVersionSuffixRegex.ReplaceAllString(modelID, "")
		if baseModelID != modelID {
			cost, err = s.repo.GetModelCostByID(ctx, baseModelID)
			if err == nil {
				log.Debug().Str("model", modelID).Str("matched", baseModelID).Msg("matched versioned model to base model")
			}
		}
	}
	if err != nil {
		return nil, err
	}

	// Update cache (cache the original modelID pointing to the resolved cost)
	s.modelCostCache.Lock()
	s.modelCostCache.costs[modelID] = cost
	if time.Now().After(s.modelCostCache.expiresAt) {
		s.modelCostCache.expiresAt = time.Now().Add(s.modelCostCache.ttl)
	}
	s.modelCostCache.Unlock()

	return cost, nil
}

// RefreshModelCostCache refreshes the model cost cache.
func (s *Service) RefreshModelCostCache(ctx context.Context) error {
	costs, err := s.repo.ListModelCosts(ctx)
	if err != nil {
		return err
	}

	s.modelCostCache.Lock()
	s.modelCostCache.costs = make(map[string]*models.ModelCost)
	for i := range costs {
		s.modelCostCache.costs[costs[i].ModelID] = &costs[i]
	}
	s.modelCostCache.expiresAt = time.Now().Add(s.modelCostCache.ttl)
	s.modelCostCache.Unlock()

	return nil
}

// GetMatchingBudgets returns all budgets that match the given context.
func (s *Service) GetMatchingBudgets(ctx context.Context, evalCtx *cel.EvalContext) ([]models.BudgetDefinition, error) {
	// Check cache
	s.budgetCache.RLock()
	budgets := s.budgetCache.budgets
	expired := time.Now().After(s.budgetCache.expiresAt)
	s.budgetCache.RUnlock()

	if expired || budgets == nil {
		var err error
		// Only get enabled budgets for enforcement
		budgets, err = s.repo.GetEnabledBudgets(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to get budgets: %w", err)
		}

		s.budgetCache.Lock()
		s.budgetCache.budgets = budgets
		s.budgetCache.expiresAt = time.Now().Add(s.budgetCache.ttl)
		s.budgetCache.Unlock()
	}

	// Filter budgets by CEL expression
	var matched []models.BudgetDefinition
	for _, b := range budgets {
		match, err := s.celEvaluator.Evaluate(b.MatchExpression, evalCtx)
		if err != nil {
			log.Warn().Err(err).Str("budget_id", b.ID.String()).Msg("failed to evaluate CEL expression")
			continue
		}
		if match {
			matched = append(matched, b)
		}
	}

	return matched, nil
}

// CalculateCost calculates the cost for a request.
func (s *Service) CalculateCost(ctx context.Context, modelID string, inputTokens, outputTokens int64) (float64, error) {
	cost, err := s.GetModelCost(ctx, modelID)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			// Use a default cost if model not found
			// Only warn if we actually have a model ID (empty is expected during headers phase)
			if modelID != "" {
				log.Warn().Str("model", modelID).Msg("model cost not found, using default")
			}
			// Default: $1/1M input, $3/1M output
			return (float64(inputTokens) * 1.0 / 1_000_000) + (float64(outputTokens) * 3.0 / 1_000_000), nil
		}
		return 0, err
	}

	inputCost := float64(inputTokens) * cost.InputCostPerMillion / 1_000_000
	outputCost := float64(outputTokens) * cost.OutputCostPerMillion / 1_000_000

	return inputCost + outputCost, nil
}

// EstimateCost estimates the cost for a request before it's processed.
func (s *Service) EstimateCost(ctx context.Context, modelID string) float64 {
	estimatedInput := s.cfg.DefaultEstimatedInputTokens
	estimatedOutput := s.cfg.DefaultEstimatedOutputTokens

	cost, err := s.CalculateCost(ctx, modelID, estimatedInput, estimatedOutput)
	if err != nil {
		log.Warn().Err(err).Str("model", modelID).Msg("failed to estimate cost")
		// Return a conservative default
		return 0.01 // $0.01 default estimate
	}

	return cost * s.cfg.DefaultEstimationMultiplier
}

// CheckBudget checks if there's enough budget for a request.
func (s *Service) CheckBudget(ctx context.Context, evalCtx *cel.EvalContext, modelID string) (*CheckResult, error) {
	// Get matching budgets
	budgets, err := s.GetMatchingBudgets(ctx, evalCtx)
	if err != nil {
		return nil, err
	}

	if len(budgets) == 0 {
		// No budgets configured, allow the request
		return &CheckResult{
			Allowed:        true,
			MatchedBudgets: nil,
		}, nil
	}

	// Calculate estimated cost
	estimatedCost := s.EstimateCost(ctx, modelID)

	// Sort budgets by hierarchy (children first, parents last)
	// UseCases before Channels
	sortedBudgets := sortBudgetsByHierarchy(budgets)

	// Build a map of parent budgets for quick lookup
	parentMap := make(map[uuid.UUID]*models.BudgetDefinition)
	for i := range sortedBudgets {
		b := &sortedBudgets[i]
		if b.ParentID != nil {
			for j := range sortedBudgets {
				if sortedBudgets[j].ID == *b.ParentID {
					parentMap[b.ID] = &sortedBudgets[j]
					break
				}
			}
		}
	}

	// Check each budget
	var rateLimitedAt *uuid.UUID
	var fallbackBudgetID *uuid.UUID
	var minRemaining float64 = -1

	for _, budget := range sortedBudgets {
		remaining := budget.CalculateRemaining()

		// Track minimum remaining
		if minRemaining < 0 || remaining < minRemaining {
			minRemaining = remaining
		}

		// Check if budget allows the request
		if remaining < estimatedCost {
			// Budget exceeded - check if fallback is allowed
			if budget.AllowFallback && budget.ParentID != nil {
				parent := parentMap[budget.ID]
				if parent != nil && parent.CalculateRemaining() >= estimatedCost {
					// Parent has capacity - allow fallback
					parentID := parent.ID
					fallbackBudgetID = &parentID

					// Record fallback metric
					metrics.RecordBudgetFallback(
						string(budget.EntityType),
						budget.Name,
						string(parent.EntityType),
						parent.Name,
					)

					log.Debug().
						Str("child_budget", budget.Name).
						Str("parent_budget", parent.Name).
						Float64("estimated_cost", estimatedCost).
						Msg("budget fallback to parent")

					// Don't rate limit, continue checking other budgets
					continue
				}
			}

			// No fallback available - rate limit
			budgetID := budget.ID
			rateLimitedAt = &budgetID

			// If this budget is isolated, stop checking parents
			if budget.Isolated {
				break
			}
		}
	}

	if rateLimitedAt != nil {
		// Find the rate-limited budget to calculate retry-after
		var retryAfter time.Duration
		for _, b := range budgets {
			if b.ID == *rateLimitedAt {
				retryAfter = time.Until(b.NextPeriodStart())
				break
			}
		}

		return &CheckResult{
			Allowed:         false,
			MatchedBudgets:  sortedBudgets,
			RateLimitedAt:   rateLimitedAt,
			EstimatedCost:   estimatedCost,
			RemainingBudget: minRemaining,
			RetryAfter:      retryAfter,
		}, nil
	}

	return &CheckResult{
		Allowed:          true,
		MatchedBudgets:   sortedBudgets,
		FallbackBudgetID: fallbackBudgetID,
		EstimatedCost:    estimatedCost,
		RemainingBudget:  minRemaining,
	}, nil
}

// CreateReservation creates a budget reservation for a request.
func (s *Service) CreateReservation(ctx context.Context, requestID string, budgets []models.BudgetDefinition, estimatedCost float64) error {
	expiresAt := time.Now().Add(s.cfg.ReservationTTL)

	for _, budget := range budgets {
		res := &models.RequestReservation{
			BudgetID:         budget.ID,
			RequestID:        requestID,
			EstimatedCostUSD: estimatedCost,
			ExpiresAt:        expiresAt,
		}

		if err := s.repo.CreateReservation(ctx, res); err != nil {
			log.Warn().Err(err).
				Str("budget_id", budget.ID.String()).
				Str("request_id", requestID).
				Msg("failed to create reservation")
			// Continue with other budgets
		} else {
			metrics.ActiveReservations.Inc()
			metrics.ReservationsCreatedTotal.Inc()
		}
	}

	return nil
}

// DecrementBudgets decrements budgets after a request completes.
func (s *Service) DecrementBudgets(ctx context.Context, requestID string, modelID string, inputTokens, outputTokens int64, rateLimitedAt *uuid.UUID) (*DecrementResult, error) {
	// Calculate actual cost
	actualCost, err := s.CalculateCost(ctx, modelID, inputTokens, outputTokens)
	if err != nil {
		return nil, err
	}

	// Get reservation to find associated budgets
	res, err := s.repo.GetReservationByRequestID(ctx, requestID)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			log.Warn().Str("request_id", requestID).Msg("reservation not found, cannot decrement budgets")
			return &DecrementResult{ActualCost: actualCost}, nil
		}
		return nil, err
	}

	// Get budget hierarchy
	budgets, err := s.repo.GetBudgetsWithParent(ctx, res.BudgetID)
	if err != nil {
		return nil, err
	}

	// Begin transaction
	tx, err := s.repo.BeginTx(ctx)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback(ctx)
		}
	}()

	var chargedBudgets []uuid.UUID
	var minRemaining float64 = -1

	for _, budget := range budgets {
		shouldCharge := true
		parentCharged := true

		// Check if this is a parent of a rate-limited isolated budget
		if rateLimitedAt != nil && budget.ID != *rateLimitedAt {
			// Check if this budget is an ancestor of the rate-limited budget
			for _, b := range budgets {
				if b.ID == *rateLimitedAt && b.Isolated {
					// This budget is an ancestor of an isolated rate-limited budget
					// Don't charge parent budgets
					if budget.ParentID == nil || budget.ID != *rateLimitedAt {
						// This is a parent, skip charging
						isParent := false
						for _, ancestor := range budgets {
							if ancestor.ParentID != nil && *ancestor.ParentID == budget.ID {
								isParent = true
								break
							}
						}
						if isParent {
							shouldCharge = false
							parentCharged = false
						}
					}
					break
				}
			}
		}

		if shouldCharge {
			// Increment usage
			if err = s.repo.IncrementUsageInTx(ctx, tx, budget.ID, actualCost); err != nil {
				return nil, err
			}
			chargedBudgets = append(chargedBudgets, budget.ID)
		}

		// Create usage record
		ur := &models.UsageRecord{
			BudgetID:      budget.ID,
			RequestID:     requestID,
			ModelID:       modelID,
			InputTokens:   inputTokens,
			OutputTokens:  outputTokens,
			CostUSD:       actualCost,
			ParentCharged: parentCharged,
		}
		if err = s.repo.CreateUsageRecordInTx(ctx, tx, ur); err != nil {
			return nil, err
		}

		// Calculate remaining after decrement
		remaining := budget.BudgetAmountUSD - budget.CurrentUsageUSD - actualCost - budget.PendingUsageUSD
		if minRemaining < 0 || remaining < minRemaining {
			minRemaining = remaining
		}
	}

	// Delete reservation
	if err = s.repo.DeleteReservationInTx(ctx, tx, requestID); err != nil {
		log.Warn().Err(err).Str("request_id", requestID).Msg("failed to delete reservation in tx")
	} else {
		metrics.ActiveReservations.Dec()
	}

	// Decrement pending usage
	if err = s.repo.DecrementPendingUsageInTx(ctx, tx, res.BudgetID, res.EstimatedCostUSD); err != nil {
		log.Warn().Err(err).Str("budget_id", res.BudgetID.String()).Msg("failed to decrement pending usage")
	}

	// Commit transaction
	if err = tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	// Invalidate cache
	s.budgetCache.Lock()
	s.budgetCache.expiresAt = time.Time{}
	s.budgetCache.Unlock()

	return &DecrementResult{
		ActualCost:      actualCost,
		RemainingBudget: minRemaining,
		BudgetsCharged:  chargedBudgets,
	}, nil
}

// sortBudgetsByHierarchy sorts budgets so that children come before parents.
// Order: Team (most specific) -> Org -> Provider (most general)
func sortBudgetsByHierarchy(budgets []models.BudgetDefinition) []models.BudgetDefinition {
	sorted := make([]models.BudgetDefinition, 0, len(budgets))

	// First, add all Teams (most specific)
	for _, b := range budgets {
		if b.EntityType == models.EntityTypeTeam {
			sorted = append(sorted, b)
		}
	}

	// Then, add all Orgs
	for _, b := range budgets {
		if b.EntityType == models.EntityTypeOrg {
			sorted = append(sorted, b)
		}
	}

	// Finally, add all Providers (most general)
	for _, b := range budgets {
		if b.EntityType == models.EntityTypeProvider {
			sorted = append(sorted, b)
		}
	}

	return sorted
}

// CleanupExpiredReservations removes expired reservations.
func (s *Service) CleanupExpiredReservations(ctx context.Context) (int64, error) {
	count, err := s.repo.CleanupExpiredReservations(ctx)
	if err != nil {
		return 0, err
	}
	if count > 0 {
		metrics.ActiveReservations.Sub(float64(count))
		metrics.ReservationsExpiredTotal.Add(float64(count))
	}
	return count, nil
}

// ResetExpiredBudgets resets budgets whose periods have expired.
func (s *Service) ResetExpiredBudgets(ctx context.Context) (int64, error) {
	count, err := s.repo.ResetExpiredBudgets(ctx)
	if err != nil {
		return 0, err
	}

	if count > 0 {
		// Invalidate cache
		s.budgetCache.Lock()
		s.budgetCache.expiresAt = time.Time{}
		s.budgetCache.Unlock()
	}

	return count, nil
}

// RefreshBudgetMetrics updates Prometheus gauges with current budget state from the database.
func (s *Service) RefreshBudgetMetrics(ctx context.Context) error {
	budgets, err := s.repo.GetAllBudgets(ctx)
	if err != nil {
		return fmt.Errorf("failed to get budgets for metrics: %w", err)
	}

	for _, b := range budgets {
		remaining := b.CalculateRemaining()
		metrics.UpdateBudgetUsage(
			string(b.EntityType),
			b.Name,
			string(b.Period),
			b.CurrentUsageUSD,
			remaining,
			b.BudgetAmountUSD,
		)
	}

	return nil
}
