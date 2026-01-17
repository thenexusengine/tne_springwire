package logger

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"strings"
	"testing"
	"time"
)

// Test Helpers

// captureLogOutput captures log output to a buffer for testing
func captureLogOutput(t *testing.T, fn func()) string {
	t.Helper()

	// Save original stdout
	originalStdout := os.Stdout
	defer func() { os.Stdout = originalStdout }()

	// Create pipe to capture output
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("Failed to create pipe: %v", err)
	}

	// Redirect stdout to pipe
	os.Stdout = w

	// Run the function
	fn()

	// Close writer and restore stdout
	w.Close()
	os.Stdout = originalStdout

	// Read captured output
	var buf bytes.Buffer
	io.Copy(&buf, r)

	return buf.String()
}

// parseLogLine parses a JSON log line into a map
func parseLogLine(t *testing.T, line string) map[string]interface{} {
	t.Helper()

	line = strings.TrimSpace(line)
	if line == "" {
		return nil
	}

	var result map[string]interface{}
	if err := json.Unmarshal([]byte(line), &result); err != nil {
		t.Fatalf("Failed to parse log line: %v\nLine: %s", err, line)
	}

	return result
}

// Config Tests

func TestDefaultConfig(t *testing.T) {
	// Clear environment variables
	os.Unsetenv("LOG_LEVEL")
	os.Unsetenv("LOG_FORMAT")

	cfg := DefaultConfig()

	if cfg.Level != "info" {
		t.Errorf("Expected default level 'info', got '%s'", cfg.Level)
	}

	if cfg.Format != "json" {
		t.Errorf("Expected default format 'json', got '%s'", cfg.Format)
	}

	if cfg.TimeFormat != time.RFC3339 {
		t.Errorf("Expected time format RFC3339, got '%s'", cfg.TimeFormat)
	}
}

func TestDefaultConfig_EnvironmentOverrides(t *testing.T) {
	tests := []struct {
		name          string
		envLevel      string
		envFormat     string
		expectedLevel string
		expectedFormat string
	}{
		{
			name:           "Debug level",
			envLevel:       "debug",
			envFormat:      "",
			expectedLevel:  "debug",
			expectedFormat: "json",
		},
		{
			name:           "Console format",
			envLevel:       "",
			envFormat:      "console",
			expectedLevel:  "info",
			expectedFormat: "console",
		},
		{
			name:           "Both overridden",
			envLevel:       "error",
			envFormat:      "console",
			expectedLevel:  "error",
			expectedFormat: "console",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set environment variables
			if tt.envLevel != "" {
				t.Setenv("LOG_LEVEL", tt.envLevel)
			} else {
				os.Unsetenv("LOG_LEVEL")
			}

			if tt.envFormat != "" {
				t.Setenv("LOG_FORMAT", tt.envFormat)
			} else {
				os.Unsetenv("LOG_FORMAT")
			}

			cfg := DefaultConfig()

			if cfg.Level != tt.expectedLevel {
				t.Errorf("Expected level '%s', got '%s'", tt.expectedLevel, cfg.Level)
			}

			if cfg.Format != tt.expectedFormat {
				t.Errorf("Expected format '%s', got '%s'", tt.expectedFormat, cfg.Format)
			}
		})
	}
}

func TestGetEnv(t *testing.T) {
	tests := []struct {
		name     string
		key      string
		value    string
		setValue bool
		defVal   string
		expected string
	}{
		{
			name:     "With value",
			key:      "TEST_VAR",
			value:    "test_value",
			setValue: true,
			defVal:   "default",
			expected: "test_value",
		},
		{
			name:     "Without value",
			key:      "MISSING_VAR",
			setValue: false,
			defVal:   "default",
			expected: "default",
		},
		{
			name:     "Empty string",
			key:      "EMPTY_VAR",
			value:    "",
			setValue: true,
			defVal:   "default",
			expected: "default",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.setValue {
				t.Setenv(tt.key, tt.value)
			} else {
				os.Unsetenv(tt.key)
			}

			result := getEnv(tt.key, tt.defVal)

			if result != tt.expected {
				t.Errorf("Expected '%s', got '%s'", tt.expected, result)
			}
		})
	}
}

