package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/agentgateway/budget-management/internal/models"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

var (
	ErrNotFound          = errors.New("not found")
	ErrOptimisticLock    = errors.New("optimistic lock failed")
	ErrDuplicateEntity   = errors.New("duplicate entity")
)

// Repository provides database operations.
type Repository struct {
	db *DB
}

// NewRepository creates a new repository.
func NewRepository(db *DB) *Repository {
	return &Repository{db: db}
}

// Model Costs

// ListModelCosts returns all model costs.
func (r *Repository) ListModelCosts(ctx context.Context) ([]models.ModelCost, error) {
	query := `
		SELECT id, model_id, provider, input_cost_per_million, output_cost_per_million,
		       cache_read_cost_million, cache_write_cost_million, model_pattern,
		       effective_date, created_at, updated_at
		FROM model_costs
		ORDER BY provider, model_id
	`

	rows, err := r.db.Pool.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query model costs: %w", err)
	}
	defer rows.Close()

	var costs []models.ModelCost
	for rows.Next() {
		var mc models.ModelCost
		err := rows.Scan(
			&mc.ID, &mc.ModelID, &mc.Provider, &mc.InputCostPerMillion, &mc.OutputCostPerMillion,
			&mc.CacheReadCostMillion, &mc.CacheWriteCostMillion, &mc.ModelPattern,
			&mc.EffectiveDate, &mc.CreatedAt, &mc.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan model cost: %w", err)
		}
		costs = append(costs, mc)
	}

	return costs, nil
}

// GetModelCostByID returns a model cost by model ID.
func (r *Repository) GetModelCostByID(ctx context.Context, modelID string) (*models.ModelCost, error) {
	query := `
		SELECT id, model_id, provider, input_cost_per_million, output_cost_per_million,
		       cache_read_cost_million, cache_write_cost_million, model_pattern,
		       effective_date, created_at, updated_at
		FROM model_costs
		WHERE model_id = $1
	`

	var mc models.ModelCost
	err := r.db.Pool.QueryRow(ctx, query, modelID).Scan(
		&mc.ID, &mc.ModelID, &mc.Provider, &mc.InputCostPerMillion, &mc.OutputCostPerMillion,
		&mc.CacheReadCostMillion, &mc.CacheWriteCostMillion, &mc.ModelPattern,
		&mc.EffectiveDate, &mc.CreatedAt, &mc.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("failed to get model cost: %w", err)
	}

	return &mc, nil
}

// CreateModelCost creates a new model cost.
func (r *Repository) CreateModelCost(ctx context.Context, mc *models.ModelCost) error {
	mc.ID = uuid.New()
	mc.CreatedAt = time.Now()
	mc.UpdatedAt = time.Now()
	if mc.EffectiveDate.IsZero() {
		mc.EffectiveDate = time.Now()
	}

	query := `
		INSERT INTO model_costs (id, model_id, provider, input_cost_per_million, output_cost_per_million,
		                         cache_read_cost_million, cache_write_cost_million, model_pattern,
		                         effective_date, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
	`

	_, err := r.db.Pool.Exec(ctx, query,
		mc.ID, mc.ModelID, mc.Provider, mc.InputCostPerMillion, mc.OutputCostPerMillion,
		mc.CacheReadCostMillion, mc.CacheWriteCostMillion, mc.ModelPattern,
		mc.EffectiveDate, mc.CreatedAt, mc.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to create model cost: %w", err)
	}

	return nil
}

// UpdateModelCost updates a model cost.
func (r *Repository) UpdateModelCost(ctx context.Context, mc *models.ModelCost) error {
	query := `
		UPDATE model_costs
		SET provider = $2, input_cost_per_million = $3, output_cost_per_million = $4,
		    cache_read_cost_million = $5, cache_write_cost_million = $6,
		    model_pattern = $7, effective_date = $8
		WHERE model_id = $1
	`

	result, err := r.db.Pool.Exec(ctx, query,
		mc.ModelID, mc.Provider, mc.InputCostPerMillion, mc.OutputCostPerMillion,
		mc.CacheReadCostMillion, mc.CacheWriteCostMillion, mc.ModelPattern, mc.EffectiveDate,
	)
	if err != nil {
		return fmt.Errorf("failed to update model cost: %w", err)
	}

	if result.RowsAffected() == 0 {
		return ErrNotFound
	}

	return nil
}

