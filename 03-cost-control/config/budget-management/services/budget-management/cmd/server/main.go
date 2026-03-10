package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/agentgateway/budget-management/internal/api"
	"github.com/agentgateway/budget-management/internal/budget"
	"github.com/agentgateway/budget-management/internal/cel"
	"github.com/agentgateway/budget-management/internal/config"
	"github.com/agentgateway/budget-management/internal/db"
	"github.com/agentgateway/budget-management/internal/extproc"
	"github.com/agentgateway/budget-management/internal/metrics"
	"github.com/gorilla/mux"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"
)

func main() {
	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		log.Fatal().Err(err).Msg("failed to load configuration")
	}

	// Configure logging
	configureLogging(cfg.LogLevel)

	log.Info().
		Int("grpc_port", cfg.GRPCPort).
		Int("http_port", cfg.HTTPPort).
		Int("metrics_port", cfg.MetricsPort).
		Msg("starting budget management server")

	// Create context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Connect to database
	database, err := db.New(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to connect to database")
	}
	defer database.Close()

	log.Info().Msg("connected to database")

	// Create repository
	repo := db.NewRepository(database)

	// Create CEL evaluator
	celEvaluator, err := cel.NewEvaluator()
	if err != nil {
		log.Fatal().Err(err).Msg("failed to create CEL evaluator")
	}

	// Create budget service
	budgetSvc := budget.NewService(repo, celEvaluator, cfg)

	// Refresh model cost cache
	if err := budgetSvc.RefreshModelCostCache(ctx); err != nil {
		log.Warn().Err(err).Msg("failed to refresh model cost cache")
	}

	// Initialize budget metrics from database
	if err := budgetSvc.RefreshBudgetMetrics(ctx); err != nil {
		log.Warn().Err(err).Msg("failed to initialize budget metrics")
	}

	// Create ext_proc server
	extprocServer := extproc.NewServer(budgetSvc, celEvaluator, cfg)

	// Create gRPC server
	grpcServer := grpc.NewServer()
	extprocServer.Register(grpcServer)

	// Register health service
	healthServer := health.NewServer()
	grpc_health_v1.RegisterHealthServer(grpcServer, healthServer)
	healthServer.SetServingStatus("", grpc_health_v1.HealthCheckResponse_SERVING)

	// Enable reflection for debugging
	reflection.Register(grpcServer)

	// Create HTTP server for management API and UI
	router := mux.NewRouter()
	apiHandler := api.NewHandler(repo, celEvaluator)
	apiHandler.RegisterRoutes(router)

	// Serve static UI files
	uiPath := "/app/ui"
	if envUIPath := os.Getenv("UI_PATH"); envUIPath != "" {
		uiPath = envUIPath
	}
	if _, err := os.Stat(uiPath); err == nil {
		// Serve static files
		fs := http.FileServer(http.Dir(uiPath))
		router.PathPrefix("/").Handler(spaHandler{staticPath: uiPath, indexPath: "index.html", fileServer: fs})
		log.Info().Str("path", uiPath).Msg("serving UI static files")
	}

	// Wrap router with CORS middleware
	corsHandler := enableCORS(router)

	httpServer := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.HTTPPort),
		Handler:      corsHandler,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	// Start background workers
	go runBackgroundWorkers(ctx, budgetSvc, cfg)

	// Start gRPC server
	grpcAddr := fmt.Sprintf(":%d", cfg.GRPCPort)
	grpcListener, err := net.Listen("tcp", grpcAddr)
	if err != nil {
		log.Fatal().Err(err).Str("addr", grpcAddr).Msg("failed to listen")
	}

	go func() {
		log.Info().Str("addr", grpcAddr).Msg("starting gRPC server")
		if err := grpcServer.Serve(grpcListener); err != nil {
			log.Fatal().Err(err).Msg("gRPC server failed")
		}
	}()

	// Start HTTP server
	go func() {
		log.Info().Str("addr", httpServer.Addr).Msg("starting HTTP server")
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal().Err(err).Msg("HTTP server failed")
		}
	}()

	// Create and start metrics server
	metricsServer := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.MetricsPort),
		Handler:      metrics.Handler(),
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	go func() {
		log.Info().Str("addr", metricsServer.Addr).Msg("starting metrics server")
		if err := metricsServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal().Err(err).Msg("metrics server failed")
		}
	}()

	// Wait for shutdown signal
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	sig := <-sigCh
	log.Info().Str("signal", sig.String()).Msg("received shutdown signal")

	// Graceful shutdown
	cancel()

	// Stop accepting new connections
	healthServer.SetServingStatus("", grpc_health_v1.HealthCheckResponse_NOT_SERVING)

	// Shutdown HTTP server
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		log.Error().Err(err).Msg("HTTP server shutdown error")
	}

	// Shutdown metrics server
	if err := metricsServer.Shutdown(shutdownCtx); err != nil {
		log.Error().Err(err).Msg("metrics server shutdown error")
	}

	// Stop gRPC server
	grpcServer.GracefulStop()

	log.Info().Msg("server stopped")
}

