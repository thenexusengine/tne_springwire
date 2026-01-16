// Package storage provides database access for Catalyst
package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

// Bidder represents a bidder configuration from the database
type Bidder struct {
	ID               string                 `json:"id"`
	BidderCode       string                 `json:"bidder_code"`
	BidderName       string                 `json:"bidder_name"`
	EndpointURL      string                 `json:"endpoint_url"`
	TimeoutMs        int                    `json:"timeout_ms"`
	Enabled          bool                   `json:"enabled"`
	Status           string                 `json:"status"`
	SupportsBanner   bool                   `json:"supports_banner"`
	SupportsVideo    bool                   `json:"supports_video"`
	SupportsNative   bool                   `json:"supports_native"`
	SupportsAudio    bool                   `json:"supports_audio"`
	GVLVendorID      *int                   `json:"gvl_vendor_id,omitempty"`
	HTTPHeaders      map[string]interface{} `json:"http_headers"`
	Description      string                 `json:"description,omitempty"`
	DocumentationURL string                 `json:"documentation_url,omitempty"`
	ContactEmail     string                 `json:"contact_email,omitempty"`
	CreatedAt        time.Time              `json:"created_at"`
	UpdatedAt        time.Time              `json:"updated_at"`
}

// PublisherBidder represents a bidder with publisher-specific configuration
type PublisherBidder struct {
	Bidder
	PublisherID   string                 `json:"publisher_id"`
	PublisherName string                 `json:"publisher_name"`
	BidderConfig  map[string]interface{} `json:"bidder_config"`
}

// BidderStore provides database operations for bidders
type BidderStore struct {
	db *sql.DB
}

// NewBidderStore creates a new bidder store
func NewBidderStore(db *sql.DB) *BidderStore {
	return &BidderStore{db: db}
}

// GetByCode retrieves a bidder by their bidder_code
func (s *BidderStore) GetByCode(ctx context.Context, bidderCode string) (*Bidder, error) {
	query := `
		SELECT id, bidder_code, bidder_name, endpoint_url, timeout_ms,
		       enabled, status, supports_banner, supports_video, supports_native, supports_audio,
		       gvl_vendor_id, http_headers, description, documentation_url, contact_email,
		       created_at, updated_at
		FROM bidders
		WHERE bidder_code = $1 AND enabled = true AND status = 'active'
	`

	var b Bidder
	var httpHeadersJSON []byte

	err := s.db.QueryRowContext(ctx, query, bidderCode).Scan(
		&b.ID,
		&b.BidderCode,
		&b.BidderName,
		&b.EndpointURL,
		&b.TimeoutMs,
		&b.Enabled,
		&b.Status,
		&b.SupportsBanner,
		&b.SupportsVideo,
		&b.SupportsNative,
		&b.SupportsAudio,
		&b.GVLVendorID,
		&httpHeadersJSON,
		&b.Description,
		&b.DocumentationURL,
		&b.ContactEmail,
		&b.CreatedAt,
		&b.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, nil // Bidder not found
	}
	if err != nil {
		return nil, fmt.Errorf("failed to query bidder: %w", err)
	}

	// Parse JSONB http_headers
	if len(httpHeadersJSON) > 0 {
		if err := json.Unmarshal(httpHeadersJSON, &b.HTTPHeaders); err != nil {
			return nil, fmt.Errorf("failed to parse http_headers: %w", err)
		}
	}

	return &b, nil
}

// ListActive retrieves all active bidders
func (s *BidderStore) ListActive(ctx context.Context) ([]*Bidder, error) {
	query := `
		SELECT id, bidder_code, bidder_name, endpoint_url, timeout_ms,
		       enabled, status, supports_banner, supports_video, supports_native, supports_audio,
		       gvl_vendor_id, http_headers, description, documentation_url, contact_email,
		       created_at, updated_at
		FROM bidders
		WHERE enabled = true AND status = 'active'
		ORDER BY bidder_code
	`

	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query bidders: %w", err)
	}
	defer rows.Close()

	bidders := make([]*Bidder, 0, 100)
	for rows.Next() {
		var b Bidder
		var httpHeadersJSON []byte

		err := rows.Scan(
			&b.ID,
			&b.BidderCode,
			&b.BidderName,
			&b.EndpointURL,
			&b.TimeoutMs,
			&b.Enabled,
			&b.Status,
			&b.SupportsBanner,
			&b.SupportsVideo,
			&b.SupportsNative,
			&b.SupportsAudio,
			&b.GVLVendorID,
			&httpHeadersJSON,
			&b.Description,
			&b.DocumentationURL,
			&b.ContactEmail,
			&b.CreatedAt,
			&b.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan bidder row: %w", err)
		}

		// Parse JSONB http_headers
		if len(httpHeadersJSON) > 0 {
			if err := json.Unmarshal(httpHeadersJSON, &b.HTTPHeaders); err != nil {
				return nil, fmt.Errorf("failed to parse http_headers: %w", err)
			}
		}

		bidders = append(bidders, &b)
	}

	return bidders, rows.Err()
}

