package main

import (
	"flag"
	"os"
	"testing"
	"time"
)

func TestParseConfig_Defaults(t *testing.T) {
	// Clear all environment variables
	clearEnvVars(t)

	// Reset flags before each test
	flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.ContinueOnError)

	cfg := ParseConfig()

	if cfg.Port != "8000" {
		t.Errorf("Expected default port '8000', got '%s'", cfg.Port)
	}

	if cfg.Timeout != 1000*time.Millisecond {
		t.Errorf("Expected default timeout 1000ms, got %v", cfg.Timeout)
	}

	if cfg.IDRUrl != "http://localhost:5050" {
		t.Errorf("Expected default IDR URL 'http://localhost:5050', got '%s'", cfg.IDRUrl)
	}

	if !cfg.IDREnabled {
		t.Error("Expected IDR to be enabled by default")
	}

	if cfg.CurrencyConversionEnabled != true {
		t.Error("Expected currency conversion to be enabled by default")
	}

	if cfg.DefaultCurrency != "USD" {
		t.Errorf("Expected default currency 'USD', got '%s'", cfg.DefaultCurrency)
	}

	if cfg.HostURL != "https://catalyst.springwire.ai" {
		t.Errorf("Expected default host URL 'https://catalyst.springwire.ai', got '%s'", cfg.HostURL)
	}

	if cfg.DatabaseConfig != nil {
		t.Error("Expected no database config when DB_HOST is not set")
	}

	if cfg.RedisURL != "" {
		t.Error("Expected empty Redis URL when REDIS_URL is not set")
	}
}

func TestParseConfig_EnvironmentOverrides(t *testing.T) {
	tests := []struct {
		name     string
		envVars  map[string]string
		validate func(*testing.T, *ServerConfig)
	}{
		{
			name: "Custom port",
			envVars: map[string]string{
				"PBS_PORT": "9000",
			},
			validate: func(t *testing.T, cfg *ServerConfig) {
				if cfg.Port != "9000" {
					t.Errorf("Expected port '9000', got '%s'", cfg.Port)
				}
			},
		},
		{
			name: "Custom IDR URL",
			envVars: map[string]string{
				"IDR_URL": "http://idr.example.com:8080",
			},
			validate: func(t *testing.T, cfg *ServerConfig) {
				if cfg.IDRUrl != "http://idr.example.com:8080" {
					t.Errorf("Expected IDR URL 'http://idr.example.com:8080', got '%s'", cfg.IDRUrl)
				}
			},
		},
		{
			name: "IDR disabled",
			envVars: map[string]string{
				"IDR_ENABLED": "false",
			},
			validate: func(t *testing.T, cfg *ServerConfig) {
				if cfg.IDREnabled {
					t.Error("Expected IDR to be disabled")
				}
			},
		},
		{
			name: "IDR API key",
			envVars: map[string]string{
				"IDR_API_KEY": "secret-key-123",
			},
			validate: func(t *testing.T, cfg *ServerConfig) {
				if cfg.IDRAPIKey != "secret-key-123" {
					t.Errorf("Expected IDR API key 'secret-key-123', got '%s'", cfg.IDRAPIKey)
				}
			},
		},
		{
			name: "Redis URL",
			envVars: map[string]string{
				"REDIS_URL": "redis://localhost:6379/0",
			},
			validate: func(t *testing.T, cfg *ServerConfig) {
				if cfg.RedisURL != "redis://localhost:6379/0" {
					t.Errorf("Expected Redis URL 'redis://localhost:6379/0', got '%s'", cfg.RedisURL)
				}
			},
		},
		{
			name: "Currency conversion disabled",
			envVars: map[string]string{
				"CURRENCY_CONVERSION_ENABLED": "false",
			},
			validate: func(t *testing.T, cfg *ServerConfig) {
				if cfg.CurrencyConversionEnabled {
					t.Error("Expected currency conversion to be disabled")
				}
			},
		},
		{
			name: "GDPR enforcement disabled",
			envVars: map[string]string{
				"PBS_DISABLE_GDPR_ENFORCEMENT": "true",
			},
			validate: func(t *testing.T, cfg *ServerConfig) {
				if !cfg.DisableGDPREnforcement {
					t.Error("Expected GDPR enforcement to be disabled")
				}
			},
		},
		{
			name: "Custom host URL",
			envVars: map[string]string{
				"PBS_HOST_URL": "https://custom.example.com",
			},
			validate: func(t *testing.T, cfg *ServerConfig) {
				if cfg.HostURL != "https://custom.example.com" {
					t.Errorf("Expected host URL 'https://custom.example.com', got '%s'", cfg.HostURL)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clear and set environment variables
			clearEnvVars(t)
			for key, value := range tt.envVars {
				t.Setenv(key, value)
			}

			// Reset flags
			flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.ContinueOnError)

			cfg := ParseConfig()
			tt.validate(t, cfg)
		})
	}
}

