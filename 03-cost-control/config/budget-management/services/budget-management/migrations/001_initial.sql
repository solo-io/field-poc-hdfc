-- Budget Rate Limit Schema
-- Version: 001_initial

-- Enable UUID extension
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

-- Budget period enum
CREATE TYPE budget_period AS ENUM ('hourly', 'daily', 'weekly', 'monthly', 'custom');

-- Entity type enum
CREATE TYPE entity_type AS ENUM ('provider', 'org', 'team');

-- Model costs table
CREATE TABLE model_costs (
    id                      UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    model_id                VARCHAR(255) NOT NULL UNIQUE,
    provider                VARCHAR(100) NOT NULL,
    input_cost_per_million  DECIMAL(20, 10) NOT NULL,
    output_cost_per_million DECIMAL(20, 10) NOT NULL,
    cache_read_cost_million DECIMAL(20, 10),
    cache_write_cost_million DECIMAL(20, 10),
    model_pattern           VARCHAR(255),
    effective_date          TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    created_at              TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at              TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

-- Create index on model_id for fast lookups
CREATE INDEX idx_model_costs_model_id ON model_costs(model_id);
CREATE INDEX idx_model_costs_provider ON model_costs(provider);

-- Budget definitions table
CREATE TABLE budget_definitions (
    id                      UUID PRIMARY KEY DEFAULT uuid_generate_v4(),

    -- Entity identification
    entity_type             entity_type NOT NULL,
    name                    VARCHAR(255) NOT NULL,

    -- CEL expression for matching requests
    match_expression        TEXT NOT NULL,

    -- Budget configuration
    budget_amount_usd       DECIMAL(20, 10) NOT NULL,
    period                  budget_period NOT NULL,
    custom_period_seconds   INTEGER,
    warning_threshold_pct   INTEGER NOT NULL DEFAULT 80,

    -- Hierarchy
    parent_id               UUID REFERENCES budget_definitions(id),
    isolated                BOOLEAN NOT NULL DEFAULT true,
    allow_fallback          BOOLEAN NOT NULL DEFAULT false,

    -- Status
    enabled                 BOOLEAN NOT NULL DEFAULT true,

    -- Current state
    current_period_start    TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    current_usage_usd       DECIMAL(20, 10) NOT NULL DEFAULT 0,
    pending_usage_usd       DECIMAL(20, 10) NOT NULL DEFAULT 0,

    -- Optimistic locking
    version                 BIGINT NOT NULL DEFAULT 1,

    -- Metadata
    description             TEXT,

    -- Ownership for RBAC
    owner_org_id            VARCHAR(255),
    owner_team_id           VARCHAR(255),

    created_at              TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at              TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),

    CONSTRAINT unique_entity UNIQUE (entity_type, name)
);

-- Create indexes for budget lookups
CREATE INDEX idx_budget_definitions_entity ON budget_definitions(entity_type, name);
CREATE INDEX idx_budget_definitions_parent ON budget_definitions(parent_id);
CREATE INDEX idx_budget_definitions_enabled ON budget_definitions(enabled);
CREATE INDEX idx_budget_definitions_owner_org ON budget_definitions(owner_org_id);
CREATE INDEX idx_budget_definitions_owner_team ON budget_definitions(owner_team_id);

