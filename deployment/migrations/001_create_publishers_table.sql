-- =====================================================
-- Catalyst Publishers Schema
-- =====================================================
-- This migration creates the publishers table for storing
-- publisher configuration and bidder parameters.
--
-- Publishers are identified by a unique publisher_id and
-- can have multiple bidder configurations stored in JSONB.
-- =====================================================

-- Enable UUID extension for generating publisher UUIDs
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

-- Create publishers table
CREATE TABLE IF NOT EXISTS publishers (
    -- Primary identification
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    publisher_id VARCHAR(255) UNIQUE NOT NULL,  -- e.g., 'totalsportspro'
    name VARCHAR(255) NOT NULL,                  -- e.g., 'Total Sports Pro'

    -- Domain configuration (allowed domains for this publisher)
    -- Supports multiple domains separated by pipe: 'example.com|*.example.com'
    -- Use '*' to allow any domain (testing only)
    allowed_domains TEXT NOT NULL,

    -- Bidder configurations stored as JSONB
    -- Format: {
    --   "rubicon": {"accountId": 26298, "siteId": 556630, "zoneId": 3767186},
    --   "pubmatic": {"publisherId": "12345", "adSlot": "slot1"},
    --   "appnexus": {"placementId": 54321}
    -- }
    bidder_params JSONB DEFAULT '{}'::jsonb,

    -- Status and metadata
    status VARCHAR(50) DEFAULT 'active',         -- 'active', 'paused', 'archived'
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,

    -- Additional metadata
    notes TEXT,                                   -- Internal notes
    contact_email VARCHAR(255),                   -- Publisher contact

    -- Indexes for fast lookups
    CONSTRAINT valid_status CHECK (status IN ('active', 'paused', 'archived'))
);

-- Create indexes for performance
CREATE INDEX idx_publishers_publisher_id ON publishers(publisher_id);
CREATE INDEX idx_publishers_status ON publishers(status);
CREATE INDEX idx_publishers_created_at ON publishers(created_at);

-- Create GIN index for JSONB bidder_params (enables fast bidder lookups)
CREATE INDEX idx_publishers_bidder_params ON publishers USING GIN (bidder_params);

-- Create function to automatically update updated_at timestamp
CREATE OR REPLACE FUNCTION update_publishers_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = CURRENT_TIMESTAMP;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- Create trigger to call the update function
CREATE TRIGGER trigger_publishers_updated_at
    BEFORE UPDATE ON publishers
    FOR EACH ROW
    EXECUTE FUNCTION update_publishers_updated_at();

-- Insert Total Sports Pro example publisher
INSERT INTO publishers (
    publisher_id,
    name,
    allowed_domains,
    bidder_params,
    status,
    notes
) VALUES (
    'totalsportspro',
    'Total Sports Pro',
    'totalsportspro.com',
    '{
        "rubicon": {
            "accountId": 26298,
            "siteId": 556630,
            "zoneId": 3767186
        }
    }'::jsonb,
    'active',
    'Initial publisher configuration with Rubicon/Magnite credentials'
) ON CONFLICT (publisher_id) DO NOTHING;

-- Create view for active publishers with bidder counts
CREATE OR REPLACE VIEW v_active_publishers AS
SELECT
    publisher_id,
    name,
    allowed_domains,
    jsonb_object_keys(bidder_params) as bidder_count,
    status,
    created_at,
    updated_at
FROM publishers
WHERE status = 'active';

COMMENT ON TABLE publishers IS 'Publisher configuration including bidder parameters and domain allowlists';
COMMENT ON COLUMN publishers.publisher_id IS 'Unique publisher identifier used in OpenRTB requests';
COMMENT ON COLUMN publishers.allowed_domains IS 'Pipe-separated list of allowed domains: domain.com|*.subdomain.com|*';
COMMENT ON COLUMN publishers.bidder_params IS 'JSONB object containing bidder-specific configuration parameters';
