package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/agentgateway/budget-management/internal/auth"
	"github.com/agentgateway/budget-management/internal/cel"
	"github.com/agentgateway/budget-management/internal/db"
	"github.com/agentgateway/budget-management/internal/metrics"
	"github.com/agentgateway/budget-management/internal/models"
	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/rs/zerolog/log"
)

// Handler provides HTTP handlers for the management API.
type Handler struct {
	repo         *db.Repository
	celEvaluator *cel.Evaluator
}

// NewHandler creates a new API handler.
func NewHandler(repo *db.Repository, celEvaluator *cel.Evaluator) *Handler {
	return &Handler{repo: repo, celEvaluator: celEvaluator}
}

// RegisterRoutes registers all API routes.
func (h *Handler) RegisterRoutes(r *mux.Router) {
	// Identity endpoint (uses optional auth to return identity if JWT present)
	r.Handle("/api/v1/identity", auth.OptionalAuthMiddleware(http.HandlerFunc(h.GetIdentity))).Methods("GET")

	// Model costs
	r.HandleFunc("/api/v1/model-costs", h.ListModelCosts).Methods("GET")
	r.HandleFunc("/api/v1/model-costs", h.CreateModelCost).Methods("POST")
	r.HandleFunc("/api/v1/model-costs/{model_id}", h.GetModelCost).Methods("GET")
	r.HandleFunc("/api/v1/model-costs/{model_id}", h.UpdateModelCost).Methods("PUT")
	r.HandleFunc("/api/v1/model-costs/{model_id}", h.DeleteModelCost).Methods("DELETE")

	// Budgets (use optional auth to filter by identity when authenticated)
	r.Handle("/api/v1/budgets", auth.OptionalAuthMiddleware(http.HandlerFunc(h.ListBudgets))).Methods("GET")
	r.Handle("/api/v1/budgets", auth.OptionalAuthMiddleware(http.HandlerFunc(h.CreateBudget))).Methods("POST")
	r.Handle("/api/v1/budgets/{id}", auth.OptionalAuthMiddleware(http.HandlerFunc(h.GetBudget))).Methods("GET")
	r.Handle("/api/v1/budgets/{id}", auth.OptionalAuthMiddleware(http.HandlerFunc(h.UpdateBudget))).Methods("PUT")
	r.Handle("/api/v1/budgets/{id}", auth.OptionalAuthMiddleware(http.HandlerFunc(h.DeleteBudget))).Methods("DELETE")
	r.Handle("/api/v1/budgets/{id}/usage", auth.OptionalAuthMiddleware(http.HandlerFunc(h.GetBudgetUsage))).Methods("GET")
	r.Handle("/api/v1/budgets/{id}/reset", auth.OptionalAuthMiddleware(http.HandlerFunc(h.ResetBudget))).Methods("POST")

	// CEL validation
	r.HandleFunc("/api/v1/validate-cel", h.ValidateCEL).Methods("POST")

	// Health check
	r.HandleFunc("/health", h.Health).Methods("GET")
	r.HandleFunc("/ready", h.Ready).Methods("GET")
}

// Response helpers

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Error().Err(err).Msg("failed to encode JSON response")
	}
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]interface{}{
		"error": map[string]string{
			"message": message,
		},
	})
}

// modelCostToMap converts a ModelCost to a map with proper JSON serialization
// for sql.Null* types (which otherwise serialize as {"String":"...", "Valid": true}).
func modelCostToMap(c *models.ModelCost) map[string]interface{} {
	result := map[string]interface{}{
		"id":                     c.ID,
		"model_id":               c.ModelID,
		"provider":               c.Provider,
		"input_cost_per_million":  c.InputCostPerMillion,
		"output_cost_per_million": c.OutputCostPerMillion,
		"effective_date":         c.EffectiveDate,
		"created_at":             c.CreatedAt,
		"updated_at":             c.UpdatedAt,
	}

	// Handle nullable fields - only include if valid
	if c.CacheReadCostMillion.Valid {
		result["cache_read_cost_million"] = c.CacheReadCostMillion.Float64
	}
	if c.CacheWriteCostMillion.Valid {
		result["cache_write_cost_million"] = c.CacheWriteCostMillion.Float64
	}
	if c.ModelPattern.Valid {
		result["model_pattern"] = c.ModelPattern.String
	}

	return result
}

