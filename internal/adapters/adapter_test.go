package adapters

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestNewHTTPClient(t *testing.T) {
	client := NewHTTPClient(5 * time.Second)
	if client == nil {
		t.Fatal("expected non-nil client")
	}
	if client.client == nil {
		t.Fatal("expected non-nil http.Client")
	}
	if client.client.Timeout != 5*time.Second {
		t.Errorf("expected 5s timeout, got %v", client.client.Timeout)
	}
}

func TestHTTPClientDo_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("expected application/json header")
		}
		w.Header().Set("X-Test", "value")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"seatbid":[]}`))
	}))
	defer server.Close()

	client := NewHTTPClient(5 * time.Second)
	req := &RequestData{
		Method:  "POST",
		URI:     server.URL,
		Body:    []byte(`{"id":"test"}`),
		Headers: http.Header{"Content-Type": []string{"application/json"}},
	}

	resp, err := client.Do(context.Background(), req, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
	if string(resp.Body) != `{"seatbid":[]}` {
		t.Errorf("unexpected body: %s", resp.Body)
	}
	if resp.Headers.Get("X-Test") != "value" {
		t.Errorf("expected X-Test header")
	}
}

func TestHTTPClientDo_ContextTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(500 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewHTTPClient(5 * time.Second)
	req := &RequestData{
		Method: "GET",
		URI:    server.URL,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err := client.Do(ctx, req, 0)
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if !strings.Contains(err.Error(), "context") {
		t.Errorf("expected context error, got: %v", err)
	}
}

func TestHTTPClientDo_RequestTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(500 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewHTTPClient(5 * time.Second)
	req := &RequestData{
		Method: "GET",
		URI:    server.URL,
	}

	_, err := client.Do(context.Background(), req, 50*time.Millisecond)
	if err == nil {
		t.Fatal("expected timeout error")
	}
}

func TestHTTPClientDo_ParentDeadlineRespected(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(500 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewHTTPClient(5 * time.Second)
	req := &RequestData{
		Method: "GET",
		URI:    server.URL,
	}

	// Parent context has shorter deadline than request timeout
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	start := time.Now()
	_, err := client.Do(ctx, req, 2*time.Second) // Request timeout is 2s, but parent is 50ms
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected timeout error")
	}
	// Should timeout around 50ms (parent), not 2s (request)
	if elapsed > 200*time.Millisecond {
		t.Errorf("expected timeout around 50ms, took %v", elapsed)
	}
}

func TestHTTPClientDo_ResponseTooLarge(t *testing.T) {
	// Create a response larger than maxResponseSize (1MB)
	largeBody := strings.Repeat("x", maxResponseSize+100)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(largeBody))
	}))
	defer server.Close()

	client := NewHTTPClient(5 * time.Second)
	req := &RequestData{
		Method: "GET",
		URI:    server.URL,
	}

	_, err := client.Do(context.Background(), req, 0)
	if err == nil {
		t.Fatal("expected error for large response")
	}
	if !strings.Contains(err.Error(), "response too large") {
		t.Errorf("expected 'response too large' error, got: %v", err)
	}
}

func TestHTTPClientDo_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("internal error"))
	}))
	defer server.Close()

	client := NewHTTPClient(5 * time.Second)
	req := &RequestData{
		Method: "GET",
		URI:    server.URL,
	}

	resp, err := client.Do(context.Background(), req, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", resp.StatusCode)
	}
	if string(resp.Body) != "internal error" {
		t.Errorf("unexpected body: %s", resp.Body)
	}
}

func TestHTTPClientDo_InvalidURL(t *testing.T) {
	client := NewHTTPClient(5 * time.Second)
	req := &RequestData{
		Method: "GET",
		URI:    "://invalid-url",
	}

	_, err := client.Do(context.Background(), req, 0)
	if err == nil {
		t.Fatal("expected error for invalid URL")
	}
}

func TestHTTPClientDo_ConnectionRefused(t *testing.T) {
	client := NewHTTPClient(100 * time.Millisecond)
	req := &RequestData{
		Method: "GET",
		URI:    "http://127.0.0.1:1", // Port 1 should be closed
	}

	_, err := client.Do(context.Background(), req, 100*time.Millisecond)
	if err == nil {
		t.Fatal("expected connection error")
	}
}

func TestBodyReader_Read(t *testing.T) {
	data := []byte("hello world")
	reader := &bodyReader{data: data}

	// Read in chunks of 5 bytes
	buf := make([]byte, 5)

	// First read: "hello"
	n, err := reader.Read(buf)
	if err != nil {
		t.Errorf("unexpected error on first read: %v", err)
	}
	if n != 5 {
		t.Errorf("expected 5 bytes, got %d", n)
	}
	if string(buf[:n]) != "hello" {
		t.Errorf("expected 'hello', got '%s'", buf[:n])
	}

	// Second read: " worl"
	n, err = reader.Read(buf)
	if err != nil {
		t.Errorf("unexpected error on second read: %v", err)
	}
	if n != 5 {
		t.Errorf("expected 5 bytes, got %d", n)
	}
	if string(buf[:n]) != " worl" {
		t.Errorf("expected ' worl', got '%s'", buf[:n])
	}

	// Third read: "d" with EOF
	n, err = reader.Read(buf)
	if !errors.Is(err, io.EOF) {
		t.Errorf("expected EOF on third read, got %v", err)
	}
	if n != 1 {
		t.Errorf("expected 1 byte, got %d", n)
	}
	if string(buf[:n]) != "d" {
		t.Errorf("expected 'd', got '%s'", buf[:n])
	}

	// Fourth read: EOF only
	n, err = reader.Read(buf)
	if !errors.Is(err, io.EOF) {
		t.Errorf("expected EOF, got %v", err)
	}
	if n != 0 {
		t.Errorf("expected 0 bytes, got %d", n)
	}
}

func TestBodyReader_ReadAll(t *testing.T) {
	data := []byte("test data for reading")
	reader := &bodyReader{data: data}

	result, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(result) != string(data) {
		t.Errorf("expected '%s', got '%s'", data, result)
	}
}

func TestBodyReader_Empty(t *testing.T) {
	reader := &bodyReader{data: []byte{}}

	buf := make([]byte, 10)
	n, err := reader.Read(buf)
	if !errors.Is(err, io.EOF) {
		t.Errorf("expected EOF, got %v", err)
	}
	if n != 0 {
		t.Errorf("expected 0 bytes, got %d", n)
	}
}

func TestBodyReader_Close(t *testing.T) {
	reader := &bodyReader{data: []byte("test")}
	err := reader.Close()
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestRequestData_Fields(t *testing.T) {
	headers := http.Header{}
	headers.Set("Content-Type", "application/json")

	req := &RequestData{
		Method:  "POST",
		URI:     "http://example.com/bid",
		Body:    []byte(`{"id":"1"}`),
		Headers: headers,
	}

	if req.Method != "POST" {
		t.Errorf("expected POST, got %s", req.Method)
	}
	if req.URI != "http://example.com/bid" {
		t.Errorf("unexpected URI: %s", req.URI)
	}
	if string(req.Body) != `{"id":"1"}` {
		t.Errorf("unexpected body: %s", req.Body)
	}
	if req.Headers.Get("Content-Type") != "application/json" {
		t.Error("expected Content-Type header")
	}
}

func TestResponseData_Fields(t *testing.T) {
	headers := http.Header{}
	headers.Set("X-Custom", "value")

	resp := &ResponseData{
		StatusCode: 200,
		Body:       []byte(`{"seatbid":[]}`),
		Headers:    headers,
	}

	if resp.StatusCode != 200 {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
	if string(resp.Body) != `{"seatbid":[]}` {
		t.Errorf("unexpected body: %s", resp.Body)
	}
	if resp.Headers.Get("X-Custom") != "value" {
		t.Error("expected X-Custom header")
	}
}

func TestBidType_Constants(t *testing.T) {
	if BidTypeBanner != "banner" {
		t.Errorf("expected 'banner', got '%s'", BidTypeBanner)
	}
	if BidTypeVideo != "video" {
		t.Errorf("expected 'video', got '%s'", BidTypeVideo)
	}
	if BidTypeAudio != "audio" {
		t.Errorf("expected 'audio', got '%s'", BidTypeAudio)
	}
	if BidTypeNative != "native" {
		t.Errorf("expected 'native', got '%s'", BidTypeNative)
	}
}

func TestBidderResponse_Fields(t *testing.T) {
	resp := &BidderResponse{
		Bids:       []*TypedBid{},
		Currency:   "USD",
		ResponseID: "resp-123",
	}

	if resp.Currency != "USD" {
		t.Errorf("expected USD, got %s", resp.Currency)
	}
	if resp.ResponseID != "resp-123" {
		t.Errorf("expected resp-123, got %s", resp.ResponseID)
	}
}

func TestBidderInfo_Fields(t *testing.T) {
	info := BidderInfo{
		Enabled:     true,
		GVLVendorID: 123,
		Endpoint:    "http://bidder.example.com",
		Maintainer: &MaintainerInfo{
			Email: "test@example.com",
		},
		Capabilities: &CapabilitiesInfo{
			Site: &PlatformInfo{
				MediaTypes: []BidType{BidTypeBanner, BidTypeVideo},
			},
		},
	}

	if !info.Enabled {
		t.Error("expected enabled")
	}
	if info.GVLVendorID != 123 {
		t.Errorf("expected 123, got %d", info.GVLVendorID)
	}
	if info.Maintainer.Email != "test@example.com" {
		t.Error("unexpected maintainer email")
	}
	if len(info.Capabilities.Site.MediaTypes) != 2 {
		t.Error("expected 2 media types")
	}
}

func TestAdapterConfig_Fields(t *testing.T) {
	config := AdapterConfig{
		Endpoint:  "http://bidder.example.com",
		Disabled:  false,
		ExtraInfo: `{"key":"value"}`,
	}

	if config.Disabled {
		t.Error("expected not disabled")
	}
	if config.Endpoint != "http://bidder.example.com" {
		t.Error("unexpected endpoint")
	}
}

func TestBidderResult_Fields(t *testing.T) {
	result := BidderResult{
		BidderCode: "appnexus",
		Bids:       []*TypedBid{},
		Errors:     []error{},
		Latency:    100 * time.Millisecond,
	}

	if result.BidderCode != "appnexus" {
		t.Errorf("expected appnexus, got %s", result.BidderCode)
	}
	if result.Latency != 100*time.Millisecond {
		t.Errorf("expected 100ms, got %v", result.Latency)
	}
}

func TestExtraRequestInfo_Fields(t *testing.T) {
	info := &ExtraRequestInfo{
		PbsEntryPoint:  "/openrtb2/auction",
		BidderCoreName: "appnexus",
		GlobalPrivacy: GlobalPrivacy{
			GDPR:        true,
			GDPRConsent: "consent-string",
			CCPA:        "1YNN",
			GPP:         "gpp-string",
			GPPSID:      []int{1, 2},
		},
	}

	if info.PbsEntryPoint != "/openrtb2/auction" {
		t.Error("unexpected entry point")
	}
	if !info.GlobalPrivacy.GDPR {
		t.Error("expected GDPR true")
	}
	if info.GlobalPrivacy.CCPA != "1YNN" {
		t.Error("unexpected CCPA")
	}
}
