// Package main is the entry point for the Prebid Server
package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"flag"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/thenexusengine/tne_springwire/internal/adapters"
	_ "github.com/thenexusengine/tne_springwire/internal/adapters/appnexus"
	_ "github.com/thenexusengine/tne_springwire/internal/adapters/demo"
	_ "github.com/thenexusengine/tne_springwire/internal/adapters/pubmatic"
	_ "github.com/thenexusengine/tne_springwire/internal/adapters/rubicon"
	pbsconfig "github.com/thenexusengine/tne_springwire/internal/config"
	"github.com/thenexusengine/tne_springwire/internal/endpoints"
	"github.com/thenexusengine/tne_springwire/internal/exchange"
	"github.com/thenexusengine/tne_springwire/internal/metrics"
	"github.com/thenexusengine/tne_springwire/internal/middleware"
	"github.com/thenexusengine/tne_springwire/internal/storage"
	"github.com/thenexusengine/tne_springwire/pkg/logger"
	"github.com/thenexusengine/tne_springwire/pkg/redis"
)

func main() {
	// Parse flags with environment variable fallbacks
	port := flag.String("port", getEnvOrDefault("PBS_PORT", "8000"), "Server port")
	idrURL := flag.String("idr-url", getEnvOrDefault("IDR_URL", "http://localhost:5050"), "IDR service URL")
	idrEnabled := flag.Bool("idr-enabled", getEnvBoolOrDefault("IDR_ENABLED", true), "Enable IDR integration")
	timeout := flag.Duration("timeout", 1000*time.Millisecond, "Default auction timeout")
	flag.Parse()

	// Initialize structured logger
	logger.Init(logger.DefaultConfig())
	log := logger.Log

	log.Info().
		Str("port", *port).
		Str("idr_url", *idrURL).
		Bool("idr_enabled", *idrEnabled).
		Dur("timeout", *timeout).
		Msg("Starting The Nexus Engine PBS Server")

	// Initialize Prometheus metrics
	m := metrics.NewMetrics("pbs")
	log.Info().Msg("Prometheus metrics enabled")

	// Initialize PostgreSQL database connection
	var db *storage.BidderStore
	var publisherStore *storage.PublisherStore
	dbHost := os.Getenv("DB_HOST")
	if dbHost != "" {
		dbPort := getEnvOrDefault("DB_PORT", "5432")
		dbUser := getEnvOrDefault("DB_USER", "catalyst")
		dbPassword := getEnvOrDefault("DB_PASSWORD", "")
		dbName := getEnvOrDefault("DB_NAME", "catalyst")
		dbSSLMode := getEnvOrDefault("DB_SSL_MODE", "disable")

		dbConn, err := storage.NewDBConnection(dbHost, dbPort, dbUser, dbPassword, dbName, dbSSLMode)
		if err != nil {
			log.Warn().Err(err).Msg("Failed to connect to PostgreSQL, database-backed features disabled")
		} else {
			db = storage.NewBidderStore(dbConn)
			publisherStore = storage.NewPublisherStore(dbConn)

			// Test connection
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			// Load and log bidders from database
			bidders, err := db.ListActive(ctx)
			if err != nil {
				log.Warn().Err(err).Msg("Failed to load bidders from database")
			} else {
				log.Info().
					Int("count", len(bidders)).
					Msg("Bidders loaded from PostgreSQL")
			}

			// Test publisher store
			publishers, err := publisherStore.List(ctx)
			if err != nil {
				log.Warn().Err(err).Msg("Failed to load publishers from database")
			} else {
				log.Info().
					Int("count", len(publishers)).
					Msg("Publishers loaded from PostgreSQL")
			}
		}
	} else {
		log.Info().Msg("DB_HOST not set, database-backed features disabled")
	}

	// Initialize middleware
	cors := middleware.NewCORS(middleware.DefaultCORSConfig())
	security := middleware.NewSecurity(nil) // Uses DefaultSecurityConfig()
	// Initialize PublisherAuth first to check if it's enabled
	publisherAuth := middleware.NewPublisherAuth(middleware.DefaultPublisherAuthConfig())

	// Build Auth config with conditional bypass for /openrtb2/auction
	// If PublisherAuth is enabled, it handles auction auth (bypass general Auth)
	// If PublisherAuth is disabled, general Auth protects auction endpoint (no bypass)
	authConfig := middleware.DefaultAuthConfig()
	if publisherAuth.IsEnabled() {
		// PublisherAuth handles auction endpoint - bypass general Auth
		authConfig.BypassPaths = append(authConfig.BypassPaths, "/openrtb2/auction")
		log.Info().Msg("PublisherAuth enabled - /openrtb2/auction bypasses general Auth")
	} else {
		// PublisherAuth disabled - general Auth protects auction endpoint
		log.Warn().Msg("PublisherAuth disabled - /openrtb2/auction requires API key auth")
	}
	auth := middleware.NewAuth(authConfig)
	rateLimiter := middleware.NewRateLimiter(middleware.DefaultRateLimitConfig())
	sizeLimiter := middleware.NewSizeLimiter(middleware.DefaultSizeLimitConfig())
	gzipMiddleware := middleware.NewGzip(middleware.DefaultGzipConfig())

	// Wire up metrics to middleware for observability
	auth.SetMetrics(m)
	rateLimiter.SetMetrics(m)

	// Wire up PostgreSQL publisher store to publisher auth middleware
	if publisherStore != nil {
		publisherAuth.SetPublisherStore(publisherStore)
		log.Info().Msg("Publisher store connected to authentication middleware")
	}

	log.Info().
		Bool("cors_enabled", true).
		Bool("security_headers_enabled", security.GetConfig().Enabled).
		Bool("auth_enabled", auth.IsEnabled()).
		Bool("rate_limiting_enabled", rateLimiter != nil).
		Msg("Middleware initialized")

	// Configure exchange
	// P0: Currency conversion ENABLED by default for proper multi-currency support
	currencyConvEnabled := os.Getenv("CURRENCY_CONVERSION_ENABLED") != "false"
	idrAPIKey := os.Getenv("IDR_API_KEY")
	config := &exchange.Config{
		DefaultTimeout:        *timeout,
		MaxBidders:            50,
		IDREnabled:            *idrEnabled,
		IDRServiceURL:         *idrURL,
		IDRAPIKey:             idrAPIKey,
		EventRecordEnabled:    true,
		EventBufferSize:       100,
		CurrencyConv:          currencyConvEnabled,
		DefaultCurrency:       "USD",
	}

	// Create exchange with default registry
	ex := exchange.New(adapters.DefaultRegistry, config)

	// Wire up metrics for margin tracking
	ex.SetMetrics(m)
	log.Info().Msg("Metrics connected to exchange for margin tracking")

	// Redis is used for API key auth and publisher admin (not dynamic bidders).
	var redisClient *redis.Client
	redisURL := os.Getenv("REDIS_URL")
	if redisURL != "" {
		var err error
		redisClient, err = redis.New(redisURL)
		if err != nil {
			log.Warn().Err(err).Msg("Failed to connect to Redis")
		} else {
			// Set Redis client on auth middlewares for shared validation
			auth.SetRedisClient(redisClient)
			publisherAuth.SetRedisClient(redisClient)
			log.Info().Msg("Redis client set for auth middlewares")
		}
	} else {
		log.Info().Msg("REDIS_URL not set, Redis-backed features disabled")
	}

	// List registered bidders
	bidders := adapters.DefaultRegistry.ListBidders()
	log.Info().
		Int("count", len(bidders)).
		Strs("bidders", bidders).
		Msg("Static bidders registered")

	// Create handlers
	auctionHandler := endpoints.NewAuctionHandler(ex)
	statusHandler := endpoints.NewStatusHandler()
	// Static bidders only (no dynamic registry).
	biddersHandler := endpoints.NewDynamicInfoBiddersHandler(adapters.DefaultRegistry)

	// Cookie sync handlers
	hostURL := os.Getenv("PBS_HOST_URL")
	if hostURL == "" {
		hostURL = "https://catalyst.springwire.ai"
	}
	cookieSyncConfig := endpoints.DefaultCookieSyncConfig(hostURL)
	cookieSyncHandler := endpoints.NewCookieSyncHandler(cookieSyncConfig)
	setuidHandler := endpoints.NewSetUIDHandler(cookieSyncHandler.ListBidders())
	optoutHandler := endpoints.NewOptOutHandler()

	log.Info().
		Str("host_url", hostURL).
		Int("syncers", len(cookieSyncHandler.ListBidders())).
		Msg("Cookie sync initialized")

	// P0-4: Initialize privacy middleware for GDPR/COPPA compliance
	privacyConfig := middleware.DefaultPrivacyConfig()
	// Allow disabling GDPR enforcement via environment variable (for testing)
	if os.Getenv("PBS_DISABLE_GDPR_ENFORCEMENT") == "true" {
		privacyConfig.EnforceGDPR = false
		log.Warn().Msg("GDPR enforcement disabled via PBS_DISABLE_GDPR_ENFORCEMENT")
	}
	privacyMiddleware := middleware.NewPrivacyMiddleware(privacyConfig)

	// Wrap auction handler with privacy middleware
	privacyProtectedAuction := privacyMiddleware(auctionHandler)

	log.Info().
		Bool("gdpr_enforcement", privacyConfig.EnforceGDPR).
		Bool("coppa_enforcement", privacyConfig.EnforceCOPPA).
		Bool("strict_mode", privacyConfig.StrictMode).
		Msg("Privacy middleware initialized")

	// Setup routes
	mux := http.NewServeMux()
	mux.Handle("/openrtb2/auction", privacyProtectedAuction)
	mux.Handle("/status", statusHandler)
	mux.Handle("/health", healthHandler())
	mux.Handle("/health/ready", readyHandler(redisClient, ex))
	mux.Handle("/info/bidders", biddersHandler)

	// Cookie sync endpoints
	mux.Handle("/cookie_sync", cookieSyncHandler)
	mux.Handle("/setuid", setuidHandler)
	mux.Handle("/optout", optoutHandler)

	// Prometheus metrics endpoint
	mux.Handle("/metrics", metrics.Handler())

	// Admin endpoints for runtime configuration
	mux.HandleFunc("/admin/circuit-breaker", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if ex.GetIDRClient() != nil {
			stats := ex.GetIDRClient().CircuitBreakerStats()
			if err := json.NewEncoder(w).Encode(stats); err != nil {
				log.Error().Err(err).Msg("failed to encode circuit breaker stats")
			}
		} else {
			if err := json.NewEncoder(w).Encode(map[string]string{"status": "IDR disabled"}); err != nil {
				log.Error().Err(err).Msg("failed to encode IDR disabled status")
			}
		}
	})

	// Live dashboard for monitoring
	dashboardHandler := endpoints.NewDashboardHandler()
	metricsAPIHandler := endpoints.NewMetricsAPIHandler()
	publisherAdminHandler := endpoints.NewPublisherAdminHandler(redisClient)
	mux.Handle("/admin/dashboard", dashboardHandler)
	mux.Handle("/admin/metrics", metricsAPIHandler)
	mux.Handle("/admin/publishers", publisherAdminHandler)
	mux.Handle("/admin/publishers/", publisherAdminHandler) // With trailing slash for IDs

	// Build middleware chain: CORS -> Security -> Logging -> Size Limit -> Auth -> PublisherAuth -> Rate Limit -> Metrics -> Gzip -> Handler
	// Note: CORS must be outermost to handle preflight OPTIONS requests
	// Note: Security headers applied early to ensure all responses have them
	// Note: Auth handles API key auth for admin endpoints
	// Note: PublisherAuth handles publisher validation for auction endpoints
	// Note: Gzip is innermost so responses are compressed before being sent
	handler := http.Handler(mux)
	handler = gzipMiddleware.Middleware(handler) // Compress responses
	handler = m.Middleware(handler)
	handler = rateLimiter.Middleware(handler)
	handler = publisherAuth.Middleware(handler) // Publisher auth for auction endpoints
	handler = auth.Middleware(handler)
	handler = sizeLimiter.Middleware(handler)
	handler = loggingMiddleware(handler)
	handler = security.Middleware(handler)
	handler = cors.Middleware(handler)

	// Create server (P2-6: use named constants for timeouts)
	server := &http.Server{
		Addr:         ":" + *port,
		Handler:      handler,
		ReadTimeout:  pbsconfig.ServerReadTimeout,
		WriteTimeout: pbsconfig.ServerWriteTimeout,
		IdleTimeout:  pbsconfig.ServerIdleTimeout,
	}

	// Start server in goroutine
	go func() {
		log.Info().Str("addr", ":"+*port).Msg("Server listening")
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal().Err(err).Msg("Server error")
		}
	}()

	// Wait for shutdown signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	sig := <-quit

	log.Info().Str("signal", sig.String()).Msg("Shutdown signal received")

	// Stop rate limiter cleanup goroutine
	rateLimiter.Stop()

	// Flush pending events from exchange
	if err := ex.Close(); err != nil {
		log.Warn().Err(err).Msg("Error flushing event recorder")
	} else {
		log.Info().Msg("Event recorder flushed")
	}

	ctx, cancel := context.WithTimeout(context.Background(), pbsconfig.ShutdownTimeout)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		log.Fatal().Err(err).Msg("Server forced to shutdown")
	}

	log.Info().Msg("Server stopped gracefully")
}