// budgetToMap converts a BudgetDefinition to a map with proper JSON serialization.
func budgetToMap(b *models.BudgetDefinition) map[string]interface{} {
	result := map[string]interface{}{
		"id":                    b.ID,
		"entity_type":           b.EntityType,
		"name":                  b.Name,
		"match_expression":      b.MatchExpression,
		"budget_amount_usd":     b.BudgetAmountUSD,
		"period":                b.Period,
		"warning_threshold_pct": b.WarningThresholdPct,
		"parent_id":             b.ParentID,
		"isolated":              b.Isolated,
		"allow_fallback":        b.AllowFallback,
		"enabled":               b.Enabled,
		"current_period_start":  b.CurrentPeriodStart,
		"current_usage_usd":     b.CurrentUsageUSD,
		"pending_usage_usd":     b.PendingUsageUSD,
		"remaining_usd":         b.CalculateRemaining(),
		"created_at":            b.CreatedAt,
		"updated_at":            b.UpdatedAt,
	}

	if b.Description.Valid {
		result["description"] = b.Description.String
	}
	if b.CustomPeriodSeconds.Valid {
		result["custom_period_seconds"] = b.CustomPeriodSeconds.Int32
	}

	return result
}

// Model Cost handlers

// ListModelCosts lists all model costs.
func (h *Handler) ListModelCosts(w http.ResponseWriter, r *http.Request) {
	costs, err := h.repo.ListModelCosts(r.Context())
	if err != nil {
		log.Error().Err(err).Msg("failed to list model costs")
		writeError(w, http.StatusInternalServerError, "failed to list model costs")
		return
	}

	// Convert sql.Null* types to proper JSON values
	result := make([]map[string]interface{}, len(costs))
	for i, c := range costs {
		result[i] = modelCostToMap(&c)
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"model_costs": result,
	})
}

// GetModelCost gets a model cost by model ID.
func (h *Handler) GetModelCost(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	modelID := vars["model_id"]

	cost, err := h.repo.GetModelCostByID(r.Context(), modelID)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			writeError(w, http.StatusNotFound, "model cost not found")
			return
		}
		log.Error().Err(err).Str("model_id", modelID).Msg("failed to get model cost")
		writeError(w, http.StatusInternalServerError, "failed to get model cost")
		return
	}

	writeJSON(w, http.StatusOK, modelCostToMap(cost))
}

// CreateModelCostRequest represents a create model cost request.
type CreateModelCostRequest struct {
	ModelID               string   `json:"model_id"`
	Provider              string   `json:"provider"`
	InputCostPerMillion   float64  `json:"input_cost_per_million"`
	OutputCostPerMillion  float64  `json:"output_cost_per_million"`
	CacheReadCostMillion  *float64 `json:"cache_read_cost_million,omitempty"`
	CacheWriteCostMillion *float64 `json:"cache_write_cost_million,omitempty"`
	ModelPattern          *string  `json:"model_pattern,omitempty"`
}

// CreateModelCost creates a new model cost.
func (h *Handler) CreateModelCost(w http.ResponseWriter, r *http.Request) {
	var req CreateModelCostRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.ModelID == "" {
		writeError(w, http.StatusBadRequest, "model_id is required")
		return
	}

	if req.Provider == "" {
		writeError(w, http.StatusBadRequest, "provider is required")
		return
	}

	mc := &models.ModelCost{
		ModelID:              req.ModelID,
		Provider:             req.Provider,
		InputCostPerMillion:  req.InputCostPerMillion,
		OutputCostPerMillion: req.OutputCostPerMillion,
		EffectiveDate:        time.Now(),
	}

	if err := h.repo.CreateModelCost(r.Context(), mc); err != nil {
		log.Error().Err(err).Msg("failed to create model cost")
		writeError(w, http.StatusInternalServerError, "failed to create model cost")
		return
	}

	writeJSON(w, http.StatusCreated, mc)
}