-- Usage records table
CREATE TABLE usage_records (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    budget_id       UUID NOT NULL REFERENCES budget_definitions(id) ON DELETE CASCADE,
    request_id      VARCHAR(255) NOT NULL,
    model_id        VARCHAR(255) NOT NULL,
    input_tokens    BIGINT NOT NULL,
    output_tokens   BIGINT NOT NULL,
    cost_usd        DECIMAL(20, 10) NOT NULL,
    parent_charged  BOOLEAN NOT NULL DEFAULT true,
    created_at      TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

-- Create indexes for usage queries
CREATE INDEX idx_usage_records_budget_id ON usage_records(budget_id);
CREATE INDEX idx_usage_records_created_at ON usage_records(created_at);
CREATE INDEX idx_usage_records_request_id ON usage_records(request_id);

-- Request reservations table
CREATE TABLE request_reservations (
    id                  UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    budget_id           UUID NOT NULL REFERENCES budget_definitions(id) ON DELETE CASCADE,
    request_id          VARCHAR(255) NOT NULL,
    estimated_cost_usd  DECIMAL(20, 10) NOT NULL,
    expires_at          TIMESTAMP WITH TIME ZONE NOT NULL,
    created_at          TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    UNIQUE (budget_id, request_id)
);

-- Create indexes for reservation management
CREATE INDEX idx_request_reservations_budget_id ON request_reservations(budget_id);
CREATE INDEX idx_request_reservations_expires_at ON request_reservations(expires_at);
CREATE INDEX idx_request_reservations_request_id ON request_reservations(request_id);

-- Function to update updated_at timestamp
CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ language 'plpgsql';

-- Triggers for updated_at
CREATE TRIGGER update_model_costs_updated_at
    BEFORE UPDATE ON model_costs
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_budget_definitions_updated_at
    BEFORE UPDATE ON budget_definitions
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

-- Insert default model costs for common models
INSERT INTO model_costs (model_id, provider, input_cost_per_million, output_cost_per_million) VALUES
    -- OpenAI models
    ('gpt-4.1', 'openai', 2.00, 8.00),
    ('gpt-4.1-mini', 'openai', 0.40, 1.60),
    ('gpt-4.1-nano', 'openai', 0.10, 0.40),
    ('gpt-4o', 'openai', 2.50, 10.00),
    ('gpt-4o-mini', 'openai', 0.15, 0.60),
    ('gpt-4-turbo', 'openai', 10.00, 30.00),
    ('gpt-4', 'openai', 30.00, 60.00),
    ('gpt-3.5-turbo', 'openai', 0.50, 1.50),
    ('o3', 'openai', 10.00, 40.00),
    ('o3-mini', 'openai', 1.10, 4.40),
    ('o4-mini', 'openai', 1.10, 4.40),
    ('o1', 'openai', 15.00, 60.00),
    ('o1-mini', 'openai', 3.00, 12.00),
    ('o1-pro', 'openai', 150.00, 600.00),

    -- Anthropic models
    ('claude-opus-4-5-20250415', 'anthropic', 15.00, 75.00),
    ('claude-sonnet-4-5-20250415', 'anthropic', 3.00, 15.00),
    ('claude-sonnet-4-20250514', 'anthropic', 3.00, 15.00),
    ('claude-haiku-4-5-20251001', 'anthropic', 1.00, 5.00),
    ('claude-3-5-sonnet-20241022', 'anthropic', 3.00, 15.00),
    ('claude-3-5-haiku-20241022', 'anthropic', 0.80, 4.00),
    ('claude-3-opus-20240229', 'anthropic', 15.00, 75.00),
    ('claude-3-sonnet-20240229', 'anthropic', 3.00, 15.00),
    ('claude-3-haiku-20240307', 'anthropic', 0.25, 1.25),

    -- Google models
    ('gemini-2.5-pro', 'google', 1.25, 10.00),
    ('gemini-2.5-flash', 'google', 0.15, 0.60),
    ('gemini-2.0-pro', 'google', 1.25, 5.00),
    ('gemini-2.0-flash', 'google', 0.10, 0.40),
    ('gemini-2.0-flash-lite', 'google', 0.075, 0.30),
    ('gemini-1.5-pro', 'google', 1.25, 5.00),
    ('gemini-1.5-flash', 'google', 0.075, 0.30),

    -- Mistral models
    ('mistral-large', 'mistral', 2.00, 6.00),
    ('mistral-medium', 'mistral', 2.70, 8.10),
    ('mistral-small', 'mistral', 0.20, 0.60),

    -- AWS Nova models
    ('amazon.nova-micro-v1:0', 'aws', 0.035, 0.14),
    ('amazon.nova-lite-v1:0', 'aws', 0.06, 0.24),
    ('amazon.nova-pro-v1:0', 'aws', 0.80, 3.20);
