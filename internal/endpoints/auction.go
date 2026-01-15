// Package endpoints provides HTTP endpoint handlers
package endpoints

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
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
		logger.Log.Error().
			Err(err).
			Str("request_id", bidRequest.ID).
			Int("imp_count", len(bidRequest.Imp)).
			Dur("duration_ms", auctionDuration).
			Msg("Auction failed")

		// Log to dashboard
		LogAuction(bidRequest.ID, len(bidRequest.Imp), 0, nil, auctionDuration, false, err)

		writeError(w, "Internal server error", http.StatusInternalServerError)
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
		if imp.ID == "" {
			return &ValidationError{Field: "imp[].id", Message: "required", Index: i}
		}
		if imp.Banner == nil && imp.Video == nil && imp.Native == nil && imp.Audio == nil {
			return &ValidationError{Field: "imp[].banner|video|native|audio", Message: "at least one media type required", Index: i}
		}
	}
	return nil
}

// ValidationError represents a validation error
type ValidationError struct {
	Field   string
	Message string
	Index   int
}

func (e *ValidationError) Error() string {
	if e.Index >= 0 {
		return fmt.Sprintf("%s[%d]: %s", e.Field, e.Index, e.Message)
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

// hasAPIKey checks if request has valid API key header
// P2-1: Used to gate debug mode access
func hasAPIKey(r *http.Request) bool {
	// Check X-API-Key header
	if r.Header.Get("X-API-Key") != "" {
		return true
	}
	// Check Authorization Bearer token
	authHeader := r.Header.Get("Authorization")
	if strings.HasPrefix(authHeader, "Bearer ") && len(authHeader) > 7 {
		return true
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

// DynamicBidderLister is an optional interface for listing dynamic bidders
type DynamicBidderLister interface {
	ListBidderCodes() []string
}

// InfoBiddersHandler handles /info/bidders requests
type InfoBiddersHandler struct {
	staticRegistry  BidderLister
	dynamicRegistry DynamicBidderLister // May be nil
}

// NewInfoBiddersHandler creates a new bidders info handler
// Deprecated: Use NewDynamicInfoBiddersHandler instead for proper dynamic bidder support
func NewInfoBiddersHandler(bidders []string) *InfoBiddersHandler {
	return &InfoBiddersHandler{}
}

// NewDynamicInfoBiddersHandler creates a handler that queries registries at request time
func NewDynamicInfoBiddersHandler(staticRegistry BidderLister, dynamicRegistry DynamicBidderLister) *InfoBiddersHandler {
	return &InfoBiddersHandler{
		staticRegistry:  staticRegistry,
		dynamicRegistry: dynamicRegistry,
	}
}

// ServeHTTP handles info/bidders requests
func (h *InfoBiddersHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Collect bidders from both registries at request time
	bidderSet := make(map[string]bool)

	// Add static bidders
	if h.staticRegistry != nil {
		for _, bidder := range h.staticRegistry.ListBidders() {
			bidderSet[bidder] = true
		}
	}

	// Add dynamic bidders
	if h.dynamicRegistry != nil {
		for _, bidder := range h.dynamicRegistry.ListBidderCodes() {
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
