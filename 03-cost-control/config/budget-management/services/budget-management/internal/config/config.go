package config

import (
	"time"

	"github.com/kelseyhightower/envconfig"
)

// Config holds all configuration for the budget limiter server.
type Config struct {
	// Server configuration
	GRPCPort    int `envconfig:"GRPC_PORT" default:"4444"`
	HTTPPort    int `envconfig:"HTTP_PORT" default:"8080"`
	MetricsPort int `envconfig:"METRICS_PORT" default:"9090"`

	// Database configuration
	DatabaseURL string `envconfig:"DATABASE_URL" required:"true"`

	// Cache configuration
	ModelCostCacheTTL  time.Duration `envconfig:"MODEL_COST_CACHE_TTL" default:"60s"`
	BudgetCacheTTL     time.Duration `envconfig:"BUDGET_CACHE_TTL" default:"30s"`

	// Reservation configuration
	ReservationTTL     time.Duration `envconfig:"RESERVATION_TTL" default:"5m"`
	ReservationCleanup time.Duration `envconfig:"RESERVATION_CLEANUP" default:"1m"`

	// Period reset check interval
	PeriodResetInterval time.Duration `envconfig:"PERIOD_RESET_INTERVAL" default:"1m"`

	// Default cost estimation multiplier (for pre-request budget checks)
	DefaultEstimationMultiplier float64 `envconfig:"DEFAULT_ESTIMATION_MULTIPLIER" default:"1.5"`

	// Default estimated tokens per request (when we can't parse the request)
	DefaultEstimatedInputTokens  int64 `envconfig:"DEFAULT_ESTIMATED_INPUT_TOKENS" default:"1000"`
	DefaultEstimatedOutputTokens int64 `envconfig:"DEFAULT_ESTIMATED_OUTPUT_TOKENS" default:"1000"`

	// Log level
	LogLevel string `envconfig:"LOG_LEVEL" default:"info"`

	// Header names for entity identification (Option A from RFE)
	OrgIDHeader  string `envconfig:"ORG_ID_HEADER" default:"x-org-id"`
	TeamIDHeader string `envconfig:"TEAM_ID_HEADER" default:"x-team-id"`
	UserIDHeader string `envconfig:"USER_ID_HEADER" default:"x-user-id"`

	// Auth configuration
	AuthEnabled bool `envconfig:"AUTH_ENABLED" default:"false"`
}

// Load loads configuration from environment variables.
func Load() (*Config, error) {
	var cfg Config
	if err := envconfig.Process("", &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}