// responseWriter wraps http.ResponseWriter to capture status code
type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

// loggingMiddleware logs HTTP requests with structured logging
func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// Wrap response writer to capture status
		wrapped := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}

		// Generate request ID
		requestID := r.Header.Get("X-Request-ID")
		if requestID == "" {
			requestID = generateRequestID()
		}

		// Add request ID to response
		w.Header().Set("X-Request-ID", requestID)

		// Process request
		next.ServeHTTP(wrapped, r)

		// Log request completion
		duration := time.Since(start)

		event := logger.Log.Info()
		if wrapped.statusCode >= 400 {
			event = logger.Log.Warn()
		}
		if wrapped.statusCode >= 500 {
			event = logger.Log.Error()
		}

		event.
			Str("request_id", requestID).
			Str("method", r.Method).
			Str("path", r.URL.Path).
			Int("status", wrapped.statusCode).
			Dur("duration_ms", duration).
			Str("remote_addr", r.RemoteAddr).
			Str("user_agent", r.UserAgent()).
			Msg("HTTP request")
	})
}

// healthHandler returns a simple liveness check (Kubernetes liveness probe)
func healthHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		health := map[string]interface{}{
			"status":    "healthy",
			"timestamp": time.Now().UTC().Format(time.RFC3339),
			"version":   "1.0.0",
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(health); err != nil {
			logger.Log.Error().Err(err).Msg("failed to encode health response")
		}
	})
}

