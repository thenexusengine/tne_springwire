package main

import (
	"flag"
	"os"
	"time"

	"github.com/thenexusengine/tne_springwire/internal/exchange"
)

// ServerConfig holds all server configuration
type ServerConfig struct {
	// Server
	Port    string
	Timeout time.Duration

	// Database
	DatabaseConfig *DatabaseConfig

	// Redis
	RedisURL string

	// IDR
	IDREnabled bool
	IDRUrl     string
	IDRAPIKey  string

	// Currency
	CurrencyConversionEnabled bool
	DefaultCurrency           string

	// Privacy
	DisableGDPREnforcement bool

	// Cookie Sync
	HostURL string
}

// DatabaseConfig holds database connection configuration
type DatabaseConfig struct {
	Host     string
	Port     string
	User     string
	Password string
	Name     string
	SSLMode  string
}

// ParseConfig parses configuration from flags and environment variables
func ParseConfig() *ServerConfig {
	// Parse flags with environment variable fallbacks
	port := flag.String("port", getEnvOrDefault("PBS_PORT", "8000"), "Server port")
	idrURL := flag.String("idr-url", getEnvOrDefault("IDR_URL", "http://localhost:5050"), "IDR service URL")
	idrEnabled := flag.Bool("idr-enabled", getEnvBoolOrDefault("IDR_ENABLED", true), "Enable IDR integration")
	timeout := flag.Duration("timeout", 1000*time.Millisecond, "Default auction timeout")
	flag.Parse()

	cfg := &ServerConfig{
		Port:                      *port,
		Timeout:                   *timeout,
		RedisURL:                  os.Getenv("REDIS_URL"),
		IDREnabled:                *idrEnabled,
		IDRUrl:                    *idrURL,
		IDRAPIKey:                 os.Getenv("IDR_API_KEY"),
		CurrencyConversionEnabled: os.Getenv("CURRENCY_CONVERSION_ENABLED") != "false",
		DefaultCurrency:           "USD",
		DisableGDPREnforcement:    os.Getenv("PBS_DISABLE_GDPR_ENFORCEMENT") == "true",
		HostURL:                   getEnvOrDefault("PBS_HOST_URL", "https://catalyst.springwire.ai"),
	}

	// Parse database config if DB_HOST is set
	if dbHost := os.Getenv("DB_HOST"); dbHost != "" {
		cfg.DatabaseConfig = &DatabaseConfig{
			Host:     dbHost,
			Port:     getEnvOrDefault("DB_PORT", "5432"),
			User:     getEnvOrDefault("DB_USER", "catalyst"),
			Password: getEnvOrDefault("DB_PASSWORD", ""),
			Name:     getEnvOrDefault("DB_NAME", "catalyst"),
			SSLMode:  getEnvOrDefault("DB_SSL_MODE", "disable"),
		}
	}

	return cfg
}

// ToExchangeConfig converts ServerConfig to exchange.Config
func (c *ServerConfig) ToExchangeConfig() *exchange.Config {
	return &exchange.Config{
		DefaultTimeout:     c.Timeout,
		MaxBidders:         50,
		IDREnabled:         c.IDREnabled,
		IDRServiceURL:      c.IDRUrl,
		IDRAPIKey:          c.IDRAPIKey,
		EventRecordEnabled: true,
		EventBufferSize:    100,
		CurrencyConv:       c.CurrencyConversionEnabled,
		DefaultCurrency:    c.DefaultCurrency,
	}
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