// DeleteModelCost deletes a model cost by model ID.
func (r *Repository) DeleteModelCost(ctx context.Context, modelID string) error {
	query := `DELETE FROM model_costs WHERE model_id = $1`

	result, err := r.db.Pool.Exec(ctx, query, modelID)
	if err != nil {
		return fmt.Errorf("failed to delete model cost: %w", err)
	}

	if result.RowsAffected() == 0 {
		return ErrNotFound
	}

	return nil
}

// Budget Definitions

// ListBudgets returns all budget definitions.
func (r *Repository) ListBudgets(ctx context.Context) ([]models.BudgetDefinition, error) {
	query := `
		SELECT id, entity_type, name, match_expression, budget_amount_usd, period,
		       custom_period_seconds, warning_threshold_pct, parent_id, isolated,
		       allow_fallback, enabled, current_period_start, current_usage_usd, pending_usage_usd,
		       version, description, created_at, updated_at, owner_org_id, owner_team_id
		FROM budget_definitions
		ORDER BY entity_type, name
	`

	rows, err := r.db.Pool.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query budgets: %w", err)
	}
	defer rows.Close()

	var budgets []models.BudgetDefinition
	for rows.Next() {
		var b models.BudgetDefinition
		err := rows.Scan(
			&b.ID, &b.EntityType, &b.Name, &b.MatchExpression, &b.BudgetAmountUSD, &b.Period,
			&b.CustomPeriodSeconds, &b.WarningThresholdPct, &b.ParentID, &b.Isolated,
			&b.AllowFallback, &b.Enabled, &b.CurrentPeriodStart, &b.CurrentUsageUSD, &b.PendingUsageUSD,
			&b.Version, &b.Description, &b.CreatedAt, &b.UpdatedAt, &b.OwnerOrgID, &b.OwnerTeamID,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan budget: %w", err)
		}
		budgets = append(budgets, b)
	}

	return budgets, nil
}

// GetBudgetByID returns a budget by ID.
func (r *Repository) GetBudgetByID(ctx context.Context, id uuid.UUID) (*models.BudgetDefinition, error) {
	query := `
		SELECT id, entity_type, name, match_expression, budget_amount_usd, period,
		       custom_period_seconds, warning_threshold_pct, parent_id, isolated,
		       allow_fallback, enabled, current_period_start, current_usage_usd, pending_usage_usd,
		       version, description, created_at, updated_at, owner_org_id, owner_team_id
		FROM budget_definitions
		WHERE id = $1
	`

	var b models.BudgetDefinition
	err := r.db.Pool.QueryRow(ctx, query, id).Scan(
		&b.ID, &b.EntityType, &b.Name, &b.MatchExpression, &b.BudgetAmountUSD, &b.Period,
		&b.CustomPeriodSeconds, &b.WarningThresholdPct, &b.ParentID, &b.Isolated,
		&b.AllowFallback, &b.Enabled, &b.CurrentPeriodStart, &b.CurrentUsageUSD, &b.PendingUsageUSD,
		&b.Version, &b.Description, &b.CreatedAt, &b.UpdatedAt, &b.OwnerOrgID, &b.OwnerTeamID,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("failed to get budget: %w", err)
	}

	return &b, nil
}

// GetBudgetByEntity returns a budget by entity type and ID.
func (r *Repository) GetBudgetByEntity(ctx context.Context, entityType models.EntityType, entityID string) (*models.BudgetDefinition, error) {
	query := `
		SELECT id, entity_type, name, match_expression, budget_amount_usd, period,
		       custom_period_seconds, warning_threshold_pct, parent_id, isolated,
		       allow_fallback, enabled, current_period_start, current_usage_usd, pending_usage_usd,
		       version, description, created_at, updated_at, owner_org_id, owner_team_id
		FROM budget_definitions
		WHERE entity_type = $1 AND name = $2
	`

	var b models.BudgetDefinition
	err := r.db.Pool.QueryRow(ctx, query, entityType, entityID).Scan(
		&b.ID, &b.EntityType, &b.Name, &b.MatchExpression, &b.BudgetAmountUSD, &b.Period,
		&b.CustomPeriodSeconds, &b.WarningThresholdPct, &b.ParentID, &b.Isolated,
		&b.AllowFallback, &b.Enabled, &b.CurrentPeriodStart, &b.CurrentUsageUSD, &b.PendingUsageUSD,
		&b.Version, &b.Description, &b.CreatedAt, &b.UpdatedAt, &b.OwnerOrgID, &b.OwnerTeamID,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("failed to get budget by entity: %w", err)
	}

	return &b, nil
}

