// Budget periods
export type BudgetPeriod = 'hourly' | 'daily' | 'weekly' | 'monthly' | 'custom';

// Entity types
export type EntityType = 'provider' | 'org' | 'team';

// Model Cost
export interface ModelCost {
  id: string;
  model_id: string;
  provider: string;
  input_cost_per_million: number;
  output_cost_per_million: number;
  cache_read_cost_million?: number;
  cache_write_cost_million?: number;
  model_pattern?: string;
  effective_date: string;
  created_at: string;
  updated_at: string;
}

export interface CreateModelCostRequest {
  model_id: string;
  provider: string;
  input_cost_per_million: number;
  output_cost_per_million: number;
  cache_read_cost_million?: number;
  cache_write_cost_million?: number;
  model_pattern?: string;
}

export interface UpdateModelCostRequest {
  provider?: string;
  input_cost_per_million?: number;
  output_cost_per_million?: number;
  cache_read_cost_million?: number;
  cache_write_cost_million?: number;
  model_pattern?: string;
}

// Budget Definition
export interface BudgetDefinition {
  id: string;
  entity_type: EntityType;
  name: string;
  match_expression: string;
  budget_amount_usd: number;
  period: BudgetPeriod;
  custom_period_seconds?: number;
  warning_threshold_pct: number;
  parent_id?: string;
  isolated: boolean;
  allow_fallback: boolean;
  enabled: boolean;
  current_period_start: string;
  current_usage_usd: number;
  pending_usage_usd: number;
  remaining_usd: number;
  next_period_start?: string;
  description?: string;
  owner_org_id?: string;
  owner_team_id?: string;
  version?: number;
  created_at: string;
  updated_at: string;
}

export interface CreateBudgetRequest {
  entity_type: EntityType;
  name: string;
  match_expression: string;
  budget_amount_usd: number;
  period: BudgetPeriod;
  custom_period_seconds?: number;
  warning_threshold_pct?: number;
  parent_id?: string;
  isolated?: boolean;
  allow_fallback?: boolean;
  enabled?: boolean;
  description?: string;
  owner_org_id?: string;
  owner_team_id?: string;
}

export interface UpdateBudgetRequest {
  match_expression?: string;
  budget_amount_usd?: number;
  period?: BudgetPeriod;
  custom_period_seconds?: number;
  warning_threshold_pct?: number;
  parent_id?: string;
  isolated?: boolean;
  allow_fallback?: boolean;
  enabled?: boolean;
  description?: string;
  version?: number;
}

// Usage Record
export interface UsageRecord {
  id: string;
  budget_id: string;
  request_id: string;
  model_id: string;
  input_tokens: number;
  output_tokens: number;
  cost_usd: number;
  parent_charged: boolean;
  created_at: string;
}

// API Responses
export interface ListModelCostsResponse {
  model_costs: ModelCost[];
}

export interface ListBudgetsResponse {
  budgets: BudgetDefinition[];
}

export interface UsageHistoryResponse {
  usage_records: UsageRecord[];
}

export interface ApiError {
  error: {
    message: string;
  };
}

// CEL Validation
export interface ValidateCELRequest {
  expression: string;
}

export interface ValidateCELResponse {
  valid: boolean;
  error?: string;
}

// Identity / Auth
export interface Identity {
  authenticated: boolean;
  subject?: string;
  email?: string;
  org_id?: string;
  team_id?: string;
  is_org?: boolean;
}
