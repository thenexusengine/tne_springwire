// Package adapters provides the bidder adapter framework
package adapters

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"

	"github.com/thenexusengine/tne_springwire/internal/openrtb"
	"github.com/thenexusengine/tne_springwire/pkg/logger"
)

// maxResponseSize limits bidder response size to prevent OOM attacks
const maxResponseSize = 1024 * 1024 // 1MB

// Adapter defines the interface for bidder adapters
type Adapter interface {
	// MakeRequests builds HTTP requests for the bidder
	MakeRequests(request *openrtb.BidRequest, extraInfo *ExtraRequestInfo) ([]*RequestData, []error)

	// MakeBids parses bidder responses into bids
	MakeBids(request *openrtb.BidRequest, responseData *ResponseData) (*BidderResponse, []error)
}

// ExtraRequestInfo contains additional info for request building
type ExtraRequestInfo struct {
	PbsEntryPoint  string
	GlobalPrivacy  GlobalPrivacy
	BidderCoreName string
}

// GlobalPrivacy contains privacy settings
type GlobalPrivacy struct {
	GDPR        bool
	GDPRConsent string
	CCPA        string
	GPP         string
	GPPSID      []int
}

// RequestData represents an HTTP request to a bidder
type RequestData struct {
	Method  string
	URI     string
	Body    []byte
	Headers http.Header
}

// ResponseData represents an HTTP response from a bidder
type ResponseData struct {
	StatusCode int
	Body       []byte
	Headers    http.Header
}

// BidderResponse contains parsed bids from a bidder
type BidderResponse struct {
	Bids       []*TypedBid
	Currency   string
	ResponseID string // P2-5: Original BidResponse.ID for validation against request
}

// TypedBid is a bid with its type
type TypedBid struct {
	Bid          *openrtb.Bid
	BidType      BidType
	BidVideo     *BidVideo
	BidMeta      *openrtb.ExtBidPrebidMeta
	DealPriority int
}

// BidType represents the type of bid
type BidType string

const (
	BidTypeBanner BidType = "banner"
	BidTypeVideo  BidType = "video"
	BidTypeAudio  BidType = "audio"
	BidTypeNative BidType = "native"
)

// DemandType represents the source of demand for bid obfuscation
type DemandType string

const (
	// DemandTypePlatform indicates TNE platform demand (obfuscated as "thenexusengine")
	DemandTypePlatform DemandType = "platform"
	// DemandTypePublisher indicates publisher's own demand partners (shown transparently)
	DemandTypePublisher DemandType = "publisher"
)

// PlatformSeatName is the obfuscated seat name for platform demand
const PlatformSeatName = "thenexusengine"

// BidVideo contains video-specific bid info
type BidVideo struct {
	Duration        int
	PrimaryCategory string
}

// BidderInfo contains bidder configuration
type BidderInfo struct {
	Enabled                 bool
	Maintainer              *MaintainerInfo
	Capabilities            *CapabilitiesInfo
	ModifyingVastXmlAllowed bool
	Debug                   *DebugInfo
	GVLVendorID             int
	Syncer                  *SyncerInfo
	Endpoint                string
	ExtraInfo               string
	DemandType              DemandType // platform (obfuscated) or publisher (transparent)
}

// MaintainerInfo contains maintainer info
type MaintainerInfo struct {
	Email string
}

// CapabilitiesInfo contains bidder capabilities
type CapabilitiesInfo struct {
	App  *PlatformInfo
	Site *PlatformInfo
}

// PlatformInfo contains platform capabilities
type PlatformInfo struct {
	MediaTypes []BidType
}

// DebugInfo contains debug configuration
type DebugInfo struct {
	AllowDebugOverride bool
}

// SyncerInfo contains user sync configuration
type SyncerInfo struct {
	Supports []string
}

// AdapterConfig holds runtime adapter configuration
type AdapterConfig struct {
	Endpoint  string
	Disabled  bool
	ExtraInfo string
}

// AdapterWithInfo wraps an adapter with its info
type AdapterWithInfo struct {
	Adapter Adapter
	Info    BidderInfo
}

// BidderResult contains bidding results
type BidderResult struct {
	BidderCode string
	Bids       []*TypedBid
	Errors     []error
	Latency    time.Duration
}

// HTTPClient defines the interface for HTTP requests
type HTTPClient interface {
	Do(ctx context.Context, req *RequestData, timeout time.Duration) (*ResponseData, error)
}

// DefaultHTTPClient implements HTTPClient
type DefaultHTTPClient struct {
	client *http.Client
}