// Initialization Tests

func TestInit_JSONFormat(t *testing.T) {
	output := captureLogOutput(t, func() {
		Init(Config{
			Level:      "info",
			Format:     "json",
			TimeFormat: time.RFC3339,
		})
		Log.Info().Msg("test message")
	})

	logEntry := parseLogLine(t, output)

	if logEntry == nil {
		t.Fatal("Expected log output, got none")
	}

	if logEntry["level"] != "info" {
		t.Errorf("Expected level 'info', got '%v'", logEntry["level"])
	}

	if logEntry["message"] != "test message" {
		t.Errorf("Expected message 'test message', got '%v'", logEntry["message"])
	}

	if logEntry["service"] != "pbs" {
		t.Errorf("Expected service 'pbs', got '%v'", logEntry["service"])
	}

	if _, ok := logEntry["time"]; !ok {
		t.Error("Expected 'time' field in log output")
	}
}

func TestInit_ConsoleFormat(t *testing.T) {
	output := captureLogOutput(t, func() {
		Init(Config{
			Level:      "info",
			Format:     "console",
			TimeFormat: time.RFC3339,
		})
		Log.Info().Msg("test message")
	})

	if !strings.Contains(output, "test message") {
		t.Errorf("Expected 'test message' in output, got: %s", output)
	}

	// Console format should show INF for info level
	if !strings.Contains(output, "INF") {
		t.Errorf("Expected 'INF' log level indicator in output, got: %s", output)
	}

	if !strings.Contains(output, "pbs") {
		t.Errorf("Expected 'pbs' service name in output, got: %s", output)
	}
}