// CreateBudget creates a new budget definition.
func (r *Repository) CreateBudget(ctx context.Context, b *models.BudgetDefinition) error {
	b.ID = uuid.New()
	b.Version = 1
	b.CreatedAt = time.Now()
	b.UpdatedAt = time.Now()
	if b.CurrentPeriodStart.IsZero() {
		b.CurrentPeriodStart = time.Now()
	}
	if b.WarningThresholdPct == 0 {
		b.WarningThresholdPct = 80
	}

	query := `
		INSERT INTO budget_definitions (id, entity_type, name, match_expression, budget_amount_usd,
		                                 period, custom_period_seconds, warning_threshold_pct,
		                                 parent_id, isolated, allow_fallback, enabled, current_period_start,
		                                 current_usage_usd, pending_usage_usd, version, description,
		                                 created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19)
	`

	_, err := r.db.Pool.Exec(ctx, query,
		b.ID, b.EntityType, b.Name, b.MatchExpression, b.BudgetAmountUSD,
		b.Period, b.CustomPeriodSeconds, b.WarningThresholdPct,
		b.ParentID, b.Isolated, b.AllowFallback, b.Enabled, b.CurrentPeriodStart,
		b.CurrentUsageUSD, b.PendingUsageUSD, b.Version, b.Description,
		b.CreatedAt, b.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to create budget: %w", err)
	}

	return nil
}

// UpdateBudget updates a budget definition with optimistic locking.
func (r *Repository) UpdateBudget(ctx context.Context, b *models.BudgetDefinition) error {
	query := `
		UPDATE budget_definitions
		SET match_expression = $2, budget_amount_usd = $3, period = $4,
		    custom_period_seconds = $5, warning_threshold_pct = $6,
		    parent_id = $7, isolated = $8, allow_fallback = $9,
		    enabled = $10, description = $11, owner_org_id = $12,
		    owner_team_id = $13, version = version + 1
		WHERE id = $1 AND version = $14
	`

	result, err := r.db.Pool.Exec(ctx, query,
		b.ID, b.MatchExpression, b.BudgetAmountUSD, b.Period,
		b.CustomPeriodSeconds, b.WarningThresholdPct,
		b.ParentID, b.Isolated, b.AllowFallback, b.Enabled, b.Description,
		b.OwnerOrgID, b.OwnerTeamID, b.Version,
	)
	if err != nil {
		return fmt.Errorf("failed to update budget: %w", err)
	}

	if result.RowsAffected() == 0 {
		return ErrOptimisticLock
	}

	return nil
}

// DeleteBudget deletes a budget by ID.
func (r *Repository) DeleteBudget(ctx context.Context, id uuid.UUID) error {
	query := `DELETE FROM budget_definitions WHERE id = $1`

	result, err := r.db.Pool.Exec(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to delete budget: %w", err)
	}

	if result.RowsAffected() == 0 {
		return ErrNotFound
	}

	return nil
}

// IncrementBudgetUsage increments the usage for a budget with optimistic locking.
func (r *Repository) IncrementBudgetUsage(ctx context.Context, id uuid.UUID, costUSD float64, expectedVersion int64) error {
	query := `
		UPDATE budget_definitions
		SET current_usage_usd = current_usage_usd + $2, version = version + 1
		WHERE id = $1 AND version = $3
	`

	result, err := r.db.Pool.Exec(ctx, query, id, costUSD, expectedVersion)
	if err != nil {
		return fmt.Errorf("failed to increment budget usage: %w", err)
	}

	if result.RowsAffected() == 0 {
		return ErrOptimisticLock
	}

	return nil
}

