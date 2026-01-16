# Changelog

All notable changes to TNE Catalyst will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- ModSecurity WAF integration with OWASP CRS and custom OpenRTB rules
- Comprehensive environment variable documentation in README
- CHANGELOG.md for tracking project changes
- Context-based authentication for `/openrtb2/auction` endpoint
- Dual geo-location checking (device.geo AND user.geo) for privacy enforcement
- ValidationError type for proper 4xx HTTP status codes
- Deep copying for bid request objects to prevent data races
- Cookie sync filter settings implementation
- GeoIP database path configuration for IVT detection
- Currency conversion configuration option

### Changed
- **BREAKING**: Removed dynamic bidder support - now using static bidders only
- Privacy middleware now preserves OpenRTB extensions during IP anonymization
- ValidationError.Index changed from `int` to `*int` (nil = no index)
- IDR timeout default updated from 50ms to 150ms in documentation
- CI workflows updated to use actions/checkout@v4 and actions/setup-go@v5
- CI workflows now use Go tip (gotip) for all commands
- Publisher authentication now uses Redis by default

### Fixed
- **Security**: GDPR/CCPA bypass vulnerability - now checks both device.geo AND user.geo
- **Security**: Debug mode could be enabled via header injection - added length validation and context checks
- **Compliance**: Privacy extensions were being dropped during IP anonymization
- **Reliability**: Data races in concurrent bidder adapters from shallow copying
- **API**: Validation errors now return 400 instead of 500 status codes
- **Data Integrity**: Empty status field now defaults to 'active' preventing DB violations
- **API**: GetBidderParams now handles scalar/array JSON gracefully
- **Config**: Cookie sync filterSettings are now properly applied
- **Code**: Removed unused anonymizeRequestIPs function
- **Code**: Fixed variable shadowing in privacy middleware
- Deprecated NewInfoBiddersHandler now uses provided bidders parameter

### Security
- Fixed GDPR/CCPA geo-location bypass (CVE-pending)
- Fixed debug mode header injection vulnerability
- Added deep copying to prevent concurrent mutation attacks
- Enhanced publisher ID validation with minimum length requirements

### Deprecated
- Dynamic bidder loading from PostgreSQL (removed in this version)
- See [deployment/BIDDER-MANAGEMENT.md](deployment/BIDDER-MANAGEMENT.md) for migration guide

## [2.0.0] - 2026-01-16

### Major Changes
- Removed dynamic bidder support (static bidders only: rubicon, pubmatic, appnexus, demo)
- Audit fixes addressing 9 critical security and reliability issues

### Added
- Static bidder architecture for improved performance and security

### Removed
- Dynamic bidder registry and database-backed bidder loading
- Redis bidder synchronization
- Runtime bidder configuration updates

## [1.x.x] - Historical

Previous versions used dynamic bidder loading from PostgreSQL. See git history for details.

---

## Migration Guides

### Upgrading to 2.0 (Static Bidders)

**Impact**: Database bidder entries are no longer loaded at runtime.

**Migration Steps**:

1. **Identify your active bidders**
   ```bash
   # Check which bidders you're using
   SELECT bidder_code FROM bidders WHERE status = 'active';
   ```

2. **Check against supported static bidders**
   - Supported: `rubicon`, `pubmatic`, `appnexus`, `demo`
   - If you need other bidders, you must add them to the codebase

3. **Add custom bidders** (if needed)
   - Create adapter in `internal/adapters/<bidder>/`
   - Register in `internal/exchange/exchange.go`
   - See [deployment/BIDDER-MANAGEMENT.md#adding-static-bidders-current-method](deployment/BIDDER-MANAGEMENT.md#adding-static-bidders-current-method)

4. **Update environment variables**
   ```bash
   # New privacy settings
   PBS_GEO_ENFORCEMENT=true
   PBS_ANONYMIZE_IP=true
   PBS_PRIVACY_STRICT_MODE=true

   # Database configuration (now required)
   DB_HOST=localhost
   DB_PORT=5432
   DB_USER=catalyst
   DB_PASSWORD=your_password
   DB_NAME=catalyst

   # Redis configuration (expanded)
   REDIS_HOST=localhost
   REDIS_PORT=6379
   REDIS_POOL_SIZE=50
   ```

5. **Update publisher bidder_params**
   - Publisher bidder_params still work the same way
   - Just ensure bidder codes match static bidders

### Security Improvements to Note

**Geo-Location Enforcement**:
- Now checks BOTH `device.geo` and `user.geo`
- Prevents bypass by omitting device location
- No configuration needed - automatic

**IP Anonymization**:
- Now preserves all OpenRTB extensions
- Set `PBS_ANONYMIZE_IP=true` for GDPR compliance

**Authentication**:
- Debug mode now requires valid publisher ID (8+ characters)
- Context-based auth prevents header spoofing

### Breaking Changes

1. **Dynamic Bidders**: Must migrate to static bidders
2. **ValidationError.Index**: Now nullable pointer (`*int`)
3. **Error Codes**: Validation errors return 400 (was 500)

---

## Support

For questions about upgrading or changes:
- Open an issue: https://github.com/thenexusengine/tne_springwire/issues
- See documentation: [README.md](README.md)
- Review audit fixes: `/tmp/audit-resolution-report.md` (if available)
