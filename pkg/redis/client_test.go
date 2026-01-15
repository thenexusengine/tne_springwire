package redis

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
)

func setupTestRedis(t *testing.T) (*miniredis.Miniredis, string) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("Failed to start miniredis: %v", err)
	}
	return mr, "redis://" + mr.Addr()
}

func TestNew_Success(t *testing.T) {
	mr, redisURL := setupTestRedis(t)
	defer mr.Close()

	client, err := New(redisURL)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	if client == nil {
		t.Fatal("Expected non-nil client")
	}

	// Test connection
	ctx := context.Background()
	if err := client.Ping(ctx); err != nil {
		t.Errorf("Ping failed: %v", err)
	}
}

func TestNew_EmptyURL(t *testing.T) {
	client, err := New("")
	if err == nil {
		t.Error("Expected error for empty URL")
	}
	if client != nil {
		t.Error("Expected nil client on error")
	}
}

func TestNew_InvalidURL(t *testing.T) {
	client, err := New("not-a-valid-redis-url")
	if err == nil {
		t.Error("Expected error for invalid URL")
	}
	if client != nil {
		t.Error("Expected nil client on error")
	}
}

func TestNewWithConfig_Success(t *testing.T) {
	mr, redisURL := setupTestRedis(t)
	defer mr.Close()

	cfg := &ClientConfig{
		PoolSize:     50,
		MinIdleConns: 5,
		MaxConnAge:   10 * time.Minute,
		DialTimeout:  2 * time.Second,
		ReadTimeout:  2 * time.Second,
		WriteTimeout: 2 * time.Second,
		PoolTimeout:  2 * time.Second,
	}

	client, err := NewWithConfig(redisURL, cfg)
	if err != nil {
		t.Fatalf("Failed to create client with config: %v", err)
	}
	defer client.Close()

	if client == nil {
		t.Fatal("Expected non-nil client")
	}

	// Verify connection works
	ctx := context.Background()
	if err := client.Ping(ctx); err != nil {
		t.Errorf("Ping failed: %v", err)
	}
}

func TestNewWithConfig_NilConfig(t *testing.T) {
	mr, redisURL := setupTestRedis(t)
	defer mr.Close()

	// Should use default config when nil
	client, err := NewWithConfig(redisURL, nil)
	if err != nil {
		t.Fatalf("Failed to create client with nil config: %v", err)
	}
	defer client.Close()

	if client == nil {
		t.Fatal("Expected non-nil client")
	}
}

func TestNewWithConfig_EmptyURL(t *testing.T) {
	cfg := DefaultClientConfig()
	client, err := NewWithConfig("", cfg)
	if err == nil {
		t.Error("Expected error for empty URL")
	}
	if client != nil {
		t.Error("Expected nil client on error")
	}
}

func TestNewWithConfig_InvalidURL(t *testing.T) {
	cfg := DefaultClientConfig()
	client, err := NewWithConfig("invalid-url", cfg)
	if err == nil {
		t.Error("Expected error for invalid URL")
	}
	if client != nil {
		t.Error("Expected nil client on error")
	}
}

func TestDefaultClientConfig(t *testing.T) {
	cfg := DefaultClientConfig()
	if cfg == nil {
		t.Fatal("Expected non-nil config")
	}

	if cfg.PoolSize != 100 {
		t.Errorf("Expected PoolSize 100, got %d", cfg.PoolSize)
	}
	if cfg.MinIdleConns != 10 {
		t.Errorf("Expected MinIdleConns 10, got %d", cfg.MinIdleConns)
	}
	if cfg.MaxConnAge != 30*time.Minute {
		t.Errorf("Expected MaxConnAge 30min, got %v", cfg.MaxConnAge)
	}
}

func TestClient_HGet_Success(t *testing.T) {
	mr, redisURL := setupTestRedis(t)
	defer mr.Close()

	client, err := New(redisURL)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	ctx := context.Background()

	// Set a value using miniredis
	mr.HSet("test-hash", "field1", "value1")

	// Get the value
	result, err := client.HGet(ctx, "test-hash", "field1")
	if err != nil {
		t.Fatalf("HGet failed: %v", err)
	}
	if result != "value1" {
		t.Errorf("Expected 'value1', got '%s'", result)
	}
}

func TestClient_HGet_NotFound(t *testing.T) {
	mr, redisURL := setupTestRedis(t)
	defer mr.Close()

	client, err := New(redisURL)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	ctx := context.Background()

	// Get non-existent field
	result, err := client.HGet(ctx, "nonexistent", "field1")
	if err != nil {
		t.Errorf("Expected no error for missing key, got: %v", err)
	}
	if result != "" {
		t.Errorf("Expected empty string for missing key, got '%s'", result)
	}
}