// ResetBudgetUsage resets the usage for a budget.
func (r *Repository) ResetBudgetUsage(ctx context.Context, id uuid.UUID) error {
	query := `
		UPDATE budget_definitions
		SET current_usage_usd = 0, pending_usage_usd = 0, current_period_start = NOW(), version = version + 1
		WHERE id = $1
	`

	result, err := r.db.Pool.Exec(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to reset budget usage: %w", err)
	}

	if result.RowsAffected() == 0 {
		return ErrNotFound
	}

	return nil
}

// ResetExpiredBudgets resets budgets whose period has expired.
func (r *Repository) ResetExpiredBudgets(ctx context.Context) (int64, error) {
	// Reset hourly budgets
	hourlyQuery := `
		UPDATE budget_definitions
		SET current_usage_usd = 0, pending_usage_usd = 0,
		    current_period_start = NOW(), version = version + 1
		WHERE period = 'hourly' AND current_period_start + INTERVAL '1 hour' <= NOW()
	`

	// Reset daily budgets
	dailyQuery := `
		UPDATE budget_definitions
		SET current_usage_usd = 0, pending_usage_usd = 0,
		    current_period_start = NOW(), version = version + 1
		WHERE period = 'daily' AND current_period_start + INTERVAL '1 day' <= NOW()
	`

	// Reset weekly budgets
	weeklyQuery := `
		UPDATE budget_definitions
		SET current_usage_usd = 0, pending_usage_usd = 0,
		    current_period_start = NOW(), version = version + 1
		WHERE period = 'weekly' AND current_period_start + INTERVAL '7 days' <= NOW()
	`

	// Reset monthly budgets
	monthlyQuery := `
		UPDATE budget_definitions
		SET current_usage_usd = 0, pending_usage_usd = 0,
		    current_period_start = NOW(), version = version + 1
		WHERE period = 'monthly' AND current_period_start + INTERVAL '1 month' <= NOW()
	`

	// Reset custom period budgets
	customQuery := `
		UPDATE budget_definitions
		SET current_usage_usd = 0, pending_usage_usd = 0,
		    current_period_start = NOW(), version = version + 1
		WHERE period = 'custom' AND custom_period_seconds IS NOT NULL
		  AND current_period_start + (custom_period_seconds || ' seconds')::INTERVAL <= NOW()
	`

	var total int64

	for _, q := range []string{hourlyQuery, dailyQuery, weeklyQuery, monthlyQuery, customQuery} {
		result, err := r.db.Pool.Exec(ctx, q)
		if err != nil {
			return total, fmt.Errorf("failed to reset expired budgets: %w", err)
		}
		total += result.RowsAffected()
	}

	return total, nil
}

// GetBudgetsWithParent returns a budget and all its ancestors.
func (r *Repository) GetBudgetsWithParent(ctx context.Context, id uuid.UUID) ([]models.BudgetDefinition, error) {
	query := `
		WITH RECURSIVE budget_hierarchy AS (
			SELECT id, entity_type, name, match_expression, budget_amount_usd, period,
			       custom_period_seconds, warning_threshold_pct, parent_id, isolated,
			       allow_fallback, enabled, current_period_start, current_usage_usd, pending_usage_usd,
			       version, description, created_at, updated_at, owner_org_id, owner_team_id, 0 as depth
			FROM budget_definitions
			WHERE id = $1

			UNION ALL

			SELECT bd.id, bd.entity_type, bd.name, bd.match_expression, bd.budget_amount_usd, bd.period,
			       bd.custom_period_seconds, bd.warning_threshold_pct, bd.parent_id, bd.isolated,
			       bd.allow_fallback, bd.enabled, bd.current_period_start, bd.current_usage_usd, bd.pending_usage_usd,
			       bd.version, bd.description, bd.created_at, bd.updated_at, bd.owner_org_id, bd.owner_team_id, bh.depth + 1
			FROM budget_definitions bd
			INNER JOIN budget_hierarchy bh ON bd.id = bh.parent_id
		)
		SELECT id, entity_type, name, match_expression, budget_amount_usd, period,
		       custom_period_seconds, warning_threshold_pct, parent_id, isolated,
		       allow_fallback, enabled, current_period_start, current_usage_usd, pending_usage_usd,
		       version, description, created_at, updated_at, owner_org_id, owner_team_id
		FROM budget_hierarchy
		ORDER BY depth
	`

	rows, err := r.db.Pool.Query(ctx, query, id)
	if err != nil {
		return nil, fmt.Errorf("failed to query budget hierarchy: %w", err)
	}
	defer rows.Close()

	var budgets []models.BudgetDefinition
	for rows.Next() {
		var b models.BudgetDefinition
		err := rows.Scan(
			&b.ID, &b.EntityType, &b.Name, &b.MatchExpression, &b.BudgetAmountUSD, &b.Period,
			&b.CustomPeriodSeconds, &b.WarningThresholdPct, &b.ParentID, &b.Isolated,
			&b.AllowFallback, &b.Enabled, &b.CurrentPeriodStart, &b.CurrentUsageUSD, &b.PendingUsageUSD,
			&b.Version, &b.Description, &b.CreatedAt, &b.UpdatedAt, &b.OwnerOrgID, &b.OwnerTeamID,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan budget: %w", err)
		}
		budgets = append(budgets, b)
	}

	return budgets, nil
}