func TestInit_LogLevels(t *testing.T) {
	tests := []struct {
		level       string
		shouldLog   map[string]bool
	}{
		{
			level: "debug",
			shouldLog: map[string]bool{
				"debug": true,
				"info":  true,
				"warn":  true,
				"error": true,
			},
		},
		{
			level: "info",
			shouldLog: map[string]bool{
				"debug": false,
				"info":  true,
				"warn":  true,
				"error": true,
			},
		},
		{
			level: "warn",
			shouldLog: map[string]bool{
				"debug": false,
				"info":  false,
				"warn":  true,
				"error": true,
			},
		},
		{
			level: "error",
			shouldLog: map[string]bool{
				"debug": false,
				"info":  false,
				"warn":  false,
				"error": true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.level, func(t *testing.T) {
			output := captureLogOutput(t, func() {
				Init(Config{
					Level:      tt.level,
					Format:     "json",
					TimeFormat: time.RFC3339,
				})

				Log.Debug().Msg("debug message")
				Log.Info().Msg("info message")
				Log.Warn().Msg("warn message")
				Log.Error().Msg("error message")
			})

			lines := strings.Split(strings.TrimSpace(output), "\n")

			for levelName, shouldLog := range tt.shouldLog {
				found := false
				for _, line := range lines {
					if line == "" {
						continue
					}
					logEntry := parseLogLine(t, line)
					if logEntry != nil && logEntry["level"] == levelName {
						found = true
						break
					}
				}

				if shouldLog && !found {
					t.Errorf("Expected %s message to be logged with level %s, but it wasn't", levelName, tt.level)
				}

				if !shouldLog && found {
					t.Errorf("Expected %s message NOT to be logged with level %s, but it was", levelName, tt.level)
				}
			}
		})
	}
}

func TestInit_InvalidLevel(t *testing.T) {
	output := captureLogOutput(t, func() {
		Init(Config{
			Level:      "invalid",
			Format:     "json",
			TimeFormat: time.RFC3339,
		})

		// With invalid level, should default to InfoLevel
		Log.Debug().Msg("debug message")
		Log.Info().Msg("info message")
	})

	lines := strings.Split(strings.TrimSpace(output), "\n")

	// Debug should NOT be logged (defaults to info level)
	debugFound := false
	infoFound := false

	for _, line := range lines {
		if line == "" {
			continue
		}
		logEntry := parseLogLine(t, line)
		if logEntry != nil {
			if logEntry["level"] == "debug" {
				debugFound = true
			}
			if logEntry["level"] == "info" {
				infoFound = true
			}
		}
	}

	if debugFound {
		t.Error("Debug message should not be logged with invalid level (defaults to info)")
	}

	if !infoFound {
		t.Error("Info message should be logged with invalid level (defaults to info)")
	}
}

// Context Tests

func TestWithRequestID(t *testing.T) {
	ctx := context.Background()
	requestID := "req-12345"

	ctx = WithRequestID(ctx, requestID)

	value := ctx.Value(RequestIDKey)
	if value == nil {
		t.Fatal("Expected request ID in context, got nil")
	}

	if value.(string) != requestID {
		t.Errorf("Expected request ID '%s', got '%s'", requestID, value.(string))
	}
}

func TestWithAuctionID(t *testing.T) {
	ctx := context.Background()
	auctionID := "auction-67890"

	ctx = WithAuctionID(ctx, auctionID)

	value := ctx.Value(AuctionIDKey)
	if value == nil {
		t.Fatal("Expected auction ID in context, got nil")
	}

	if value.(string) != auctionID {
		t.Errorf("Expected auction ID '%s', got '%s'", auctionID, value.(string))
	}
}

func TestFromContext_WithRequestID(t *testing.T) {
	ctx := context.Background()
	requestID := "req-12345"
	ctx = WithRequestID(ctx, requestID)

	output := captureLogOutput(t, func() {
		Init(Config{Level: "info", Format: "json", TimeFormat: time.RFC3339})
		logger := FromContext(ctx)
		logger.Info().Msg("test message")
	})

	logEntry := parseLogLine(t, output)

	if logEntry == nil {
		t.Fatal("Expected log output, got none")
	}

	if logEntry["request_id"] != requestID {
		t.Errorf("Expected request_id '%s', got '%v'", requestID, logEntry["request_id"])
	}
}

func TestFromContext_WithAuctionID(t *testing.T) {
	ctx := context.Background()
	auctionID := "auction-67890"
	ctx = WithAuctionID(ctx, auctionID)

	output := captureLogOutput(t, func() {
		Init(Config{Level: "info", Format: "json", TimeFormat: time.RFC3339})
		logger := FromContext(ctx)
		logger.Info().Msg("test message")
	})

	logEntry := parseLogLine(t, output)

	if logEntry == nil {
		t.Fatal("Expected log output, got none")
	}

	if logEntry["auction_id"] != auctionID {
		t.Errorf("Expected auction_id '%s', got '%v'", auctionID, logEntry["auction_id"])
	}
}

func TestFromContext_WithBothIDs(t *testing.T) {
	ctx := context.Background()
	requestID := "req-12345"
	auctionID := "auction-67890"
	ctx = WithRequestID(ctx, requestID)
	ctx = WithAuctionID(ctx, auctionID)

	output := captureLogOutput(t, func() {
		Init(Config{Level: "info", Format: "json", TimeFormat: time.RFC3339})
		logger := FromContext(ctx)
		logger.Info().Msg("test message")
	})

	logEntry := parseLogLine(t, output)

	if logEntry == nil {
		t.Fatal("Expected log output, got none")
	}

	if logEntry["request_id"] != requestID {
		t.Errorf("Expected request_id '%s', got '%v'", requestID, logEntry["request_id"])
	}

	if logEntry["auction_id"] != auctionID {
		t.Errorf("Expected auction_id '%s', got '%v'", auctionID, logEntry["auction_id"])
	}
}

func TestFromContext_Empty(t *testing.T) {
	ctx := context.Background()

	output := captureLogOutput(t, func() {
		Init(Config{Level: "info", Format: "json", TimeFormat: time.RFC3339})
		logger := FromContext(ctx)
		logger.Info().Msg("test message")
	})

	logEntry := parseLogLine(t, output)

	if logEntry == nil {
		t.Fatal("Expected log output, got none")
	}

	// Should not have request_id or auction_id
	if _, ok := logEntry["request_id"]; ok {
		t.Error("Expected no request_id in empty context")
	}

	if _, ok := logEntry["auction_id"]; ok {
		t.Error("Expected no auction_id in empty context")
	}

	// Should still have service field
	if logEntry["service"] != "pbs" {
		t.Errorf("Expected service 'pbs', got '%v'", logEntry["service"])
	}
}

// Specialized Constructor Tests

func TestAuction(t *testing.T) {
	auctionID := "auction-12345"

	output := captureLogOutput(t, func() {
		Init(Config{Level: "info", Format: "json", TimeFormat: time.RFC3339})
		logger := Auction(auctionID)
		logger.Info().Msg("auction event")
	})

	logEntry := parseLogLine(t, output)

	if logEntry == nil {
		t.Fatal("Expected log output, got none")
	}

	if logEntry["auction_id"] != auctionID {
		t.Errorf("Expected auction_id '%s', got '%v'", auctionID, logEntry["auction_id"])
	}

	if logEntry["message"] != "auction event" {
		t.Errorf("Expected message 'auction event', got '%v'", logEntry["message"])
	}
}

func TestBidder(t *testing.T) {
	bidderCode := "appnexus"

	output := captureLogOutput(t, func() {
		Init(Config{Level: "info", Format: "json", TimeFormat: time.RFC3339})
		logger := Bidder(bidderCode)
		logger.Info().Msg("bidder event")
	})

	logEntry := parseLogLine(t, output)

	if logEntry == nil {
		t.Fatal("Expected log output, got none")
	}

	if logEntry["bidder"] != bidderCode {
		t.Errorf("Expected bidder '%s', got '%v'", bidderCode, logEntry["bidder"])
	}

	if logEntry["message"] != "bidder event" {
		t.Errorf("Expected message 'bidder event', got '%v'", logEntry["message"])
	}
}

func TestHTTP(t *testing.T) {
	output := captureLogOutput(t, func() {
		Init(Config{Level: "info", Format: "json", TimeFormat: time.RFC3339})
		logger := HTTP()
		logger.Info().Msg("http event")
	})

	logEntry := parseLogLine(t, output)

	if logEntry == nil {
		t.Fatal("Expected log output, got none")
	}

	if logEntry["component"] != "http" {
		t.Errorf("Expected component 'http', got '%v'", logEntry["component"])
	}

	if logEntry["message"] != "http event" {
		t.Errorf("Expected message 'http event', got '%v'", logEntry["message"])
	}
}

func TestIDR(t *testing.T) {
	output := captureLogOutput(t, func() {
		Init(Config{Level: "info", Format: "json", TimeFormat: time.RFC3339})
		logger := IDR()
		logger.Info().Msg("idr event")
	})

	logEntry := parseLogLine(t, output)

	if logEntry == nil {
		t.Fatal("Expected log output, got none")
	}

	if logEntry["component"] != "idr" {
		t.Errorf("Expected component 'idr', got '%v'", logEntry["component"])
	}

	if logEntry["message"] != "idr event" {
		t.Errorf("Expected message 'idr event', got '%v'", logEntry["message"])
	}
}

// RequestLogger Tests

func TestNewRequestLogger(t *testing.T) {
	requestID := "req-12345"

	output := captureLogOutput(t, func() {
		Init(Config{Level: "info", Format: "json", TimeFormat: time.RFC3339})
		rl := NewRequestLogger(requestID)
		rl.Info("test message")
	})

	logEntry := parseLogLine(t, output)

	if logEntry == nil {
		t.Fatal("Expected log output, got none")
	}

	if logEntry["request_id"] != requestID {
		t.Errorf("Expected request_id '%s', got '%v'", requestID, logEntry["request_id"])
	}

	if logEntry["message"] != "test message" {
		t.Errorf("Expected message 'test message', got '%v'", logEntry["message"])
	}
}

func TestRequestLogger_Info(t *testing.T) {
	output := captureLogOutput(t, func() {
		Init(Config{Level: "info", Format: "json", TimeFormat: time.RFC3339})
		rl := NewRequestLogger("req-123")
		rl.Info("info message")
	})

	logEntry := parseLogLine(t, output)

	if logEntry == nil {
		t.Fatal("Expected log output, got none")
	}

	if logEntry["level"] != "info" {
		t.Errorf("Expected level 'info', got '%v'", logEntry["level"])
	}

	if logEntry["message"] != "info message" {
		t.Errorf("Expected message 'info message', got '%v'", logEntry["message"])
	}
}

func TestRequestLogger_Error(t *testing.T) {
	testErr := errors.New("test error")

	output := captureLogOutput(t, func() {
		Init(Config{Level: "info", Format: "json", TimeFormat: time.RFC3339})
		rl := NewRequestLogger("req-123")
		rl.Error("error occurred", testErr)
	})

	logEntry := parseLogLine(t, output)

	if logEntry == nil {
		t.Fatal("Expected log output, got none")
	}

	if logEntry["level"] != "error" {
		t.Errorf("Expected level 'error', got '%v'", logEntry["level"])
	}

	if logEntry["message"] != "error occurred" {
		t.Errorf("Expected message 'error occurred', got '%v'", logEntry["message"])
	}

	if logEntry["error"] != "test error" {
		t.Errorf("Expected error 'test error', got '%v'", logEntry["error"])
	}
}

func TestRequestLogger_WithField(t *testing.T) {
	output := captureLogOutput(t, func() {
		Init(Config{Level: "info", Format: "json", TimeFormat: time.RFC3339})
		rl := NewRequestLogger("req-123")
		rl.WithField("user_id", 42).Info("with field")
	})

	logEntry := parseLogLine(t, output)

	if logEntry == nil {
		t.Fatal("Expected log output, got none")
	}

	// JSON numbers are float64
	if logEntry["user_id"] != float64(42) {
		t.Errorf("Expected user_id 42, got '%v'", logEntry["user_id"])
	}

	if logEntry["message"] != "with field" {
		t.Errorf("Expected message 'with field', got '%v'", logEntry["message"])
	}
}

func TestRequestLogger_WithField_Multiple(t *testing.T) {
	output := captureLogOutput(t, func() {
		Init(Config{Level: "info", Format: "json", TimeFormat: time.RFC3339})
		rl := NewRequestLogger("req-123")
		rl.WithField("user_id", 42).
			WithField("action", "login").
			Info("multiple fields")
	})

	logEntry := parseLogLine(t, output)

	if logEntry == nil {
		t.Fatal("Expected log output, got none")
	}

	if logEntry["user_id"] != float64(42) {
		t.Errorf("Expected user_id 42, got '%v'", logEntry["user_id"])
	}

	if logEntry["action"] != "login" {
		t.Errorf("Expected action 'login', got '%v'", logEntry["action"])
	}

	if logEntry["message"] != "multiple fields" {
		t.Errorf("Expected message 'multiple fields', got '%v'", logEntry["message"])
	}
}

func TestRequestLogger_Duration(t *testing.T) {
	rl := NewRequestLogger("req-123")

	// Sleep for a short duration
	time.Sleep(10 * time.Millisecond)

	duration := rl.Duration()

	if duration < 10*time.Millisecond {
		t.Errorf("Expected duration >= 10ms, got %v", duration)
	}

	if duration > 100*time.Millisecond {
		t.Errorf("Expected duration < 100ms, got %v (test environment might be slow)", duration)
	}
}

func TestRequestLogger_LogComplete(t *testing.T) {
	output := captureLogOutput(t, func() {
		Init(Config{Level: "info", Format: "json", TimeFormat: time.RFC3339})
		rl := NewRequestLogger("req-123")
		time.Sleep(10 * time.Millisecond)
		rl.LogComplete(200)
	})

	logEntry := parseLogLine(t, output)

	if logEntry == nil {
		t.Fatal("Expected log output, got none")
	}

	if logEntry["status"] != float64(200) {
		t.Errorf("Expected status 200, got '%v'", logEntry["status"])
	}

	if logEntry["message"] != "request completed" {
		t.Errorf("Expected message 'request completed', got '%v'", logEntry["message"])
	}

	// Check that duration_ms exists and is a number
	if _, ok := logEntry["duration_ms"]; !ok {
		t.Error("Expected duration_ms field in log output")
	}

	// Verify duration is reasonable (should be >= 10ms since we slept)
	// Note: duration_ms is in milliseconds as a floating point number
	if duration, ok := logEntry["duration_ms"].(float64); ok {
		if duration < 10.0 {
			t.Errorf("Expected duration >= 10ms, got %vms", duration)
		}
	}
}
