package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
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

// Server represents the PBS server
type Server struct {
	config      *ServerConfig
	httpServer  *http.Server
	metrics     *metrics.Metrics
	exchange    *exchange.Exchange
	rateLimiter *middleware.RateLimiter
	db          *storage.BidderStore
	publisher   *storage.PublisherStore
	redisClient *redis.Client
}

// NewServer creates a new PBS server instance
func NewServer(cfg *ServerConfig) (*Server, error) {
	s := &Server{
		config: cfg,
	}

	if err := s.initialize(); err != nil {
		return nil, err
	}

	return s, nil
}

// initialize sets up all server components
func (s *Server) initialize() error {
	log := logger.Log

	log.Info().
		Str("port", s.config.Port).
		Str("idr_url", s.config.IDRUrl).
		Bool("idr_enabled", s.config.IDREnabled).
		Dur("timeout", s.config.Timeout).
		Msg("Initializing The Nexus Engine PBS Server")

	// Initialize Prometheus metrics
	s.metrics = metrics.NewMetrics("pbs")
	log.Info().Msg("Prometheus metrics enabled")

	// Initialize database if configured
	if err := s.initDatabase(); err != nil {
		// Database failures are non-fatal, log and continue
		log.Warn().Err(err).Msg("Database initialization failed, continuing with reduced functionality")
	}

	// Initialize middleware
	s.initMiddleware()

	// Initialize exchange
	s.initExchange()

	// Initialize Redis if configured
	if err := s.initRedis(); err != nil {
		// Redis failures are non-fatal, log and continue
		log.Warn().Err(err).Msg("Redis initialization failed, continuing with reduced functionality")
	}

	// List registered bidders
	bidders := adapters.DefaultRegistry.ListBidders()
	log.Info().
		Int("count", len(bidders)).
		Strs("bidders", bidders).
		Msg("Static bidders registered")

	// Initialize handlers and build HTTP server
	s.initHandlers()

	return nil
}

// initDatabase initializes database connections
func (s *Server) initDatabase() error {
	log := logger.Log

	if s.config.DatabaseConfig == nil {
		log.Info().Msg("DB_HOST not set, database-backed features disabled")
		return nil
	}

	dbCfg := s.config.DatabaseConfig
	dbConn, err := storage.NewDBConnection(
		dbCfg.Host,
		dbCfg.Port,
		dbCfg.User,
		dbCfg.Password,
		dbCfg.Name,
		dbCfg.SSLMode,
	)
	if err != nil {
		log.Warn().Err(err).Msg("Failed to connect to PostgreSQL, database-backed features disabled")
		return err
	}

	s.db = storage.NewBidderStore(dbConn)
	s.publisher = storage.NewPublisherStore(dbConn)

	// Test connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Load and log bidders from database
	bidders, err := s.db.ListActive(ctx)
	if err != nil {
		log.Warn().Err(err).Msg("Failed to load bidders from database")
	} else {
		log.Info().
			Int("count", len(bidders)).
			Msg("Bidders loaded from PostgreSQL")
	}

	// Test publisher store
	publishers, err := s.publisher.List(ctx)
	if err != nil {
		log.Warn().Err(err).Msg("Failed to load publishers from database")
	} else {
		log.Info().
			Int("count", len(publishers)).
			Msg("Publishers loaded from PostgreSQL")
	}

	return nil
}

// initMiddleware initializes all middleware components
func (s *Server) initMiddleware() {
	log := logger.Log

	// Initialize PublisherAuth first to check if it's enabled
	publisherAuth := middleware.NewPublisherAuth(middleware.DefaultPublisherAuthConfig())

	// Build Auth config with conditional bypass for /openrtb2/auction
	authConfig := middleware.DefaultAuthConfig()
	if publisherAuth.IsEnabled() {
		authConfig.BypassPaths = append(authConfig.BypassPaths, "/openrtb2/auction")
		log.Info().Msg("PublisherAuth enabled - /openrtb2/auction bypasses general Auth")
	} else {
		log.Warn().Msg("PublisherAuth disabled - /openrtb2/auction requires API key auth")
	}

	// Store rate limiter for graceful shutdown
	s.rateLimiter = middleware.NewRateLimiter(middleware.DefaultRateLimitConfig())

	log.Info().Msg("Middleware initialized")
}