// Reservations

// CreateReservation creates a new request reservation.
func (r *Repository) CreateReservation(ctx context.Context, res *models.RequestReservation) error {
	res.ID = uuid.New()
	res.CreatedAt = time.Now()

	query := `
		INSERT INTO request_reservations (id, budget_id, request_id, estimated_cost_usd, expires_at, created_at)
		VALUES ($1, $2, $3, $4, $5, $6)
	`

	_, err := r.db.Pool.Exec(ctx, query,
		res.ID, res.BudgetID, res.RequestID, res.EstimatedCostUSD, res.ExpiresAt, res.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to create reservation: %w", err)
	}

	// Update pending usage
	updateQuery := `
		UPDATE budget_definitions
		SET pending_usage_usd = pending_usage_usd + $2
		WHERE id = $1
	`
	_, err = r.db.Pool.Exec(ctx, updateQuery, res.BudgetID, res.EstimatedCostUSD)
	if err != nil {
		return fmt.Errorf("failed to update pending usage: %w", err)
	}

	return nil
}

// GetReservationByRequestID returns a reservation by request ID.
func (r *Repository) GetReservationByRequestID(ctx context.Context, requestID string) (*models.RequestReservation, error) {
	query := `
		SELECT id, budget_id, request_id, estimated_cost_usd, expires_at, created_at
		FROM request_reservations
		WHERE request_id = $1
	`

	var res models.RequestReservation
	err := r.db.Pool.QueryRow(ctx, query, requestID).Scan(
		&res.ID, &res.BudgetID, &res.RequestID, &res.EstimatedCostUSD, &res.ExpiresAt, &res.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("failed to get reservation: %w", err)
	}

	return &res, nil
}

// DeleteReservation deletes a reservation and updates pending usage.
func (r *Repository) DeleteReservation(ctx context.Context, requestID string) error {
	// Get reservation first
	res, err := r.GetReservationByRequestID(ctx, requestID)
	if err != nil {
		return err
	}

	// Delete reservation
	deleteQuery := `DELETE FROM request_reservations WHERE request_id = $1`
	_, err = r.db.Pool.Exec(ctx, deleteQuery, requestID)
	if err != nil {
		return fmt.Errorf("failed to delete reservation: %w", err)
	}

	// Update pending usage
	updateQuery := `
		UPDATE budget_definitions
		SET pending_usage_usd = GREATEST(0, pending_usage_usd - $2)
		WHERE id = $1
	`
	_, err = r.db.Pool.Exec(ctx, updateQuery, res.BudgetID, res.EstimatedCostUSD)
	if err != nil {
		return fmt.Errorf("failed to update pending usage: %w", err)
	}

	return nil
}

