-- =====================================================
-- Add Bid Multiplier to Publishers
-- =====================================================
-- This migration adds a bid_multiplier column to enable
-- transparent revenue sharing. Bid prices are DIVIDED
-- by the multiplier before returning them to publishers.
--
-- Example: bid_multiplier = 1.05 means the platform
-- keeps ~5% and returns ~95% of the bid to the publisher.
-- =====================================================

-- Add bid_multiplier column to publishers table
ALTER TABLE publishers
ADD COLUMN bid_multiplier DECIMAL(6,4) DEFAULT 1.0000
CHECK (bid_multiplier >= 1.0000 AND bid_multiplier <= 10.0000);

-- Add index for efficient filtering
CREATE INDEX idx_publishers_bid_multiplier ON publishers(bid_multiplier);

-- Update existing publishers to default multiplier (no adjustment)
UPDATE publishers
SET bid_multiplier = 1.0000
WHERE bid_multiplier IS NULL;

-- Add comment explaining the field
COMMENT ON COLUMN publishers.bid_multiplier IS 'Multiplier for revenue sharing (1.0000-10.0000). Bid is divided by this value. Example: 1.05 = platform keeps ~5%, publisher gets ~95%';

-- Update the active publishers view to include bid_multiplier
CREATE OR REPLACE VIEW v_active_publishers AS
SELECT
    publisher_id,
    name,
    allowed_domains,
    jsonb_object_keys(bidder_params) as bidder_count,
    bid_multiplier,
    status,
    created_at,
    updated_at
FROM publishers
WHERE status = 'active';

-- Update publisher-bidder view to include bid_multiplier
CREATE OR REPLACE VIEW v_publisher_bidders AS
SELECT
    p.publisher_id,
    p.name as publisher_name,
    p.bid_multiplier,
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
  AND p.bidder_params ? b.bidder_code
ORDER BY p.publisher_id, b.bidder_code;