// UpdateModelCost updates a model cost.
func (h *Handler) UpdateModelCost(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	modelID := vars["model_id"]

	var req CreateModelCostRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	mc := &models.ModelCost{
		ModelID:              modelID,
		Provider:             req.Provider,
		InputCostPerMillion:  req.InputCostPerMillion,
		OutputCostPerMillion: req.OutputCostPerMillion,
	}

	if err := h.repo.UpdateModelCost(r.Context(), mc); err != nil {
		if errors.Is(err, db.ErrNotFound) {
			writeError(w, http.StatusNotFound, "model cost not found")
			return
		}
		log.Error().Err(err).Str("model_id", modelID).Msg("failed to update model cost")
		writeError(w, http.StatusInternalServerError, "failed to update model cost")
		return
	}

	writeJSON(w, http.StatusOK, mc)
}

// DeleteModelCost deletes a model cost.
func (h *Handler) DeleteModelCost(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	modelID := vars["model_id"]

	if err := h.repo.DeleteModelCost(r.Context(), modelID); err != nil {
		if errors.Is(err, db.ErrNotFound) {
			writeError(w, http.StatusNotFound, "model cost not found")
			return
		}
		log.Error().Err(err).Str("model_id", modelID).Msg("failed to delete model cost")
		writeError(w, http.StatusInternalServerError, "failed to delete model cost")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// Budget handlers

// ListBudgets lists all budgets, filtered by the user's identity if authenticated.
func (h *Handler) ListBudgets(w http.ResponseWriter, r *http.Request) {
	budgets, err := h.repo.ListBudgets(r.Context())
	if err != nil {
		log.Error().Err(err).Msg("failed to list budgets")
		writeError(w, http.StatusInternalServerError, "failed to list budgets")
		return
	}

	// Filter budgets by identity if authenticated
	identity := auth.GetIdentity(r.Context())
	if identity != nil {
		var ownership []auth.BudgetOwnership
		for i, b := range budgets {
			ownership = append(ownership, auth.BudgetOwnership{
				Index:       i,
				OwnerOrgID:  b.OwnerOrgID.String,
				OwnerTeamID: b.OwnerTeamID.String,
			})
			log.Debug().
				Str("budget_name", b.Name).
				Str("owner_org_id", b.OwnerOrgID.String).
				Str("owner_team_id", b.OwnerTeamID.String).
				Bool("org_valid", b.OwnerOrgID.Valid).
				Bool("team_valid", b.OwnerTeamID.Valid).
				Msg("budget ownership info")
		}

		allowedIndices := auth.FilterBudgetsByIdentity(identity, ownership)
		filteredBudgets := make([]models.BudgetDefinition, 0, len(allowedIndices))
		for _, idx := range allowedIndices {
			filteredBudgets = append(filteredBudgets, budgets[idx])
		}
		budgets = filteredBudgets

		log.Debug().
			Str("identity_org_id", identity.OrgID).
			Str("identity_team_id", identity.TeamID).
			Bool("identity_is_org", identity.IsOrg).
			Int("total_budgets", len(ownership)).
			Int("filtered_budgets", len(budgets)).
			Msg("filtered budgets by identity")
	} else {
		log.Debug().Msg("no identity in context, returning all budgets")
	}

	// Add remaining budget calculation
	result := make([]map[string]interface{}, len(budgets))
	for i, b := range budgets {
		item := map[string]interface{}{
			"id":                    b.ID,
			"entity_type":           b.EntityType,
			"name":                  b.Name,
			"match_expression":      b.MatchExpression,
			"budget_amount_usd":     b.BudgetAmountUSD,
			"period":                b.Period,
			"warning_threshold_pct": b.WarningThresholdPct,
			"parent_id":             b.ParentID,
			"isolated":              b.Isolated,
			"allow_fallback":        b.AllowFallback,
			"enabled":               b.Enabled,
			"current_period_start":  b.CurrentPeriodStart,
			"current_usage_usd":     b.CurrentUsageUSD,
			"pending_usage_usd":     b.PendingUsageUSD,
			"remaining_usd":         b.CalculateRemaining(),
			"description":           b.Description.String,
			"version":               b.Version,
			"created_at":            b.CreatedAt,
			"updated_at":            b.UpdatedAt,
		}
		if b.OwnerOrgID.Valid {
			item["owner_org_id"] = b.OwnerOrgID.String
		}
		if b.OwnerTeamID.Valid {
			item["owner_team_id"] = b.OwnerTeamID.String
		}
		result[i] = item
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"budgets": result,
	})
}

// GetBudget gets a budget by ID.
func (h *Handler) GetBudget(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id, err := uuid.Parse(vars["id"])
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid budget ID")
		return
	}

	budget, err := h.repo.GetBudgetByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			writeError(w, http.StatusNotFound, "budget not found")
			return
		}
		log.Error().Err(err).Str("id", id.String()).Msg("failed to get budget")
		writeError(w, http.StatusInternalServerError, "failed to get budget")
		return
	}

	result := map[string]interface{}{
		"id":                    budget.ID,
		"entity_type":           budget.EntityType,
		"name":                  budget.Name,
		"match_expression":      budget.MatchExpression,
		"budget_amount_usd":     budget.BudgetAmountUSD,
		"period":                budget.Period,
		"warning_threshold_pct": budget.WarningThresholdPct,
		"parent_id":             budget.ParentID,
		"isolated":              budget.Isolated,
		"allow_fallback":        budget.AllowFallback,
		"enabled":               budget.Enabled,
		"current_period_start":  budget.CurrentPeriodStart,
		"current_usage_usd":     budget.CurrentUsageUSD,
		"pending_usage_usd":     budget.PendingUsageUSD,
		"remaining_usd":         budget.CalculateRemaining(),
		"next_period_start":     budget.NextPeriodStart(),
		"description":           budget.Description.String,
		"version":               budget.Version,
		"created_at":            budget.CreatedAt,
		"updated_at":            budget.UpdatedAt,
	}
	if budget.OwnerOrgID.Valid {
		result["owner_org_id"] = budget.OwnerOrgID.String
	}
	if budget.OwnerTeamID.Valid {
		result["owner_team_id"] = budget.OwnerTeamID.String
	}

	writeJSON(w, http.StatusOK, result)
}

