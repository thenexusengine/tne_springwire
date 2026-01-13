-- =====================================================
-- Catalyst Bidders Schema
-- =====================================================
-- This migration creates the bidders table for storing
-- global bidder configurations including endpoint URLs,
-- capabilities, and metadata.
--
-- Bidders are identified by bidder_code and contain
-- the endpoint URL and configuration that applies to
-- all publishers using that bidder.
-- =====================================================

-- Create bidders table
CREATE TABLE IF NOT EXISTS bidders (
    -- Primary identification
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    bidder_code VARCHAR(50) UNIQUE NOT NULL,     -- e.g., 'rubicon', 'pubmatic'
    bidder_name VARCHAR(255) NOT NULL,            -- e.g., 'Rubicon/Magnite', 'PubMatic'

    -- Endpoint configuration
    endpoint_url TEXT NOT NULL,                   -- OpenRTB endpoint URL
    timeout_ms INTEGER DEFAULT 1000,              -- Request timeout in milliseconds

    -- Status
    enabled BOOLEAN DEFAULT true,                 -- Global enable/disable
    status VARCHAR(50) DEFAULT 'active',          -- 'active', 'testing', 'disabled'

    -- Capabilities (what ad formats this bidder supports)
    supports_banner BOOLEAN DEFAULT true,
    supports_video BOOLEAN DEFAULT false,
    supports_native BOOLEAN DEFAULT false,
    supports_audio BOOLEAN DEFAULT false,

    -- Privacy & Compliance
    gvl_vendor_id INTEGER,                        -- IAB GVL (Global Vendor List) ID for GDPR

    -- HTTP Configuration
    http_headers JSONB DEFAULT '{}'::jsonb,       -- Custom headers to send with requests

    -- Metadata
    description TEXT,                             -- Internal notes about this bidder
    documentation_url TEXT,                       -- Link to bidder's integration docs
    contact_email VARCHAR(255),                   -- Bidder support contact

    -- Timestamps
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,

    -- Constraints
    CONSTRAINT valid_status CHECK (status IN ('active', 'testing', 'disabled', 'archived')),
    CONSTRAINT valid_timeout CHECK (timeout_ms >= 100 AND timeout_ms <= 10000),
    CONSTRAINT valid_gvl_id CHECK (gvl_vendor_id IS NULL OR gvl_vendor_id > 0)
);

-- Create indexes
CREATE INDEX idx_bidders_code ON bidders(bidder_code);
CREATE INDEX idx_bidders_enabled ON bidders(enabled);
CREATE INDEX idx_bidders_status ON bidders(status);
CREATE INDEX idx_bidders_gvl_id ON bidders(gvl_vendor_id);

-- Create trigger for updated_at
CREATE TRIGGER trigger_bidders_updated_at
    BEFORE UPDATE ON bidders
    FOR EACH ROW
    EXECUTE FUNCTION update_publishers_updated_at();

-- Insert common bidders with their configurations
INSERT INTO bidders (
    bidder_code,
    bidder_name,
    endpoint_url,
    timeout_ms,
    gvl_vendor_id,
    supports_banner,
    supports_video,
    supports_native,
    status,
    description
) VALUES
    -- Rubicon/Magnite
    (
        'rubicon',
        'Rubicon/Magnite',
        'https://prebid-server.rubiconproject.com/openrtb2/auction',
        1500,
        52,
        true,
        true,
        true,
        'active',
        'Rubicon Project (now Magnite) - requires accountId, siteId, zoneId'
    ),

    -- PubMatic
    (
        'pubmatic',
        'PubMatic',
        'https://hbopenbid.pubmatic.com/translator?source=prebid-server',
        1500,
        76,
        true,
        true,
        false,
        'active',
        'PubMatic SSP - requires publisherId and adSlot'
    ),

    -- AppNexus/Xandr
    (
        'appnexus',
        'AppNexus/Xandr',
        'https://ib.adnxs.com/openrtb2',
        1500,
        32,
        true,
        true,
        true,
        'active',
        'AppNexus (now Xandr) - requires placementId'
    ),

    -- Index Exchange
    (
        'ix',
        'Index Exchange',
        'https://htlb.casalemedia.com/openrtb/pbjs',
        1500,
        10,
        true,
        true,
        false,
        'active',
        'Index Exchange - requires siteId'
    ),

    -- OpenX
    (
        'openx',
        'OpenX',
        'https://rtb.openx.net/prebid',
        1500,
        69,
        true,
        true,
        false,
        'active',
        'OpenX - requires unit and delDomain'
    ),

    -- Criteo
    (
        'criteo',
        'Criteo',
        'https://bidder.criteo.com/openrtb/pbjs',
        1500,
        91,
        true,
        false,
        false,
        'active',
        'Criteo - requires networkId or zoneId'
    ),

    -- TripleLift
    (
        'triplelift',
        'TripleLift',
        'https://tlx.3lift.com/s2s/auction',
        1500,
        28,
        true,
        false,
        true,
        'active',
        'TripleLift - requires inventoryCode'
    ),

    -- Sovrn
    (
        'sovrn',
        'Sovrn',
        'https://ap.lijit.com/rtb/bid?src=prebid_server',
        1500,
        13,
        true,
        false,
        false,
        'active',
        'Sovrn - requires tagid'
    ),

    -- Demo Bidder (for testing)
    (
        'demo',
        'Demo Bidder',
        'https://demo.bidder.com/openrtb2/auction',
        500,
        NULL,
        true,
        true,
        true,
        'testing',
        'Internal demo bidder for testing - always returns mock bids'
    )
ON CONFLICT (bidder_code) DO NOTHING;

-- Create view for active bidders with capabilities
CREATE OR REPLACE VIEW v_active_bidders AS
SELECT
    bidder_code,
    bidder_name,
    endpoint_url,
    timeout_ms,
    gvl_vendor_id,
    supports_banner,
    supports_video,
    supports_native,
    status
FROM bidders
WHERE enabled = true AND status = 'active';

-- Create view for bidder-publisher pairings
CREATE OR REPLACE VIEW v_publisher_bidders AS
SELECT
    p.publisher_id,
    p.name as publisher_name,
    b.bidder_code,
    b.bidder_name,
    b.endpoint_url,
    b.timeout_ms,
    p.bidder_params->b.bidder_code as bidder_config
FROM publishers p
CROSS JOIN bidders b
WHERE p.status = 'active'
  AND b.enabled = true
  AND b.status = 'active'
  AND p.bidder_params ? b.bidder_code  -- Only where publisher has config for this bidder
ORDER BY p.publisher_id, b.bidder_code;

COMMENT ON TABLE bidders IS 'Global bidder configurations including endpoints and capabilities';
COMMENT ON COLUMN bidders.bidder_code IS 'Unique bidder identifier used in OpenRTB ext objects';
COMMENT ON COLUMN bidders.endpoint_url IS 'OpenRTB 2.x endpoint URL for bid requests';
COMMENT ON COLUMN bidders.gvl_vendor_id IS 'IAB Global Vendor List ID for GDPR TCF consent validation';
COMMENT ON COLUMN bidders.http_headers IS 'Custom HTTP headers to include in bid requests';