func configureLogging(level string) {
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix

	switch level {
	case "debug":
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
	case "info":
		zerolog.SetGlobalLevel(zerolog.InfoLevel)
	case "warn":
		zerolog.SetGlobalLevel(zerolog.WarnLevel)
	case "error":
		zerolog.SetGlobalLevel(zerolog.ErrorLevel)
	default:
		zerolog.SetGlobalLevel(zerolog.InfoLevel)
	}

	// Use console writer for development
	if os.Getenv("LOG_FORMAT") != "json" {
		log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})
	}
}

// enableCORS adds CORS headers to allow UI requests
func enableCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// spaHandler serves the SPA and handles client-side routing
type spaHandler struct {
	staticPath string
	indexPath  string
	fileServer http.Handler
}

func (h spaHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	// Check if the file exists
	fullPath := h.staticPath + path
	_, err := os.Stat(fullPath)
	if os.IsNotExist(err) {
		// File doesn't exist, serve index.html for SPA routing
		http.ServeFile(w, r, h.staticPath+"/"+h.indexPath)
		return
	} else if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	// File exists, serve it
	h.fileServer.ServeHTTP(w, r)
}

func runBackgroundWorkers(ctx context.Context, budgetSvc *budget.Service, cfg *config.Config) {
	// Period reset ticker
	periodResetTicker := time.NewTicker(cfg.PeriodResetInterval)
	defer periodResetTicker.Stop()

	// Reservation cleanup ticker
	reservationCleanupTicker := time.NewTicker(cfg.ReservationCleanup)
	defer reservationCleanupTicker.Stop()

	// Model cost cache refresh ticker
	modelCostRefreshTicker := time.NewTicker(cfg.ModelCostCacheTTL)
	defer modelCostRefreshTicker.Stop()

	// Budget metrics refresh ticker (every 30 seconds)
	budgetMetricsTicker := time.NewTicker(30 * time.Second)
	defer budgetMetricsTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			return

		case <-periodResetTicker.C:
			count, err := budgetSvc.ResetExpiredBudgets(ctx)
			if err != nil {
				log.Error().Err(err).Msg("failed to reset expired budgets")
			} else if count > 0 {
				log.Info().Int64("count", count).Msg("reset expired budgets")
			}

		case <-reservationCleanupTicker.C:
			count, err := budgetSvc.CleanupExpiredReservations(ctx)
			if err != nil {
				log.Error().Err(err).Msg("failed to cleanup expired reservations")
			} else if count > 0 {
				log.Info().Int64("count", count).Msg("cleaned up expired reservations")
			}

		case <-modelCostRefreshTicker.C:
			if err := budgetSvc.RefreshModelCostCache(ctx); err != nil {
				log.Error().Err(err).Msg("failed to refresh model cost cache")
			}

		case <-budgetMetricsTicker.C:
			if err := budgetSvc.RefreshBudgetMetrics(ctx); err != nil {
				log.Error().Err(err).Msg("failed to refresh budget metrics")
			}
		}
	}
}