// CreateBudgetRequest represents a create budget request.
type CreateBudgetRequest struct {
	EntityType          string  `json:"entity_type"`
	Name                string  `json:"name"`
	MatchExpression     string  `json:"match_expression"`
	BudgetAmountUSD     float64 `json:"budget_amount_usd"`
	Period              string  `json:"period"`
	CustomPeriodSeconds *int32  `json:"custom_period_seconds,omitempty"`
	WarningThresholdPct *int    `json:"warning_threshold_pct,omitempty"`
	ParentID            *string `json:"parent_id,omitempty"`
	Isolated            *bool   `json:"isolated,omitempty"`
	AllowFallback       *bool   `json:"allow_fallback,omitempty"`
	Enabled             *bool   `json:"enabled,omitempty"`
	Description         *string `json:"description,omitempty"`
	Version             *int64  `json:"version,omitempty"`
	OwnerOrgID          *string `json:"owner_org_id,omitempty"`
	OwnerTeamID         *string `json:"owner_team_id,omitempty"`
}

// CreateBudget creates a new budget.
func (h *Handler) CreateBudget(w http.ResponseWriter, r *http.Request) {
	var req CreateBudgetRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.EntityType == "" {
		writeError(w, http.StatusBadRequest, "entity_type is required")
		return
	}

	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}

	if req.MatchExpression == "" {
		writeError(w, http.StatusBadRequest, "match_expression is required")
		return
	}

	if req.Period == "" {
		writeError(w, http.StatusBadRequest, "period is required")
		return
	}

	budget := &models.BudgetDefinition{
		EntityType:      models.EntityType(req.EntityType),
		Name:            req.Name,
		MatchExpression: req.MatchExpression,
		BudgetAmountUSD: req.BudgetAmountUSD,
		Period:          models.BudgetPeriod(req.Period),
		Isolated:        true, // Default to isolated
		Enabled:         true, // Default to enabled
	}

	if req.CustomPeriodSeconds != nil {
		budget.CustomPeriodSeconds.Valid = true
		budget.CustomPeriodSeconds.Int32 = *req.CustomPeriodSeconds
	}

	if req.WarningThresholdPct != nil {
		budget.WarningThresholdPct = *req.WarningThresholdPct
	} else {
		budget.WarningThresholdPct = 80
	}

	var warning string
	if req.ParentID != nil {
		parentID, err := uuid.Parse(*req.ParentID)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid parent_id")
			return
		}
		budget.ParentID = &parentID

		// Warn if child budget exceeds parent budget
		parent, err := h.repo.GetBudgetByID(r.Context(), parentID)
		if err == nil && budget.BudgetAmountUSD > parent.BudgetAmountUSD {
			warning = fmt.Sprintf("budget $%.4f exceeds parent budget $%.4f", budget.BudgetAmountUSD, parent.BudgetAmountUSD)
			log.Warn().Str("child", budget.Name).Str("parent", parent.Name).Msg(warning)
		}
	}

	if req.Isolated != nil {
		budget.Isolated = *req.Isolated
	}

	if req.AllowFallback != nil {
		budget.AllowFallback = *req.AllowFallback
	}

	if req.Enabled != nil {
		budget.Enabled = *req.Enabled
	}

	if req.Description != nil {
		budget.Description.Valid = true
		budget.Description.String = *req.Description
	}

	// Set ownership from request or identity
	if req.OwnerOrgID != nil && *req.OwnerOrgID != "" {
		budget.OwnerOrgID.Valid = true
		budget.OwnerOrgID.String = *req.OwnerOrgID
	}
	if req.OwnerTeamID != nil && *req.OwnerTeamID != "" {
		budget.OwnerTeamID.Valid = true
		budget.OwnerTeamID.String = *req.OwnerTeamID
	}

	// If no ownership specified in request, use identity
	identity := auth.GetIdentity(r.Context())
	if identity != nil && !budget.OwnerOrgID.Valid && !budget.OwnerTeamID.Valid {
		orgID, teamID := auth.GetOwnershipFromIdentity(identity)
		if orgID != "" {
			budget.OwnerOrgID.Valid = true
			budget.OwnerOrgID.String = orgID
		}
		if teamID != "" {
			budget.OwnerTeamID.Valid = true
			budget.OwnerTeamID.String = teamID
		}
	}

	// Check if budget already exists (upsert behavior)
	existing, err := h.repo.GetBudgetByEntity(r.Context(), budget.EntityType, budget.Name)
	if err == nil {
		// Budget exists - return it with 200 OK
		log.Info().
			Str("entity_type", string(existing.EntityType)).
			Str("name", existing.Name).
			Msg("budget already exists, returning existing")
		writeJSON(w, http.StatusOK, budgetToMap(existing))
		return
	}

	if err := h.repo.CreateBudget(r.Context(), budget); err != nil {
		log.Error().Err(err).Msg("failed to create budget")
		writeError(w, http.StatusInternalServerError, "failed to create budget")
		return
	}

	resp := budgetToMap(budget)
	if warning != "" {
		resp["warning"] = warning
	}
	writeJSON(w, http.StatusCreated, resp)
}