func TestParseConfig_DatabaseConfig(t *testing.T) {
	clearEnvVars(t)

	// Set database environment variables
	t.Setenv("DB_HOST", "postgres.example.com")
	t.Setenv("DB_PORT", "5433")
	t.Setenv("DB_USER", "testuser")
	t.Setenv("DB_PASSWORD", "testpass")
	t.Setenv("DB_NAME", "testdb")
	t.Setenv("DB_SSL_MODE", "require")

	// Reset flags
	flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.ContinueOnError)

	cfg := ParseConfig()

	if cfg.DatabaseConfig == nil {
		t.Fatal("Expected database config to be set")
	}

	dbCfg := cfg.DatabaseConfig

	if dbCfg.Host != "postgres.example.com" {
		t.Errorf("Expected DB host 'postgres.example.com', got '%s'", dbCfg.Host)
	}

	if dbCfg.Port != "5433" {
		t.Errorf("Expected DB port '5433', got '%s'", dbCfg.Port)
	}

	if dbCfg.User != "testuser" {
		t.Errorf("Expected DB user 'testuser', got '%s'", dbCfg.User)
	}

	if dbCfg.Password != "testpass" {
		t.Errorf("Expected DB password 'testpass', got '%s'", dbCfg.Password)
	}

	if dbCfg.Name != "testdb" {
		t.Errorf("Expected DB name 'testdb', got '%s'", dbCfg.Name)
	}

	if dbCfg.SSLMode != "require" {
		t.Errorf("Expected DB SSL mode 'require', got '%s'", dbCfg.SSLMode)
	}
}

func TestParseConfig_DatabaseConfig_NotSet(t *testing.T) {
	clearEnvVars(t)

	// Reset flags
	flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.ContinueOnError)

	cfg := ParseConfig()

	if cfg.DatabaseConfig != nil {
		t.Error("Expected no database config when DB_HOST is not set")
	}
}

func TestParseConfig_DatabaseConfig_Defaults(t *testing.T) {
	clearEnvVars(t)

	// Set only DB_HOST, use defaults for the rest
	t.Setenv("DB_HOST", "localhost")

	// Reset flags
	flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.ContinueOnError)

	cfg := ParseConfig()

	if cfg.DatabaseConfig == nil {
		t.Fatal("Expected database config to be set")
	}

	dbCfg := cfg.DatabaseConfig

	if dbCfg.Host != "localhost" {
		t.Errorf("Expected DB host 'localhost', got '%s'", dbCfg.Host)
	}

	if dbCfg.Port != "5432" {
		t.Errorf("Expected default DB port '5432', got '%s'", dbCfg.Port)
	}

	if dbCfg.User != "catalyst" {
		t.Errorf("Expected default DB user 'catalyst', got '%s'", dbCfg.User)
	}

	if dbCfg.Password != "" {
		t.Errorf("Expected default DB password '', got '%s'", dbCfg.Password)
	}

	if dbCfg.Name != "catalyst" {
		t.Errorf("Expected default DB name 'catalyst', got '%s'", dbCfg.Name)
	}

	if dbCfg.SSLMode != "disable" {
		t.Errorf("Expected default DB SSL mode 'disable', got '%s'", dbCfg.SSLMode)
	}
}