// NewHTTPClient creates a new HTTP client with connection pooling
// P1-14: Configure transport for high-performance connection reuse
// Connection pooling reduces latency by reusing TCP connections and TLS sessions
// for repeated requests to the same bidder endpoints.
func NewHTTPClient(timeout time.Duration) *DefaultHTTPClient {
	transport := &http.Transport{
		// Connection pooling settings
		MaxIdleConns:        100,              // Total idle connections across all hosts
		MaxIdleConnsPerHost: 10,               // Idle connections per bidder endpoint
		MaxConnsPerHost:     50,               // Max concurrent connections per host
		IdleConnTimeout:     90 * time.Second, // Keep idle connections for 90s

		// TLS session caching reduces handshake overhead for repeated connections
		TLSClientConfig: &tls.Config{
			ClientSessionCache: tls.NewLRUClientSessionCache(100),
			MinVersion:         tls.VersionTLS12, // Require TLS 1.2+
		},

		// Timeouts for connection establishment
		DialContext: (&net.Dialer{
			Timeout:   5 * time.Second,  // Connection timeout
			KeepAlive: 30 * time.Second, // TCP keepalive interval
		}).DialContext,
		TLSHandshakeTimeout:   5 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		ResponseHeaderTimeout: 10 * time.Second,

		// Disable compression to reduce latency (bidder responses are usually small)
		DisableCompression: true,
	}

	return &DefaultHTTPClient{
		client: &http.Client{
			Timeout:   timeout,
			Transport: transport,
		},
	}
}

// Do executes an HTTP request with proper timeout handling
func (c *DefaultHTTPClient) Do(ctx context.Context, req *RequestData, timeout time.Duration) (*ResponseData, error) {
	// P1-3: Respect parent context deadline - use shorter of parent deadline or specified timeout
	if timeout > 0 {
		if deadline, hasDeadline := ctx.Deadline(); hasDeadline {
			remaining := time.Until(deadline)
			if remaining < timeout {
				timeout = remaining // Use parent's shorter deadline
			}
		}
		// Only create new context if we still have positive timeout
		if timeout > 0 {
			var cancel context.CancelFunc
			ctx, cancel = context.WithTimeout(ctx, timeout)
			defer cancel()
		}
	}

	httpReq, err := http.NewRequestWithContext(ctx, req.Method, req.URI, nil)
	if err != nil {
		return nil, err
	}

	if len(req.Body) > 0 {
		httpReq.Body = &bodyReader{data: req.Body}
		httpReq.ContentLength = int64(len(req.Body))
	}

	for k, v := range req.Headers {
		httpReq.Header[k] = v
	}

	resp, err := c.client.Do(httpReq) //nolint:bodyclose
	if err != nil {
		return nil, err
	}

	// P1-NEW-1: Use single goroutine for entire read with proper cleanup on cancellation
	// This prevents goroutine leaks that occurred when spawning per-read goroutines
	type readResult struct {
		data []byte
		err  error
	}
	readCh := make(chan readResult, 1)

	// Single goroutine for the entire read operation
	go func() {
		defer resp.Body.Close()
		// Read with size limit to prevent OOM from malicious bidders
		limitedReader := io.LimitReader(resp.Body, maxResponseSize+1) // +1 to detect overflow
		data, err := io.ReadAll(limitedReader)
		readCh <- readResult{data: data, err: err}
	}()

	// Wait for read completion or context cancellation
	select {
	case <-ctx.Done():
		// Close response body to unblock the read goroutine
		resp.Body.Close()
		// P1-NEW-2: Drain channel and log any unexpected errors for debugging
		// This helps diagnose bidder issues that occur during timeout/cancellation
		result := <-readCh
		if result.err != nil && !errors.Is(result.err, io.EOF) {
			// Log non-EOF errors that occurred during cancellation for debugging
			// These are typically network errors masked by the context cancellation
			logger.Log.Debug().
				Err(result.err).
				Str("uri", req.URI).
				Msg("read error during context cancellation (masked by timeout)")
		}
		return nil, ctx.Err()
	case result := <-readCh:
		if result.err != nil {
			return nil, result.err
		}
		if len(result.data) > maxResponseSize {
			return nil, fmt.Errorf("response too large: exceeded %d bytes", maxResponseSize)
		}
		return &ResponseData{
			StatusCode: resp.StatusCode,
			Body:       result.data,
			Headers:    resp.Header,
		}, nil
	}
}

// bodyReader wraps bytes for http.Request.Body
type bodyReader struct {
	data []byte
	pos  int
}

// Read implements io.Reader with proper EOF handling
func (r *bodyReader) Read(p []byte) (n int, err error) {
	if r.pos >= len(r.data) {
		return 0, io.EOF
	}
	n = copy(p, r.data[r.pos:])
	r.pos += n
	// Return io.EOF along with the final bytes if we've reached the end
	if r.pos >= len(r.data) {
		return n, io.EOF
	}
	return n, nil
}

// Close implements io.Closer
func (r *bodyReader) Close() error {
	return nil
}