// UpdateBudget updates a budget.
func (h *Handler) UpdateBudget(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id, err := uuid.Parse(vars["id"])
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid budget ID")
		return
	}

	// Get existing budget
	existing, err := h.repo.GetBudgetByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			writeError(w, http.StatusNotFound, "budget not found")
			return
		}
		log.Error().Err(err).Str("id", id.String()).Msg("failed to get budget")
		writeError(w, http.StatusInternalServerError, "failed to get budget")
		return
	}

	var req CreateBudgetRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// Update fields
	if req.MatchExpression != "" {
		existing.MatchExpression = req.MatchExpression
	}
	if req.BudgetAmountUSD > 0 {
		existing.BudgetAmountUSD = req.BudgetAmountUSD
	}
	if req.Period != "" {
		existing.Period = models.BudgetPeriod(req.Period)
	}
	if req.CustomPeriodSeconds != nil {
		existing.CustomPeriodSeconds.Valid = true
		existing.CustomPeriodSeconds.Int32 = *req.CustomPeriodSeconds
	}
	if req.WarningThresholdPct != nil {
		existing.WarningThresholdPct = *req.WarningThresholdPct
	}
	if req.ParentID != nil {
		parentID, err := uuid.Parse(*req.ParentID)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid parent_id")
			return
		}
		existing.ParentID = &parentID
	}
	if req.Isolated != nil {
		existing.Isolated = *req.Isolated
	}
	if req.AllowFallback != nil {
		existing.AllowFallback = *req.AllowFallback
	}
	if req.Enabled != nil {
		existing.Enabled = *req.Enabled
	}
	if req.Description != nil {
		existing.Description.Valid = true
		existing.Description.String = *req.Description
	}
	if req.Version != nil {
		existing.Version = *req.Version
	}
	if req.OwnerOrgID != nil {
		if *req.OwnerOrgID == "" {
			existing.OwnerOrgID.Valid = false
			existing.OwnerOrgID.String = ""
		} else {
			existing.OwnerOrgID.Valid = true
			existing.OwnerOrgID.String = *req.OwnerOrgID
		}
	}
	if req.OwnerTeamID != nil {
		if *req.OwnerTeamID == "" {
			existing.OwnerTeamID.Valid = false
			existing.OwnerTeamID.String = ""
		} else {
			existing.OwnerTeamID.Valid = true
			existing.OwnerTeamID.String = *req.OwnerTeamID
		}
	}

	// Warn if child budget exceeds parent budget
	var warning string
	if existing.ParentID != nil {
		parent, err := h.repo.GetBudgetByID(r.Context(), *existing.ParentID)
		if err == nil && existing.BudgetAmountUSD > parent.BudgetAmountUSD {
			warning = fmt.Sprintf("budget $%.4f exceeds parent budget $%.4f", existing.BudgetAmountUSD, parent.BudgetAmountUSD)
			log.Warn().Str("child", existing.Name).Str("parent", parent.Name).Msg(warning)
		}
	}

	if err := h.repo.UpdateBudget(r.Context(), existing); err != nil {
		if errors.Is(err, db.ErrOptimisticLock) {
			writeError(w, http.StatusConflict, "budget was modified by another request")
			return
		}
		log.Error().Err(err).Str("id", id.String()).Msg("failed to update budget")
		writeError(w, http.StatusInternalServerError, "failed to update budget")
		return
	}

	resp := budgetToMap(existing)
	if warning != "" {
		resp["warning"] = warning
	}
	writeJSON(w, http.StatusOK, resp)
}