// GetForPublisher retrieves all bidders configured for a specific publisher
// This joins bidders with the publisher's bidder_params to get complete configurations
func (s *BidderStore) GetForPublisher(ctx context.Context, publisherID string) ([]*PublisherBidder, error) {
	query := `
		SELECT
			b.id,
			b.bidder_code,
			b.bidder_name,
			b.endpoint_url,
			b.timeout_ms,
			b.enabled,
			b.status,
			b.supports_banner,
			b.supports_video,
			b.supports_native,
			b.supports_audio,
			b.gvl_vendor_id,
			b.http_headers,
			b.description,
			b.documentation_url,
			b.contact_email,
			b.created_at,
			b.updated_at,
			p.publisher_id,
			p.name as publisher_name,
			p.bidder_params->b.bidder_code as bidder_config
		FROM bidders b
		CROSS JOIN publishers p
		WHERE p.publisher_id = $1
		  AND p.status = 'active'
		  AND b.enabled = true
		  AND b.status = 'active'
		  AND p.bidder_params ? b.bidder_code
		ORDER BY b.bidder_code
	`

	rows, err := s.db.QueryContext(ctx, query, publisherID)
	if err != nil {
		return nil, fmt.Errorf("failed to query publisher bidders: %w", err)
	}
	defer rows.Close()

	bidders := make([]*PublisherBidder, 0, 100)
	for rows.Next() {
		var pb PublisherBidder
		var httpHeadersJSON []byte
		var bidderConfigJSON []byte

		err := rows.Scan(
			&pb.ID,
			&pb.BidderCode,
			&pb.BidderName,
			&pb.EndpointURL,
			&pb.TimeoutMs,
			&pb.Enabled,
			&pb.Status,
			&pb.SupportsBanner,
			&pb.SupportsVideo,
			&pb.SupportsNative,
			&pb.SupportsAudio,
			&pb.GVLVendorID,
			&httpHeadersJSON,
			&pb.Description,
			&pb.DocumentationURL,
			&pb.ContactEmail,
			&pb.CreatedAt,
			&pb.UpdatedAt,
			&pb.PublisherID,
			&pb.PublisherName,
			&bidderConfigJSON,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan publisher bidder row: %w", err)
		}

		// Parse JSONB http_headers
		if len(httpHeadersJSON) > 0 {
			if err := json.Unmarshal(httpHeadersJSON, &pb.HTTPHeaders); err != nil {
				return nil, fmt.Errorf("failed to parse http_headers: %w", err)
			}
		}

		// Parse JSONB bidder_config
		if len(bidderConfigJSON) > 0 {
			if err := json.Unmarshal(bidderConfigJSON, &pb.BidderConfig); err != nil {
				return nil, fmt.Errorf("failed to parse bidder_config: %w", err)
			}
		}

		bidders = append(bidders, &pb)
	}

	return bidders, rows.Err()
}

// List retrieves all bidders (active and inactive)
func (s *BidderStore) List(ctx context.Context) ([]*Bidder, error) {
	query := `
		SELECT id, bidder_code, bidder_name, endpoint_url, timeout_ms,
		       enabled, status, supports_banner, supports_video, supports_native, supports_audio,
		       gvl_vendor_id, http_headers, description, documentation_url, contact_email,
		       created_at, updated_at
		FROM bidders
		ORDER BY bidder_code
	`

	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query bidders: %w", err)
	}
	defer rows.Close()

	bidders := make([]*Bidder, 0, 10)
	for rows.Next() {
		var b Bidder
		var httpHeadersJSON []byte

		err := rows.Scan(
			&b.ID,
			&b.BidderCode,
			&b.BidderName,
			&b.EndpointURL,
			&b.TimeoutMs,
			&b.Enabled,
			&b.Status,
			&b.SupportsBanner,
			&b.SupportsVideo,
			&b.SupportsNative,
			&b.SupportsAudio,
			&b.GVLVendorID,
			&httpHeadersJSON,
			&b.Description,
			&b.DocumentationURL,
			&b.ContactEmail,
			&b.CreatedAt,
			&b.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan bidder row: %w", err)
		}

		// Parse JSONB http_headers
		if len(httpHeadersJSON) > 0 {
			if err := json.Unmarshal(httpHeadersJSON, &b.HTTPHeaders); err != nil {
				return nil, fmt.Errorf("failed to parse http_headers: %w", err)
			}
		}

		bidders = append(bidders, &b)
	}

	return bidders, rows.Err()
}

// Create adds a new bidder
func (s *BidderStore) Create(ctx context.Context, b *Bidder) error {
	query := `
		INSERT INTO bidders (
			bidder_code, bidder_name, endpoint_url, timeout_ms,
			enabled, status, supports_banner, supports_video, supports_native, supports_audio,
			gvl_vendor_id, http_headers, description, documentation_url, contact_email
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
		RETURNING id, created_at, updated_at
	`

	httpHeadersJSON, err := json.Marshal(b.HTTPHeaders)
	if err != nil {
		return fmt.Errorf("failed to marshal http_headers: %w", err)
	}

	// Default status to 'active' if not set to prevent DB constraint violation
	status := b.Status
	if status == "" {
		status = "active"
	}

	err = s.db.QueryRowContext(ctx, query,
		b.BidderCode,
		b.BidderName,
		b.EndpointURL,
		b.TimeoutMs,
		b.Enabled,
		status,
		b.SupportsBanner,
		b.SupportsVideo,
		b.SupportsNative,
		b.SupportsAudio,
		b.GVLVendorID,
		httpHeadersJSON,
		b.Description,
		b.DocumentationURL,
		b.ContactEmail,
	).Scan(&b.ID, &b.CreatedAt, &b.UpdatedAt)

	if err != nil {
		return fmt.Errorf("failed to create bidder: %w", err)
	}

	return nil
}

