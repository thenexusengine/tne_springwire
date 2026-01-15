// Package endpoints provides HTTP endpoint handlers
package endpoints

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/thenexusengine/tne_springwire/pkg/logger"
	"github.com/thenexusengine/tne_springwire/pkg/redis"
)

// PublisherAdminHandler handles publisher CRUD operations via API
type PublisherAdminHandler struct {
	redisClient *redis.Client
}

// NewPublisherAdminHandler creates a new publisher admin handler
func NewPublisherAdminHandler(redisClient *redis.Client) *PublisherAdminHandler {
	return &PublisherAdminHandler{
		redisClient: redisClient,
	}
}

// Publisher represents a publisher configuration
type Publisher struct {
	ID             string   `json:"id"`
	AllowedDomains string   `json:"allowed_domains"` // Pipe-separated: "domain1.com|*.domain2.com"
	DomainList     []string `json:"domain_list"`     // Parsed array for display
}

// PublisherListResponse is the response for listing publishers
type PublisherListResponse struct {
	Publishers []Publisher `json:"publishers"`
	Count      int         `json:"count"`
}

// PublisherRequest is the request body for creating/updating publishers
type PublisherRequest struct {
	ID             string `json:"id"`
	AllowedDomains string `json:"allowed_domains"`
}

// ErrorResponse is a standard error response
type ErrorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message,omitempty"`
}

const publishersHashKey = "tne_catalyst:publishers"

// ServeHTTP handles publisher API requests
// Routes:
//
//	GET    /admin/publishers       - List all publishers
//	GET    /admin/publishers/:id   - Get specific publisher
//	POST   /admin/publishers       - Create publisher
//	PUT    /admin/publishers/:id   - Update publisher
//	DELETE /admin/publishers/:id   - Delete publisher
func (h *PublisherAdminHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Check if Redis is available
	if h.redisClient == nil {
		h.sendError(w, http.StatusServiceUnavailable, "Redis not available", "Publisher management requires Redis connection")
		return
	}

	// Parse path to extract publisher ID if present
	path := strings.TrimPrefix(r.URL.Path, "/admin/publishers")
	path = strings.Trim(path, "/")
	publisherID := ""
	if path != "" {
		publisherID = path
	}

	switch r.Method {
	case http.MethodGet:
		if publisherID != "" {
			h.getPublisher(w, r, publisherID)
		} else {
			h.listPublishers(w, r)
		}
	case http.MethodPost:
		h.createPublisher(w, r)
	case http.MethodPut:
		if publisherID == "" {
			h.sendError(w, http.StatusBadRequest, "missing_publisher_id", "Publisher ID required in path")
			return
		}
		h.updatePublisher(w, r, publisherID)
	case http.MethodDelete:
		if publisherID == "" {
			h.sendError(w, http.StatusBadRequest, "missing_publisher_id", "Publisher ID required in path")
			return
		}
		h.deletePublisher(w, r, publisherID)
	default:
		h.sendError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed")
	}
}

// listPublishers returns all registered publishers
func (h *PublisherAdminHandler) listPublishers(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Get all publishers from Redis hash
	publishers, err := h.redisClient.HGetAll(ctx, publishersHashKey)
	if err != nil {
		logger.Log.Error().Err(err).Msg("Failed to list publishers from Redis")
		h.sendError(w, http.StatusInternalServerError, "redis_error", "Failed to retrieve publishers")
		return
	}

	// Convert to response format
	pubList := make([]Publisher, 0, len(publishers))
	for id, domains := range publishers {
		pubList = append(pubList, Publisher{
			ID:             id,
			AllowedDomains: domains,
			DomainList:     parseDomains(domains),
		})
	}

	response := PublisherListResponse{
		Publishers: pubList,
		Count:      len(pubList),
	}

	h.sendJSON(w, http.StatusOK, response)
}

// getPublisher returns a specific publisher by ID
func (h *PublisherAdminHandler) getPublisher(w http.ResponseWriter, r *http.Request, publisherID string) {
	ctx := r.Context()

	// Get publisher from Redis
	domains, err := h.redisClient.HGet(ctx, publishersHashKey, publisherID)
	if err != nil {
		logger.Log.Error().Err(err).Str("publisher_id", publisherID).Msg("Failed to get publisher from Redis")
		h.sendError(w, http.StatusInternalServerError, "redis_error", "Failed to retrieve publisher")
		return
	}

	if domains == "" {
		h.sendError(w, http.StatusNotFound, "not_found", "Publisher not found")
		return
	}

	publisher := Publisher{
		ID:             publisherID,
		AllowedDomains: domains,
		DomainList:     parseDomains(domains),
	}

	h.sendJSON(w, http.StatusOK, publisher)
}