// readyHandler returns a readiness check with dependency verification (Kubernetes readiness probe)
func readyHandler(redisClient *redis.Client, ex *exchange.Exchange) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()

		checks := make(map[string]interface{})
		allHealthy := true

		// Check Redis if available
		if redisClient != nil {
			if err := redisClient.Ping(ctx); err != nil {
				checks["redis"] = map[string]interface{}{
					"status": "unhealthy",
					"error":  err.Error(),
				}
				allHealthy = false
			} else {
				checks["redis"] = map[string]interface{}{
					"status": "healthy",
				}
			}
		} else {
			checks["redis"] = map[string]interface{}{
				"status": "disabled",
			}
		}

		// Check IDR service if enabled
		idrClient := ex.GetIDRClient()
		if idrClient != nil {
			if err := idrClient.HealthCheck(ctx); err != nil {
				checks["idr"] = map[string]interface{}{
					"status": "unhealthy",
					"error":  err.Error(),
				}
				allHealthy = false
			} else {
				checks["idr"] = map[string]interface{}{
					"status": "healthy",
				}
			}
		} else {
			checks["idr"] = map[string]interface{}{
				"status": "disabled",
			}
		}

		status := http.StatusOK
		if !allHealthy {
			status = http.StatusServiceUnavailable
		}

		response := map[string]interface{}{
			"ready":     allHealthy,
			"timestamp": time.Now().UTC().Format(time.RFC3339),
			"checks":    checks,
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		if err := json.NewEncoder(w).Encode(response); err != nil {
			logger.Log.Error().Err(err).Msg("failed to encode readiness response")
		}
	})
}

// generateRequestID creates a unique request ID using cryptographically secure randomness
func generateRequestID() string {
	// Use 8 bytes (16 hex chars) for a good balance of uniqueness and brevity
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		// Fallback to timestamp-based ID if crypto/rand fails (should never happen)
		return time.Now().Format("20060102150405.000000000")
	}
	return hex.EncodeToString(b)
}

// getEnvOrDefault returns the environment variable value or a default
func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// getEnvBoolOrDefault returns the environment variable as bool or a default
func getEnvBoolOrDefault(key string, defaultValue bool) bool {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	return value == "true" || value == "1" || value == "yes"
}