// DeleteBudget deletes a budget.
func (h *Handler) DeleteBudget(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id, err := uuid.Parse(vars["id"])
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid budget ID")
		return
	}

	// Fetch budget first to get info for metrics cleanup
	budget, err := h.repo.GetBudgetByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			writeError(w, http.StatusNotFound, "budget not found")
			return
		}
		log.Error().Err(err).Str("id", id.String()).Msg("failed to fetch budget for deletion")
		writeError(w, http.StatusInternalServerError, "failed to delete budget")
		return
	}

	if err := h.repo.DeleteBudget(r.Context(), id); err != nil {
		if errors.Is(err, db.ErrNotFound) {
			writeError(w, http.StatusNotFound, "budget not found")
			return
		}
		log.Error().Err(err).Str("id", id.String()).Msg("failed to delete budget")
		writeError(w, http.StatusInternalServerError, "failed to delete budget")
		return
	}

	// Clean up Prometheus metrics for the deleted budget
	metrics.DeleteBudgetMetrics(string(budget.EntityType), budget.Name, string(budget.Period))

	w.WriteHeader(http.StatusNoContent)
}

// GetBudgetUsage gets usage history for a budget.
func (h *Handler) GetBudgetUsage(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id, err := uuid.Parse(vars["id"])
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid budget ID")
		return
	}

	// Parse query parameters
	since := time.Now().AddDate(0, 0, -7) // Default to last 7 days
	if sinceStr := r.URL.Query().Get("since"); sinceStr != "" {
		parsedTime, err := time.Parse(time.RFC3339, sinceStr)
		if err == nil {
			since = parsedTime
		}
	}

	limit := 100
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		parsedLimit, err := strconv.Atoi(limitStr)
		if err == nil && parsedLimit > 0 && parsedLimit <= 1000 {
			limit = parsedLimit
		}
	}

	records, err := h.repo.GetUsageByBudgetID(r.Context(), id, since, limit)
	if err != nil {
		log.Error().Err(err).Str("id", id.String()).Msg("failed to get usage records")
		writeError(w, http.StatusInternalServerError, "failed to get usage records")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"usage_records": records,
	})
}

