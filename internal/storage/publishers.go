// Package storage provides database access for Catalyst
package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	_ "github.com/lib/pq" // PostgreSQL driver
)

// Publisher represents a publisher configuration from the database
type Publisher struct {
	ID             string                 `json:"id"`
	PublisherID    string                 `json:"publisher_id"`
	Name           string                 `json:"name"`
	AllowedDomains string                 `json:"allowed_domains"`
	BidderParams   map[string]interface{} `json:"bidder_params"`
	Status         string                 `json:"status"`
	CreatedAt      time.Time              `json:"created_at"`
	UpdatedAt      time.Time              `json:"updated_at"`
	Notes          string                 `json:"notes,omitempty"`
	ContactEmail   string                 `json:"contact_email,omitempty"`
}

// GetAllowedDomains returns the allowed domains string (for middleware interface)
func (p *Publisher) GetAllowedDomains() string {
	return p.AllowedDomains
}

// PublisherStore provides database operations for publishers
type PublisherStore struct {
	db *sql.DB
}

// NewPublisherStore creates a new publisher store
func NewPublisherStore(db *sql.DB) *PublisherStore {
	return &PublisherStore{db: db}
}

// GetByPublisherID retrieves a publisher by their publisher_id
func (s *PublisherStore) GetByPublisherID(ctx context.Context, publisherID string) (*Publisher, error) {
	query := `
		SELECT id, publisher_id, name, allowed_domains, bidder_params, status,
		       created_at, updated_at, notes, contact_email
		FROM publishers
		WHERE publisher_id = $1 AND status = 'active'
	`

	var p Publisher
	var bidderParamsJSON []byte

	err := s.db.QueryRowContext(ctx, query, publisherID).Scan(
		&p.ID,
		&p.PublisherID,
		&p.Name,
		&p.AllowedDomains,
		&bidderParamsJSON,
		&p.Status,
		&p.CreatedAt,
		&p.UpdatedAt,
		&p.Notes,
		&p.ContactEmail,
	)

	if err == sql.ErrNoRows {
		return nil, nil // Publisher not found
	}
	if err != nil {
		return nil, fmt.Errorf("failed to query publisher: %w", err)
	}

	// Parse JSONB bidder_params
	if len(bidderParamsJSON) > 0 {
		if err := json.Unmarshal(bidderParamsJSON, &p.BidderParams); err != nil {
			return nil, fmt.Errorf("failed to parse bidder_params: %w", err)
		}
	}

	return &p, nil
}

// List retrieves all active publishers
func (s *PublisherStore) List(ctx context.Context) ([]*Publisher, error) {
	query := `
		SELECT id, publisher_id, name, allowed_domains, bidder_params, status,
		       created_at, updated_at, notes, contact_email
		FROM publishers
		WHERE status = 'active'
		ORDER BY publisher_id
	`

	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query publishers: %w", err)
	}
	defer rows.Close()

	var publishers []*Publisher
	for rows.Next() {
		var p Publisher
		var bidderParamsJSON []byte

		err := rows.Scan(
			&p.ID,
			&p.PublisherID,
			&p.Name,
			&p.AllowedDomains,
			&bidderParamsJSON,
			&p.Status,
			&p.CreatedAt,
			&p.UpdatedAt,
			&p.Notes,
			&p.ContactEmail,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan publisher row: %w", err)
		}

		// Parse JSONB bidder_params
		if len(bidderParamsJSON) > 0 {
			if err := json.Unmarshal(bidderParamsJSON, &p.BidderParams); err != nil {
				return nil, fmt.Errorf("failed to parse bidder_params: %w", err)
			}
		}

		publishers = append(publishers, &p)
	}

	return publishers, rows.Err()
}

// Create adds a new publisher
func (s *PublisherStore) Create(ctx context.Context, p *Publisher) error {
	query := `
		INSERT INTO publishers (
			publisher_id, name, allowed_domains, bidder_params, status, notes, contact_email
		) VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id, created_at, updated_at
	`

	bidderParamsJSON, err := json.Marshal(p.BidderParams)
	if err != nil {
		return fmt.Errorf("failed to marshal bidder_params: %w", err)
	}

	err = s.db.QueryRowContext(ctx, query,
		p.PublisherID,
		p.Name,
		p.AllowedDomains,
		bidderParamsJSON,
		p.Status,
		p.Notes,
		p.ContactEmail,
	).Scan(&p.ID, &p.CreatedAt, &p.UpdatedAt)

	if err != nil {
		return fmt.Errorf("failed to create publisher: %w", err)
	}

	return nil
}

// Update modifies an existing publisher
func (s *PublisherStore) Update(ctx context.Context, p *Publisher) error {
	query := `
		UPDATE publishers
		SET name = $1, allowed_domains = $2, bidder_params = $3,
		    status = $4, notes = $5, contact_email = $6
		WHERE publisher_id = $7
	`

	bidderParamsJSON, err := json.Marshal(p.BidderParams)
	if err != nil {
		return fmt.Errorf("failed to marshal bidder_params: %w", err)
	}

	result, err := s.db.ExecContext(ctx, query,
		p.Name,
		p.AllowedDomains,
		bidderParamsJSON,
		p.Status,
		p.Notes,
		p.ContactEmail,
		p.PublisherID,
	)

	if err != nil {
		return fmt.Errorf("failed to update publisher: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rows == 0 {
		return fmt.Errorf("publisher not found: %s", p.PublisherID)
	}

	return nil
}

// Delete soft-deletes a publisher by setting status to 'archived'
func (s *PublisherStore) Delete(ctx context.Context, publisherID string) error {
	query := `
		UPDATE publishers
		SET status = 'archived'
		WHERE publisher_id = $1
	`

	result, err := s.db.ExecContext(ctx, query, publisherID)
	if err != nil {
		return fmt.Errorf("failed to delete publisher: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rows == 0 {
		return fmt.Errorf("publisher not found: %s", publisherID)
	}

	return nil
}

// GetBidderParams retrieves bidder parameters for a specific bidder
func (s *PublisherStore) GetBidderParams(ctx context.Context, publisherID, bidderCode string) (map[string]interface{}, error) {
	query := `
		SELECT bidder_params->$2 as params
		FROM publishers
		WHERE publisher_id = $1 AND status = 'active'
	`

	var paramsJSON []byte
	err := s.db.QueryRowContext(ctx, query, publisherID, bidderCode).Scan(&paramsJSON)

	if err == sql.ErrNoRows {
		return nil, nil // No params for this bidder
	}
	if err != nil {
		return nil, fmt.Errorf("failed to query bidder params: %w", err)
	}

	var params map[string]interface{}
	if len(paramsJSON) > 0 {
		if err := json.Unmarshal(paramsJSON, &params); err != nil {
			return nil, fmt.Errorf("failed to parse bidder params: %w", err)
		}
	}

	return params, nil
}

// NewDBConnection creates a new database connection
func NewDBConnection(host, port, user, password, dbname, sslmode string) (*sql.DB, error) {
	connStr := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=%s",
		host, port, user, password, dbname, sslmode)

	db, err := sql.Open("postgres", connStr)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Configure connection pool
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)

	// Test connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	return db, nil
}