// CleanupExpiredReservations removes expired reservations.
func (r *Repository) CleanupExpiredReservations(ctx context.Context) (int64, error) {
	// Get expired reservations
	selectQuery := `
		SELECT budget_id, SUM(estimated_cost_usd) as total_cost
		FROM request_reservations
		WHERE expires_at <= NOW()
		GROUP BY budget_id
	`

	rows, err := r.db.Pool.Query(ctx, selectQuery)
	if err != nil {
		return 0, fmt.Errorf("failed to query expired reservations: %w", err)
	}
	defer rows.Close()

	type budgetCost struct {
		BudgetID uuid.UUID
		Cost     float64
	}

	var budgetCosts []budgetCost
	for rows.Next() {
		var bc budgetCost
		if err := rows.Scan(&bc.BudgetID, &bc.Cost); err != nil {
			return 0, fmt.Errorf("failed to scan budget cost: %w", err)
		}
		budgetCosts = append(budgetCosts, bc)
	}

	// Delete expired reservations
	deleteQuery := `DELETE FROM request_reservations WHERE expires_at <= NOW()`
	result, err := r.db.Pool.Exec(ctx, deleteQuery)
	if err != nil {
		return 0, fmt.Errorf("failed to delete expired reservations: %w", err)
	}

	// Update pending usage for affected budgets
	for _, bc := range budgetCosts {
		updateQuery := `
			UPDATE budget_definitions
			SET pending_usage_usd = GREATEST(0, pending_usage_usd - $2)
			WHERE id = $1
		`
		_, err = r.db.Pool.Exec(ctx, updateQuery, bc.BudgetID, bc.Cost)
		if err != nil {
			return result.RowsAffected(), fmt.Errorf("failed to update pending usage: %w", err)
		}
	}

	return result.RowsAffected(), nil
}

// Usage Records

// CreateUsageRecord creates a usage record.
func (r *Repository) CreateUsageRecord(ctx context.Context, ur *models.UsageRecord) error {
	ur.ID = uuid.New()
	ur.CreatedAt = time.Now()

	query := `
		INSERT INTO usage_records (id, budget_id, request_id, model_id, input_tokens, output_tokens, cost_usd, parent_charged, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`

	_, err := r.db.Pool.Exec(ctx, query,
		ur.ID, ur.BudgetID, ur.RequestID, ur.ModelID, ur.InputTokens, ur.OutputTokens, ur.CostUSD, ur.ParentCharged, ur.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to create usage record: %w", err)
	}

	return nil
}

// GetUsageByBudgetID returns usage records for a budget.
func (r *Repository) GetUsageByBudgetID(ctx context.Context, budgetID uuid.UUID, since time.Time, limit int) ([]models.UsageRecord, error) {
	query := `
		SELECT id, budget_id, request_id, model_id, input_tokens, output_tokens, cost_usd, parent_charged, created_at
		FROM usage_records
		WHERE budget_id = $1 AND created_at >= $2
		ORDER BY created_at DESC
		LIMIT $3
	`

	rows, err := r.db.Pool.Query(ctx, query, budgetID, since, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query usage records: %w", err)
	}
	defer rows.Close()

	var records []models.UsageRecord
	for rows.Next() {
		var ur models.UsageRecord
		err := rows.Scan(
			&ur.ID, &ur.BudgetID, &ur.RequestID, &ur.ModelID, &ur.InputTokens, &ur.OutputTokens, &ur.CostUSD, &ur.ParentCharged, &ur.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan usage record: %w", err)
		}
		records = append(records, ur)
	}

	return records, nil
}

// GetBudgetForUpdate returns a budget with a row lock for update.
func (r *Repository) GetBudgetForUpdate(ctx context.Context, tx pgx.Tx, id uuid.UUID) (*models.BudgetDefinition, error) {
	query := `
		SELECT id, entity_type, name, match_expression, budget_amount_usd, period,
		       custom_period_seconds, warning_threshold_pct, parent_id, isolated,
		       allow_fallback, enabled, current_period_start, current_usage_usd, pending_usage_usd,
		       version, description, created_at, updated_at, owner_org_id, owner_team_id
		FROM budget_definitions
		WHERE id = $1
		FOR UPDATE
	`

	var b models.BudgetDefinition
	err := tx.QueryRow(ctx, query, id).Scan(
		&b.ID, &b.EntityType, &b.Name, &b.MatchExpression, &b.BudgetAmountUSD, &b.Period,
		&b.CustomPeriodSeconds, &b.WarningThresholdPct, &b.ParentID, &b.Isolated,
		&b.AllowFallback, &b.Enabled, &b.CurrentPeriodStart, &b.CurrentUsageUSD, &b.PendingUsageUSD,
		&b.Version, &b.Description, &b.CreatedAt, &b.UpdatedAt, &b.OwnerOrgID, &b.OwnerTeamID,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("failed to get budget for update: %w", err)
	}

	return &b, nil
}