func TestClient_HGetAll_Success(t *testing.T) {
	mr, redisURL := setupTestRedis(t)
	defer mr.Close()

	client, err := New(redisURL)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	ctx := context.Background()

	// Set multiple values
	mr.HSet("test-hash", "field1", "value1")
	mr.HSet("test-hash", "field2", "value2")

	// Get all values
	result, err := client.HGetAll(ctx, "test-hash")
	if err != nil {
		t.Fatalf("HGetAll failed: %v", err)
	}

	if len(result) != 2 {
		t.Fatalf("Expected 2 fields, got %d", len(result))
	}
	if result["field1"] != "value1" {
		t.Errorf("Expected 'value1', got '%s'", result["field1"])
	}
	if result["field2"] != "value2" {
		t.Errorf("Expected 'value2', got '%s'", result["field2"])
	}
}

func TestClient_HGetAll_Empty(t *testing.T) {
	mr, redisURL := setupTestRedis(t)
	defer mr.Close()

	client, err := New(redisURL)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	ctx := context.Background()

	// Get from non-existent hash
	result, err := client.HGetAll(ctx, "nonexistent")
	if err != nil {
		t.Fatalf("HGetAll failed: %v", err)
	}

	if len(result) != 0 {
		t.Errorf("Expected empty map, got %d entries", len(result))
	}
}

func TestClient_HSet_Success(t *testing.T) {
	mr, redisURL := setupTestRedis(t)
	defer mr.Close()

	client, err := New(redisURL)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	ctx := context.Background()

	// Set a value
	err = client.HSet(ctx, "test-hash", "field1", "value1")
	if err != nil {
		t.Fatalf("HSet failed: %v", err)
	}

	// Verify using miniredis
	result := mr.HGet("test-hash", "field1")
	if result != "value1" {
		t.Errorf("Expected 'value1', got '%s'", result)
	}
}

func TestClient_HSet_Integer(t *testing.T) {
	mr, redisURL := setupTestRedis(t)
	defer mr.Close()

	client, err := New(redisURL)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	ctx := context.Background()

	// Set an integer value
	err = client.HSet(ctx, "test-hash", "count", 42)
	if err != nil {
		t.Fatalf("HSet failed: %v", err)
	}

	// Verify using miniredis
	result := mr.HGet("test-hash", "count")
	if result != "42" {
		t.Errorf("Expected '42', got '%s'", result)
	}
}

func TestClient_HDel_Success(t *testing.T) {
	mr, redisURL := setupTestRedis(t)
	defer mr.Close()

	client, err := New(redisURL)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	ctx := context.Background()

	// Set values
	mr.HSet("test-hash", "field1", "value1")
	mr.HSet("test-hash", "field2", "value2")
	mr.HSet("test-hash", "field3", "value3")

	// Delete fields
	err = client.HDel(ctx, "test-hash", "field1", "field2")
	if err != nil {
		t.Fatalf("HDel failed: %v", err)
	}

	// Verify deletion using miniredis - deleted fields return empty string
	if mr.HGet("test-hash", "field1") != "" {
		t.Error("Expected field1 to be deleted")
	}
	if mr.HGet("test-hash", "field2") != "" {
		t.Error("Expected field2 to be deleted")
	}

	// field3 should still exist
	result := mr.HGet("test-hash", "field3")
	if result != "value3" {
		t.Errorf("Expected 'value3', got '%s'", result)
	}
}

func TestClient_SMembers_Success(t *testing.T) {
	mr, redisURL := setupTestRedis(t)
	defer mr.Close()

	client, err := New(redisURL)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	ctx := context.Background()

	// Add members to set
	mr.SetAdd("test-set", "member1", "member2", "member3")

	// Get members
	members, err := client.SMembers(ctx, "test-set")
	if err != nil {
		t.Fatalf("SMembers failed: %v", err)
	}

	if len(members) != 3 {
		t.Fatalf("Expected 3 members, got %d", len(members))
	}

	// Check all members are present (order not guaranteed)
	memberMap := make(map[string]bool)
	for _, m := range members {
		memberMap[m] = true
	}

	if !memberMap["member1"] || !memberMap["member2"] || !memberMap["member3"] {
		t.Errorf("Missing expected members, got: %v", members)
	}
}

func TestClient_SMembers_Empty(t *testing.T) {
	mr, redisURL := setupTestRedis(t)
	defer mr.Close()

	client, err := New(redisURL)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	ctx := context.Background()

	// Get members from non-existent set
	members, err := client.SMembers(ctx, "nonexistent")
	if err != nil {
		t.Fatalf("SMembers failed: %v", err)
	}

	if len(members) != 0 {
		t.Errorf("Expected empty slice, got %d members", len(members))
	}
}