// initExchange initializes the exchange engine
func (s *Server) initExchange() {
	log := logger.Log

	// Create exchange with default registry
	s.exchange = exchange.New(adapters.DefaultRegistry, s.config.ToExchangeConfig())

	// Wire up metrics for margin tracking
	s.exchange.SetMetrics(s.metrics)
	log.Info().Msg("Metrics connected to exchange for margin tracking")
}

// initRedis initializes Redis client
func (s *Server) initRedis() error {
	log := logger.Log

	if s.config.RedisURL == "" {
		log.Info().Msg("REDIS_URL not set, Redis-backed features disabled")
		return nil
	}

	var err error
	s.redisClient, err = redis.New(s.config.RedisURL)
	if err != nil {
		log.Warn().Err(err).Msg("Failed to connect to Redis")
		return err
	}

	log.Info().Msg("Redis client initialized")
	return nil
}

// initHandlers initializes HTTP handlers and builds the handler chain
func (s *Server) initHandlers() {
	log := logger.Log

	// Create handlers
	auctionHandler := endpoints.NewAuctionHandler(s.exchange)
	statusHandler := endpoints.NewStatusHandler()
	biddersHandler := endpoints.NewDynamicInfoBiddersHandler(adapters.DefaultRegistry)

	// Cookie sync handlers
	cookieSyncConfig := endpoints.DefaultCookieSyncConfig(s.config.HostURL)
	cookieSyncHandler := endpoints.NewCookieSyncHandler(cookieSyncConfig)
	setuidHandler := endpoints.NewSetUIDHandler(cookieSyncHandler.ListBidders())
	optoutHandler := endpoints.NewOptOutHandler()

	log.Info().
		Str("host_url", s.config.HostURL).
		Int("syncers", len(cookieSyncHandler.ListBidders())).
		Msg("Cookie sync initialized")

	// Initialize privacy middleware
	privacyConfig := middleware.DefaultPrivacyConfig()
	if s.config.DisableGDPREnforcement {
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
	mux.Handle("/health/ready", readyHandler(s.redisClient, s.exchange))
	mux.Handle("/info/bidders", biddersHandler)

	// Cookie sync endpoints
	mux.Handle("/cookie_sync", cookieSyncHandler)
	mux.Handle("/setuid", setuidHandler)
	mux.Handle("/optout", optoutHandler)

	// Prometheus metrics endpoint
	mux.Handle("/metrics", metrics.Handler())

	// Admin endpoints
	mux.HandleFunc("/admin/circuit-breaker", s.circuitBreakerHandler)
	dashboardHandler := endpoints.NewDashboardHandler()
	metricsAPIHandler := endpoints.NewMetricsAPIHandler()
	publisherAdminHandler := endpoints.NewPublisherAdminHandler(s.redisClient)
	mux.Handle("/admin/dashboard", dashboardHandler)
	mux.Handle("/admin/metrics", metricsAPIHandler)
	mux.Handle("/admin/publishers", publisherAdminHandler)
	mux.Handle("/admin/publishers/", publisherAdminHandler)

	// Build middleware chain
	handler := s.buildHandler(mux)

	// Create HTTP server
	s.httpServer = &http.Server{
		Addr:         ":" + s.config.Port,
		Handler:      handler,
		ReadTimeout:  pbsconfig.ServerReadTimeout,
		WriteTimeout: pbsconfig.ServerWriteTimeout,
		IdleTimeout:  pbsconfig.ServerIdleTimeout,
	}
}

// buildHandler builds the middleware chain
func (s *Server) buildHandler(mux *http.ServeMux) http.Handler {
	log := logger.Log

	// Initialize middleware
	cors := middleware.NewCORS(middleware.DefaultCORSConfig())
	security := middleware.NewSecurity(nil)
	publisherAuth := middleware.NewPublisherAuth(middleware.DefaultPublisherAuthConfig())

	// Build Auth config with conditional bypass
	authConfig := middleware.DefaultAuthConfig()
	if publisherAuth.IsEnabled() {
		authConfig.BypassPaths = append(authConfig.BypassPaths, "/openrtb2/auction")
	}
	auth := middleware.NewAuth(authConfig)
	sizeLimiter := middleware.NewSizeLimiter(middleware.DefaultSizeLimitConfig())
	gzipMiddleware := middleware.NewGzip(middleware.DefaultGzipConfig())

	// Wire up metrics
	auth.SetMetrics(s.metrics)
	s.rateLimiter.SetMetrics(s.metrics)

	// Wire up stores
	if s.publisher != nil {
		publisherAuth.SetPublisherStore(s.publisher)
		log.Info().Msg("Publisher store connected to authentication middleware")
	}

	// Wire up Redis
	if s.redisClient != nil {
		auth.SetRedisClient(s.redisClient)
		publisherAuth.SetRedisClient(s.redisClient)
		log.Info().Msg("Redis client set for auth middlewares")
	}

	log.Info().
		Bool("cors_enabled", true).
		Bool("security_headers_enabled", security.GetConfig().Enabled).
		Bool("auth_enabled", auth.IsEnabled()).
		Bool("rate_limiting_enabled", s.rateLimiter != nil).
		Msg("Middleware chain built")

	// Build chain: CORS -> Security -> Logging -> Size Limit -> Auth -> PublisherAuth -> Rate Limit -> Metrics -> Gzip -> Handler
	handler := http.Handler(mux)
	handler = gzipMiddleware.Middleware(handler)
	handler = s.metrics.Middleware(handler)
	handler = s.rateLimiter.Middleware(handler)
	handler = publisherAuth.Middleware(handler)
	handler = auth.Middleware(handler)
	handler = sizeLimiter.Middleware(handler)
	handler = loggingMiddleware(handler)
	handler = security.Middleware(handler)
	handler = cors.Middleware(handler)

	return handler
}

// circuitBreakerHandler returns circuit breaker stats
func (s *Server) circuitBreakerHandler(w http.ResponseWriter, r *http.Request) {
	log := logger.Log
	w.Header().Set("Content-Type", "application/json")

	response := make(map[string]interface{})

	// Include IDR circuit breaker stats
	if s.exchange.GetIDRClient() != nil {
		response["idr"] = s.exchange.GetIDRClient().CircuitBreakerStats()
	} else {
		response["idr"] = map[string]string{"status": "disabled"}
	}

	// Include bidder circuit breaker stats
	response["bidders"] = s.exchange.GetBidderCircuitBreakerStats()

	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Error().Err(err).Msg("failed to encode circuit breaker stats")
	}
}

// Start starts the HTTP server
func (s *Server) Start() error {
	log := logger.Log
	log.Info().Str("addr", s.httpServer.Addr).Msg("Server listening")

	if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

// Shutdown performs graceful shutdown
func (s *Server) Shutdown(ctx context.Context) error {
	log := logger.Log
	log.Info().Msg("Starting graceful shutdown")

	// Stop rate limiter cleanup goroutine
	if s.rateLimiter != nil {
		s.rateLimiter.Stop()
	}

	// Flush pending events from exchange
	if s.exchange != nil {
		if err := s.exchange.Close(); err != nil {
			log.Warn().Err(err).Msg("Error flushing event recorder")
		} else {
			log.Info().Msg("Event recorder flushed")
		}
	}

	// Shutdown HTTP server
	if err := s.httpServer.Shutdown(ctx); err != nil {
		return err
	}

	log.Info().Msg("Server stopped gracefully")
	return nil
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

// healthHandler returns a simple liveness check
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

// readyHandler returns a readiness check with dependency verification
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

// generateRequestID creates a unique request ID
func generateRequestID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return time.Now().Format("20060102150405.000000000")
	}
	return hex.EncodeToString(b)
}