// BeginTx starts a transaction.
func (r *Repository) BeginTx(ctx context.Context) (pgx.Tx, error) {
	return r.db.Pool.Begin(ctx)
}

// GetAllBudgets returns all budgets for CEL matching.
func (r *Repository) GetAllBudgets(ctx context.Context) ([]models.BudgetDefinition, error) {
	return r.ListBudgets(ctx)
}

// GetEnabledBudgets returns only enabled budgets for enforcement.
func (r *Repository) GetEnabledBudgets(ctx context.Context) ([]models.BudgetDefinition, error) {
	query := `
		SELECT id, entity_type, name, match_expression, budget_amount_usd, period,
		       custom_period_seconds, warning_threshold_pct, parent_id, isolated,
		       allow_fallback, enabled, current_period_start, current_usage_usd, pending_usage_usd,
		       version, description, created_at, updated_at, owner_org_id, owner_team_id
		FROM budget_definitions
		WHERE enabled = true
		ORDER BY entity_type, name
	`

	rows, err := r.db.Pool.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query enabled budgets: %w", err)
	}
	defer rows.Close()

	var budgets []models.BudgetDefinition
	for rows.Next() {
		var b models.BudgetDefinition
		err := rows.Scan(
			&b.ID, &b.EntityType, &b.Name, &b.MatchExpression, &b.BudgetAmountUSD, &b.Period,
			&b.CustomPeriodSeconds, &b.WarningThresholdPct, &b.ParentID, &b.Isolated,
			&b.AllowFallback, &b.Enabled, &b.CurrentPeriodStart, &b.CurrentUsageUSD, &b.PendingUsageUSD,
			&b.Version, &b.Description, &b.CreatedAt, &b.UpdatedAt, &b.OwnerOrgID, &b.OwnerTeamID,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan budget: %w", err)
		}
		budgets = append(budgets, b)
	}

	return budgets, nil
}

// IncrementUsageInTx increments budget usage within a transaction.
func (r *Repository) IncrementUsageInTx(ctx context.Context, tx pgx.Tx, id uuid.UUID, costUSD float64) error {
	query := `
		UPDATE budget_definitions
		SET current_usage_usd = current_usage_usd + $2, version = version + 1
		WHERE id = $1
	`

	_, err := tx.Exec(ctx, query, id, costUSD)
	if err != nil {
		return fmt.Errorf("failed to increment budget usage: %w", err)
	}

	return nil
}

// DecrementPendingUsageInTx decrements pending usage within a transaction.
func (r *Repository) DecrementPendingUsageInTx(ctx context.Context, tx pgx.Tx, id uuid.UUID, costUSD float64) error {
	query := `
		UPDATE budget_definitions
		SET pending_usage_usd = GREATEST(0, pending_usage_usd - $2)
		WHERE id = $1
	`

	_, err := tx.Exec(ctx, query, id, costUSD)
	if err != nil {
		return fmt.Errorf("failed to decrement pending usage: %w", err)
	}

	return nil
}

// DeleteReservationInTx deletes a reservation within a transaction.
func (r *Repository) DeleteReservationInTx(ctx context.Context, tx pgx.Tx, requestID string) error {
	query := `DELETE FROM request_reservations WHERE request_id = $1`
	_, err := tx.Exec(ctx, query, requestID)
	if err != nil {
		return fmt.Errorf("failed to delete reservation: %w", err)
	}
	return nil
}

// CreateUsageRecordInTx creates a usage record within a transaction.
func (r *Repository) CreateUsageRecordInTx(ctx context.Context, tx pgx.Tx, ur *models.UsageRecord) error {
	ur.ID = uuid.New()
	ur.CreatedAt = time.Now()

	query := `
		INSERT INTO usage_records (id, budget_id, request_id, model_id, input_tokens, output_tokens, cost_usd, parent_charged, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`

	_, err := tx.Exec(ctx, query,
		ur.ID, ur.BudgetID, ur.RequestID, ur.ModelID, ur.InputTokens, ur.OutputTokens, ur.CostUSD, ur.ParentCharged, ur.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to create usage record: %w", err)
	}

	return nil
}

// Avoid unused import error
var _ = sql.ErrNoRows