// ResetBudget resets the usage for a budget.
func (h *Handler) ResetBudget(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id, err := uuid.Parse(vars["id"])
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid budget ID")
		return
	}

	if err := h.repo.ResetBudgetUsage(r.Context(), id); err != nil {
		if errors.Is(err, db.ErrNotFound) {
			writeError(w, http.StatusNotFound, "budget not found")
			return
		}
		log.Error().Err(err).Str("id", id.String()).Msg("failed to reset budget")
		writeError(w, http.StatusInternalServerError, "failed to reset budget")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"message": "budget reset successfully",
	})
}

// GetIdentity returns the authenticated user's identity.
func (h *Handler) GetIdentity(w http.ResponseWriter, r *http.Request) {
	identity := auth.GetIdentity(r.Context())
	if identity == nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"authenticated": false,
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"authenticated": true,
		"subject":       identity.Subject,
		"email":         identity.Email,
		"org_id":        identity.OrgID,
		"team_id":       identity.TeamID,
		"is_org":        identity.IsOrg,
	})
}

// Health returns the health status.
func (h *Handler) Health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{
		"status": "healthy",
	})
}

// Ready returns the readiness status.
func (h *Handler) Ready(w http.ResponseWriter, r *http.Request) {
	// Try to ping the database
	_, err := h.repo.ListBudgets(r.Context())
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"status": "not ready",
			"error":  err.Error(),
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"status": "ready",
	})
}

// ValidateCELRequest represents a CEL validation request.
type ValidateCELRequest struct {
	Expression string `json:"expression"`
}

// ValidateCEL validates a CEL expression.
func (h *Handler) ValidateCEL(w http.ResponseWriter, r *http.Request) {
	var req ValidateCELRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Expression == "" {
		writeError(w, http.StatusBadRequest, "expression is required")
		return
	}

	err := h.celEvaluator.ValidateExpression(req.Expression)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"valid": false,
			"error": err.Error(),
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"valid": true,
	})
}