func TestToExchangeConfig(t *testing.T) {
	cfg := &ServerConfig{
		Port:                      "8000",
		Timeout:                   2000 * time.Millisecond,
		IDREnabled:                true,
		IDRUrl:                    "http://idr.example.com",
		IDRAPIKey:                 "test-api-key",
		CurrencyConversionEnabled: true,
		DefaultCurrency:           "EUR",
	}

	exCfg := cfg.ToExchangeConfig()

	if exCfg.DefaultTimeout != 2000*time.Millisecond {
		t.Errorf("Expected timeout 2000ms, got %v", exCfg.DefaultTimeout)
	}

	if exCfg.MaxBidders != 50 {
		t.Errorf("Expected max bidders 50, got %d", exCfg.MaxBidders)
	}

	if !exCfg.IDREnabled {
		t.Error("Expected IDR to be enabled")
	}

	if exCfg.IDRServiceURL != "http://idr.example.com" {
		t.Errorf("Expected IDR URL 'http://idr.example.com', got '%s'", exCfg.IDRServiceURL)
	}

	if exCfg.IDRAPIKey != "test-api-key" {
		t.Errorf("Expected IDR API key 'test-api-key', got '%s'", exCfg.IDRAPIKey)
	}

	if !exCfg.EventRecordEnabled {
		t.Error("Expected event recording to be enabled")
	}

	if exCfg.EventBufferSize != 100 {
		t.Errorf("Expected event buffer size 100, got %d", exCfg.EventBufferSize)
	}

	if !exCfg.CurrencyConv {
		t.Error("Expected currency conversion to be enabled")
	}

	if exCfg.DefaultCurrency != "EUR" {
		t.Errorf("Expected default currency 'EUR', got '%s'", exCfg.DefaultCurrency)
	}
}

func TestGetEnvOrDefault(t *testing.T) {
	tests := []struct {
		name         string
		key          string
		value        string
		setValue     bool
		defaultValue string
		expected     string
	}{
		{
			name:         "With value",
			key:          "TEST_VAR",
			value:        "test_value",
			setValue:     true,
			defaultValue: "default",
			expected:     "test_value",
		},
		{
			name:         "Without value",
			key:          "MISSING_VAR",
			setValue:     false,
			defaultValue: "default",
			expected:     "default",
		},
		{
			name:         "Empty string",
			key:          "EMPTY_VAR",
			value:        "",
			setValue:     true,
			defaultValue: "default",
			expected:     "default",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.setValue {
				t.Setenv(tt.key, tt.value)
			} else {
				os.Unsetenv(tt.key)
			}

			result := getEnvOrDefault(tt.key, tt.defaultValue)

			if result != tt.expected {
				t.Errorf("Expected '%s', got '%s'", tt.expected, result)
			}
		})
	}
}

func TestGetEnvBoolOrDefault(t *testing.T) {
	tests := []struct {
		name         string
		value        string
		setValue     bool
		defaultValue bool
		expected     bool
	}{
		{
			name:         "true",
			value:        "true",
			setValue:     true,
			defaultValue: false,
			expected:     true,
		},
		{
			name:         "1",
			value:        "1",
			setValue:     true,
			defaultValue: false,
			expected:     true,
		},
		{
			name:         "yes",
			value:        "yes",
			setValue:     true,
			defaultValue: false,
			expected:     true,
		},
		{
			name:         "false",
			value:        "false",
			setValue:     true,
			defaultValue: true,
			expected:     false,
		},
		{
			name:         "0",
			value:        "0",
			setValue:     true,
			defaultValue: true,
			expected:     false,
		},
		{
			name:         "no",
			value:        "no",
			setValue:     true,
			defaultValue: true,
			expected:     false,
		},
		{
			name:         "Empty uses default false",
			value:        "",
			setValue:     false,
			defaultValue: false,
			expected:     false,
		},
		{
			name:         "Empty uses default true",
			value:        "",
			setValue:     false,
			defaultValue: true,
			expected:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key := "TEST_BOOL_VAR"
			if tt.setValue {
				t.Setenv(key, tt.value)
			} else {
				os.Unsetenv(key)
			}

			result := getEnvBoolOrDefault(key, tt.defaultValue)

			if result != tt.expected {
				t.Errorf("Expected %v, got %v", tt.expected, result)
			}
		})
	}
}

// Helper function to clear relevant environment variables
func clearEnvVars(t *testing.T) {
	t.Helper()

	envVars := []string{
		"PBS_PORT",
		"IDR_URL",
		"IDR_ENABLED",
		"IDR_API_KEY",
		"DB_HOST",
		"DB_PORT",
		"DB_USER",
		"DB_PASSWORD",
		"DB_NAME",
		"DB_SSL_MODE",
		"REDIS_URL",
		"CURRENCY_CONVERSION_ENABLED",
		"PBS_DISABLE_GDPR_ENFORCEMENT",
		"PBS_HOST_URL",
	}

	for _, key := range envVars {
		os.Unsetenv(key)
	}
}
