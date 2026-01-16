// Package endpoints provides HTTP endpoint handlers
package endpoints

import (
	"encoding/json"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	log "github.com/rs/zerolog/log"

	"github.com/thenexusengine/tne_springwire/internal/exchange"
	"github.com/thenexusengine/tne_springwire/internal/openrtb"
	"github.com/thenexusengine/tne_springwire/pkg/logger"
)

// maxRequestBodySize limits request body reads to prevent OOM attacks (1MB)
const maxRequestBodySize = 1024 * 1024

// debugRequiresAuth controls whether debug mode requires authentication
// P2-1: Enabled by default to prevent information disclosure
var debugRequiresAuth = os.Getenv("DEBUG_REQUIRES_AUTH") != "false"

// Context key for authenticated publisher ID (set by auth middleware)
type contextKey string

const publisherIDContextKey contextKey = "publisher_id"

// SetPublisherID sets the authenticated publisher ID in request context
// This should only be called by auth middleware after validating the API key
func SetPublisherID(ctx context.Context, publisherID string) context.Context {
	return context.WithValue(ctx, publisherIDContextKey, publisherID)
}

// GetPublisherID retrieves the authenticated publisher ID from context
func GetPublisherID(ctx context.Context) (string, bool) {
	publisherID, ok := ctx.Value(publisherIDContextKey).(string)
	return publisherID, ok && publisherID != ""
}

// AuctionHandler handles /openrtb2/auction requests
type AuctionHandler struct {
	exchange *exchange.Exchange
}

// NewAuctionHandler creates a new auction handler
func NewAuctionHandler(ex *exchange.Exchange) *AuctionHandler {
	return &AuctionHandler{exchange: ex}
}

// ServeHTTP handles the auction request
func (h *AuctionHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Read request body with size limit to prevent OOM attacks
	defer r.Body.Close()
	body, err := io.ReadAll(io.LimitReader(r.Body, maxRequestBodySize))
	if err != nil {
		writeError(w, "Failed to read request body", http.StatusBadRequest)
		return
	}

	// Parse OpenRTB request
	var bidRequest openrtb.BidRequest
	err = json.Unmarshal(body, &bidRequest)
	if err != nil {
		logger.Log.Warn().Err(err).Msg("Invalid JSON in bid request")
		writeError(w, "Invalid JSON in request body", http.StatusBadRequest)
		return
	}

	// Validate request
	err = validateBidRequest(&bidRequest)
	if err != nil {
		writeError(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Build auction request
	// P2-1: Debug mode requires authentication to prevent information disclosure
	debugRequested := r.URL.Query().Get("debug") == "1"
	debugEnabled := false
	if debugRequested {
		if debugRequiresAuth {
			// Check for API key in headers
			if hasAPIKey(r) {
				debugEnabled = true
			} else {
				logger.Log.Debug().Msg("Debug mode requested without authentication, ignoring")
			}
		} else {
			debugEnabled = true
		}
	}

	auctionReq := &exchange.AuctionRequest{
		BidRequest: &bidRequest,
		Debug:      debugEnabled,
	}

	// Run auction
	ctx := r.Context()
	auctionStart := time.Now()
	result, err := h.exchange.RunAuction(ctx, auctionReq)
	auctionDuration := time.Since(auctionStart)

	if err != nil {
		// Determine if this is a validation error (client error) or server error
		statusCode := http.StatusInternalServerError
		errorMsg := "Internal server error"

		// Check if error is a ValidationError (client-side error)
		var validationErr *exchange.ValidationError
		if errors.As(err, &validationErr) {
			statusCode = http.StatusBadRequest
			errorMsg = validationErr.Message
		}

		logger.Log.Error().
			Err(err).
			Str("request_id", bidRequest.ID).
			Int("imp_count", len(bidRequest.Imp)).
			Dur("duration_ms", auctionDuration).
			Int("status_code", statusCode).
			Msg("Auction failed")

		// Log to dashboard
		LogAuction(bidRequest.ID, len(bidRequest.Imp), 0, nil, auctionDuration, false, err)

		writeError(w, errorMsg, statusCode)
		return
	}

	// Log successful auction with key metrics
	bidCount := 0
	winningBidders := make([]string, 0)
	if result.BidResponse != nil {
		for _, seatBid := range result.BidResponse.SeatBid {
			bidCount += len(seatBid.Bid)
			if len(seatBid.Bid) > 0 && seatBid.Seat != "" {
				winningBidders = append(winningBidders, seatBid.Seat)
			}
		}
	}

	logger.Log.Info().
		Str("request_id", bidRequest.ID).
		Int("imp_count", len(bidRequest.Imp)).
		Int("bid_count", bidCount).
		Strs("winning_bidders", winningBidders).
		Dur("duration_ms", auctionDuration).
		Bool("debug", auctionReq.Debug).
		Msg("Auction completed")

	// Log to dashboard
	LogAuction(bidRequest.ID, len(bidRequest.Imp), bidCount, winningBidders, auctionDuration, true, nil)

	// Build response with extensions
	response := result.BidResponse
	if auctionReq.Debug && result.DebugInfo != nil {
		// Add debug info to extension
		ext := buildResponseExt(result)
		if extBytes, err := json.Marshal(ext); err == nil {
			response.Ext = extBytes
		}
	}

	// Write response
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Error().Err(err).Str("request_id", bidRequest.ID).Msg("failed to encode auction response")
	}
}

// validateBidRequest validates the bid request
func validateBidRequest(req *openrtb.BidRequest) error {
	if req.ID == "" {
		return &ValidationError{Field: "id", Message: "required"}
	}
	if len(req.Imp) == 0 {
		return &ValidationError{Field: "imp", Message: "at least one impression required"}
	}
	for i, imp := range req.Imp {
		idx := i
		if imp.ID == "" {
			return &ValidationError{Field: "imp[].id", Message: "required", Index: &idx}
		}
		if imp.Banner == nil && imp.Video == nil && imp.Native == nil && imp.Audio == nil {
			return &ValidationError{Field: "imp[].banner|video|native|audio", Message: "at least one media type required", Index: &idx}
		}
	}
	return nil
}

// ValidationError represents a validation error
type ValidationError struct {
	Field   string
	Message string
	Index   *int // nil means no index (non-array field)
}

func (e *ValidationError) Error() string {
	if e.Index != nil && *e.Index >= 0 {
		return fmt.Sprintf("%s[%d]: %s", e.Field, *e.Index, e.Message)
	}
	return e.Field + ": " + e.Message
}

// buildResponseExt builds response extensions with debug info
func buildResponseExt(result *exchange.AuctionResponse) *openrtb.BidResponseExt {
	ext := &openrtb.BidResponseExt{
		ResponseTimeMillis: make(map[string]int),
		Errors:             make(map[string][]openrtb.ExtBidderMessage),
	}

	if result.DebugInfo != nil {
		for bidder, latency := range result.DebugInfo.BidderLatencies {
			ext.ResponseTimeMillis[bidder] = int(latency.Milliseconds())
		}

		for bidder, errs := range result.DebugInfo.Errors {
			messages := make([]openrtb.ExtBidderMessage, len(errs))
			for i, e := range errs {
				messages[i] = openrtb.ExtBidderMessage{Code: 1, Message: e}
			}
			ext.Errors[bidder] = messages
		}

		ext.TMMaxRequest = int(result.DebugInfo.TotalLatency.Milliseconds())
	}

	return ext
}

// writeError writes an error response
func writeError(w http.ResponseWriter, message string, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(map[string]string{"error": message}); err != nil {
		log.Error().Err(err).Str("message", message).Msg("failed to encode error response")
	}
}

// hasAPIKey checks if request has valid API key
// P2-1: Used to gate debug mode access
func hasAPIKey(r *http.Request) bool {
	// Check context first (secure - can't be spoofed by client)
	if publisherID, ok := GetPublisherID(r.Context()); ok && publisherID != "" {
		return true
	}

	// Fallback: check if auth middleware set X-Publisher-ID header
	// SECURITY NOTE: Auth middleware should strip incoming X-Publisher-ID headers
	// to prevent header injection attacks
	publisherIDHeader := r.Header.Get("X-Publisher-ID")
	if publisherIDHeader != "" && len(publisherIDHeader) > 0 {
		// Additional validation: publisher ID should look like a valid ID
		// (not just "1" or "test" which could be injected)
		// Basic check: should be alphanumeric and reasonable length
		if len(publisherIDHeader) >= 8 {
			return true
		}
		logger.Log.Warn().
			Str("publisher_id", publisherIDHeader).
			Msg("Rejecting suspicious X-Publisher-ID header (too short, possible injection attempt)")
	}

	return false
}

// StatusHandler handles /status requests
type StatusHandler struct{}

// NewStatusHandler creates a new status handler
func NewStatusHandler() *StatusHandler {
	return &StatusHandler{}
}

// ServeHTTP handles status requests
func (h *StatusHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]interface{}{
		"status":    "ok",
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	}); err != nil {
		log.Error().Err(err).Msg("failed to encode status response")
	}
}