// Update modifies an existing bidder
func (s *BidderStore) Update(ctx context.Context, b *Bidder) error {
	query := `
		UPDATE bidders
		SET bidder_name = $1, endpoint_url = $2, timeout_ms = $3,
		    enabled = $4, status = $5, supports_banner = $6, supports_video = $7,
		    supports_native = $8, supports_audio = $9, gvl_vendor_id = $10,
		    http_headers = $11, description = $12, documentation_url = $13, contact_email = $14
		WHERE bidder_code = $15
	`

	httpHeadersJSON, err := json.Marshal(b.HTTPHeaders)
	if err != nil {
		return fmt.Errorf("failed to marshal http_headers: %w", err)
	}

	result, err := s.db.ExecContext(ctx, query,
		b.BidderName,
		b.EndpointURL,
		b.TimeoutMs,
		b.Enabled,
		b.Status,
		b.SupportsBanner,
		b.SupportsVideo,
		b.SupportsNative,
		b.SupportsAudio,
		b.GVLVendorID,
		httpHeadersJSON,
		b.Description,
		b.DocumentationURL,
		b.ContactEmail,
		b.BidderCode,
	)

	if err != nil {
		return fmt.Errorf("failed to update bidder: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rows == 0 {
		return fmt.Errorf("bidder not found: %s", b.BidderCode)
	}

	return nil
}

// Delete soft-deletes a bidder by setting status to 'archived'
func (s *BidderStore) Delete(ctx context.Context, bidderCode string) error {
	query := `
		UPDATE bidders
		SET status = 'archived', enabled = false
		WHERE bidder_code = $1
	`

	result, err := s.db.ExecContext(ctx, query, bidderCode)
	if err != nil {
		return fmt.Errorf("failed to delete bidder: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rows == 0 {
		return fmt.Errorf("bidder not found: %s", bidderCode)
	}

	return nil
}

// SetEnabled enables or disables a bidder
func (s *BidderStore) SetEnabled(ctx context.Context, bidderCode string, enabled bool) error {
	query := `
		UPDATE bidders
		SET enabled = $1
		WHERE bidder_code = $2
	`

	result, err := s.db.ExecContext(ctx, query, enabled, bidderCode)
	if err != nil {
		return fmt.Errorf("failed to set bidder enabled: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rows == 0 {
		return fmt.Errorf("bidder not found: %s", bidderCode)
	}

	return nil
}

// GetCapabilities returns bidders filtered by format capability
func (s *BidderStore) GetCapabilities(ctx context.Context, banner, video, native, audio bool) ([]*Bidder, error) {
	query := `
		SELECT id, bidder_code, bidder_name, endpoint_url, timeout_ms,
		       enabled, status, supports_banner, supports_video, supports_native, supports_audio,
		       gvl_vendor_id, http_headers, description, documentation_url, contact_email,
		       created_at, updated_at
		FROM bidders
		WHERE enabled = true
		  AND status = 'active'
		  AND ($1 = false OR supports_banner = true)
		  AND ($2 = false OR supports_video = true)
		  AND ($3 = false OR supports_native = true)
		  AND ($4 = false OR supports_audio = true)
		ORDER BY bidder_code
	`

	rows, err := s.db.QueryContext(ctx, query, banner, video, native, audio)
	if err != nil {
		return nil, fmt.Errorf("failed to query bidders by capabilities: %w", err)
	}
	defer rows.Close()

	bidders := make([]*Bidder, 0, 100)
	for rows.Next() {
		var b Bidder
		var httpHeadersJSON []byte

		err := rows.Scan(
			&b.ID,
			&b.BidderCode,
			&b.BidderName,
			&b.EndpointURL,
			&b.TimeoutMs,
			&b.Enabled,
			&b.Status,
			&b.SupportsBanner,
			&b.SupportsVideo,
			&b.SupportsNative,
			&b.SupportsAudio,
			&b.GVLVendorID,
			&httpHeadersJSON,
			&b.Description,
			&b.DocumentationURL,
			&b.ContactEmail,
			&b.CreatedAt,
			&b.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan bidder row: %w", err)
		}

		// Parse JSONB http_headers
		if len(httpHeadersJSON) > 0 {
			if err := json.Unmarshal(httpHeadersJSON, &b.HTTPHeaders); err != nil {
				return nil, fmt.Errorf("failed to parse http_headers: %w", err)
			}
		}

		bidders = append(bidders, &b)
	}

	return bidders, rows.Err()
}