// createPublisher creates a new publisher
func (h *PublisherAdminHandler) createPublisher(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Parse request body
	var req PublisherRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.sendError(w, http.StatusBadRequest, "invalid_json", "Invalid request body")
		return
	}

	// Validate
	if req.ID == "" {
		h.sendError(w, http.StatusBadRequest, "missing_id", "Publisher ID is required")
		return
	}
	if req.AllowedDomains == "" {
		h.sendError(w, http.StatusBadRequest, "missing_domains", "Allowed domains are required")
		return
	}

	// Check if publisher already exists
	existing, err := h.redisClient.HGet(ctx, publishersHashKey, req.ID)
	if err != nil {
		logger.Log.Error().Err(err).Str("publisher_id", req.ID).Msg("Failed to check existing publisher")
		h.sendError(w, http.StatusInternalServerError, "redis_error", "Failed to check existing publisher")
		return
	}
	if existing != "" {
		h.sendError(w, http.StatusConflict, "already_exists", "Publisher already exists. Use PUT to update.")
		return
	}

	// Create publisher in Redis
	if err := h.redisClient.HSet(ctx, publishersHashKey, req.ID, req.AllowedDomains); err != nil {
		logger.Log.Error().Err(err).Str("publisher_id", req.ID).Msg("Failed to create publisher in Redis")
		h.sendError(w, http.StatusInternalServerError, "redis_error", "Failed to create publisher")
		return
	}

	logger.Log.Info().
		Str("publisher_id", req.ID).
		Str("domains", req.AllowedDomains).
		Msg("Publisher created")

	// Return created publisher
	publisher := Publisher{
		ID:             req.ID,
		AllowedDomains: req.AllowedDomains,
		DomainList:     parseDomains(req.AllowedDomains),
	}

	h.sendJSON(w, http.StatusCreated, publisher)
}

// updatePublisher updates an existing publisher
func (h *PublisherAdminHandler) updatePublisher(w http.ResponseWriter, r *http.Request, publisherID string) {
	ctx := r.Context()

	// Parse request body
	var req PublisherRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.sendError(w, http.StatusBadRequest, "invalid_json", "Invalid request body")
		return
	}

	// Validate
	if req.AllowedDomains == "" {
		h.sendError(w, http.StatusBadRequest, "missing_domains", "Allowed domains are required")
		return
	}

	// Check if publisher exists
	existing, err := h.redisClient.HGet(ctx, publishersHashKey, publisherID)
	if err != nil {
		logger.Log.Error().Err(err).Str("publisher_id", publisherID).Msg("Failed to check existing publisher")
		h.sendError(w, http.StatusInternalServerError, "redis_error", "Failed to check existing publisher")
		return
	}
	if existing == "" {
		h.sendError(w, http.StatusNotFound, "not_found", "Publisher not found. Use POST to create.")
		return
	}

	// Update publisher in Redis
	if err := h.redisClient.HSet(ctx, publishersHashKey, publisherID, req.AllowedDomains); err != nil {
		logger.Log.Error().Err(err).Str("publisher_id", publisherID).Msg("Failed to update publisher in Redis")
		h.sendError(w, http.StatusInternalServerError, "redis_error", "Failed to update publisher")
		return
	}

	logger.Log.Info().
		Str("publisher_id", publisherID).
		Str("old_domains", existing).
		Str("new_domains", req.AllowedDomains).
		Msg("Publisher updated")

	// Return updated publisher
	publisher := Publisher{
		ID:             publisherID,
		AllowedDomains: req.AllowedDomains,
		DomainList:     parseDomains(req.AllowedDomains),
	}

	h.sendJSON(w, http.StatusOK, publisher)
}

// deletePublisher deletes a publisher
func (h *PublisherAdminHandler) deletePublisher(w http.ResponseWriter, r *http.Request, publisherID string) {
	ctx := r.Context()

	// Check if publisher exists
	existing, err := h.redisClient.HGet(ctx, publishersHashKey, publisherID)
	if err != nil {
		logger.Log.Error().Err(err).Str("publisher_id", publisherID).Msg("Failed to check existing publisher")
		h.sendError(w, http.StatusInternalServerError, "redis_error", "Failed to check existing publisher")
		return
	}
	if existing == "" {
		h.sendError(w, http.StatusNotFound, "not_found", "Publisher not found")
		return
	}

	// Delete publisher from Redis
	if err := h.redisClient.HDel(ctx, publishersHashKey, publisherID); err != nil {
		logger.Log.Error().Err(err).Str("publisher_id", publisherID).Msg("Failed to delete publisher from Redis")
		h.sendError(w, http.StatusInternalServerError, "redis_error", "Failed to delete publisher")
		return
	}

	logger.Log.Info().
		Str("publisher_id", publisherID).
		Str("domains", existing).
		Msg("Publisher deleted")

	// Return success with deleted info
	response := map[string]interface{}{
		"success":         true,
		"publisher_id":    publisherID,
		"deleted_domains": existing,
	}

	h.sendJSON(w, http.StatusOK, response)
}

// parseDomains splits pipe-separated domains into array
func parseDomains(domains string) []string {
	if domains == "" {
		return []string{}
	}
	parts := strings.Split(domains, "|")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

// sendJSON sends a JSON response
func (h *PublisherAdminHandler) sendJSON(w http.ResponseWriter, statusCode int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		logger.Log.Error().Err(err).Msg("Failed to encode JSON response")
	}
}

// sendError sends a JSON error response
func (h *PublisherAdminHandler) sendError(w http.ResponseWriter, statusCode int, errorCode, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	response := ErrorResponse{
		Error:   errorCode,
		Message: message,
	}
	if err := json.NewEncoder(w).Encode(response); err != nil {
		logger.Log.Error().Err(err).Msg("Failed to encode error response")
	}
}