// BidderLister is an interface for listing bidders
type BidderLister interface {
	ListBidders() []string
}

// InfoBiddersHandler handles /info/bidders requests
type InfoBiddersHandler struct {
	staticRegistry BidderLister
}

// NewInfoBiddersHandler creates a new bidders info handler from a static list.
// Deprecated: Use NewDynamicInfoBiddersHandler instead.
func NewInfoBiddersHandler(bidders []string) *InfoBiddersHandler {
	logger.Log.Warn().
		Int("bidder_count", len(bidders)).
		Msg("NewInfoBiddersHandler is deprecated - use NewDynamicInfoBiddersHandler instead")
	return &InfoBiddersHandler{
		staticRegistry: &staticBidderList{bidders: bidders},
	}
}

// staticBidderList implements BidderLister with a fixed list of bidders
type staticBidderList struct {
	bidders []string
}

func (s *staticBidderList) ListBidders() []string {
	return s.bidders
}

// NewDynamicInfoBiddersHandler creates a handler that queries the registry at request time
func NewDynamicInfoBiddersHandler(staticRegistry BidderLister) *InfoBiddersHandler {
	return &InfoBiddersHandler{
		staticRegistry: staticRegistry,
	}
}

// ServeHTTP handles info/bidders requests
func (h *InfoBiddersHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Collect bidders from the registry at request time
	bidderSet := make(map[string]bool)

	// Add static bidders
	if h.staticRegistry != nil {
		for _, bidder := range h.staticRegistry.ListBidders() {
			bidderSet[bidder] = true
		}
	}

	// Convert to slice
	bidders := make([]string, 0, len(bidderSet))
	for bidder := range bidderSet {
		bidders = append(bidders, bidder)
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(bidders); err != nil {
		log.Error().Err(err).Msg("failed to encode bidders response")
	}
}
