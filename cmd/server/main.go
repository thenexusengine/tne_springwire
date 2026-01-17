// Package main is the entry point for the Prebid Server
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	pbsconfig "github.com/thenexusengine/tne_springwire/internal/config"
	"github.com/thenexusengine/tne_springwire/pkg/logger"
)

func main() {
	// Parse configuration from flags and environment
	cfg := ParseConfig()

	// Initialize structured logger
	logger.Init(logger.DefaultConfig())
	log := logger.Log

	// Create server
	server, err := NewServer(cfg)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to create server")
	}

	// Start server in goroutine
	go func() {
		if err := server.Start(); err != nil {
			log.Fatal().Err(err).Msg("Server error")
		}
	}()

	// Wait for shutdown signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	sig := <-quit

	log.Info().Str("signal", sig.String()).Msg("Shutdown signal received")

	// Graceful shutdown
	ctx, cancel := context.WithTimeout(context.Background(), pbsconfig.ShutdownTimeout)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		log.Fatal().Err(err).Msg("Server forced to shutdown")
	}
}