func TestClient_Ping_Success(t *testing.T) {
	mr, redisURL := setupTestRedis(t)
	defer mr.Close()

	client, err := New(redisURL)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	ctx := context.Background()

	err = client.Ping(ctx)
	if err != nil {
		t.Errorf("Ping failed: %v", err)
	}
}

func TestClient_Ping_AfterServerClosed(t *testing.T) {
	mr, redisURL := setupTestRedis(t)

	client, err := New(redisURL)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	// Close the server
	mr.Close()

	ctx := context.Background()

	// Ping should fail
	err = client.Ping(ctx)
	if err == nil {
		t.Error("Expected error when pinging closed server")
	}
}

func TestClient_Close(t *testing.T) {
	mr, redisURL := setupTestRedis(t)
	defer mr.Close()

	client, err := New(redisURL)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	err = client.Close()
	if err != nil {
		t.Errorf("Close failed: %v", err)
	}
}

func TestClient_PoolStats(t *testing.T) {
	mr, redisURL := setupTestRedis(t)
	defer mr.Close()

	client, err := New(redisURL)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	stats := client.PoolStats()
	if stats == nil {
		t.Error("Expected non-nil pool stats")
	}
}

func TestClient_Do_Success(t *testing.T) {
	mr, redisURL := setupTestRedis(t)
	defer mr.Close()

	client, err := New(redisURL)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	ctx := context.Background()

	// Use Do to execute SET command
	cmd := client.Do(ctx, "SET", "test-key", "test-value")
	if cmd.Err() != nil {
		t.Fatalf("Do SET failed: %v", cmd.Err())
	}

	// Verify using GET
	cmd = client.Do(ctx, "GET", "test-key")
	if cmd.Err() != nil {
		t.Fatalf("Do GET failed: %v", cmd.Err())
	}

	result, err := cmd.Text()
	if err != nil {
		t.Fatalf("Failed to get result text: %v", err)
	}

	if result != "test-value" {
		t.Errorf("Expected 'test-value', got '%s'", result)
	}
}

func TestClient_HGet_ClosedConnection(t *testing.T) {
	mr, redisURL := setupTestRedis(t)
	defer mr.Close()

	client, err := New(redisURL)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	// Close the client
	client.Close()

	ctx := context.Background()

	// Operations should fail after close
	_, err = client.HGet(ctx, "test", "field")
	if err == nil {
		t.Error("Expected error after client close")
	}
}

func TestClient_MultipleOperations(t *testing.T) {
	mr, redisURL := setupTestRedis(t)
	defer mr.Close()

	client, err := New(redisURL)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	ctx := context.Background()

	// Set multiple hash fields
	err = client.HSet(ctx, "user:1", "name", "Alice")
	if err != nil {
		t.Fatalf("HSet name failed: %v", err)
	}
	err = client.HSet(ctx, "user:1", "email", "alice@example.com")
	if err != nil {
		t.Fatalf("HSet email failed: %v", err)
	}
	err = client.HSet(ctx, "user:1", "age", 30)
	if err != nil {
		t.Fatalf("HSet age failed: %v", err)
	}

	// Get all fields
	all, err := client.HGetAll(ctx, "user:1")
	if err != nil {
		t.Fatalf("HGetAll failed: %v", err)
	}

	if len(all) != 3 {
		t.Fatalf("Expected 3 fields, got %d", len(all))
	}

	if all["name"] != "Alice" {
		t.Errorf("Expected name 'Alice', got '%s'", all["name"])
	}
	if all["email"] != "alice@example.com" {
		t.Errorf("Expected email 'alice@example.com', got '%s'", all["email"])
	}
	if all["age"] != "30" {
		t.Errorf("Expected age '30', got '%s'", all["age"])
	}

	// Get single field
	name, err := client.HGet(ctx, "user:1", "name")
	if err != nil {
		t.Fatalf("HGet failed: %v", err)
	}
	if name != "Alice" {
		t.Errorf("Expected 'Alice', got '%s'", name)
	}

	// Delete a field
	err = client.HDel(ctx, "user:1", "email")
	if err != nil {
		t.Fatalf("HDel failed: %v", err)
	}

	// Verify deletion
	email, err := client.HGet(ctx, "user:1", "email")
	if err != nil {
		t.Fatalf("HGet after delete failed: %v", err)
	}
	if email != "" {
		t.Errorf("Expected empty string for deleted field, got '%s'", email)
	}

	// Other fields should still exist
	all, err = client.HGetAll(ctx, "user:1")
	if err != nil {
		t.Fatalf("HGetAll after delete failed: %v", err)
	}
	if len(all) != 2 {
		t.Errorf("Expected 2 fields after delete, got %d", len(all))
	}
}
