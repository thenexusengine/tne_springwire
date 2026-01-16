# ModSecurity WAF for Catalyst

Web Application Firewall configuration using ModSecurity 3 with OWASP Core Rule Set (CRS) and custom ad-tech rules.

## Quick Start

### Option 1: Docker Compose (Recommended)

```bash
# Build and run with WAF
cd deployment
docker compose -f docker-compose.yml -f waf/docker-compose.waf.yml up -d

# View WAF logs
docker compose logs -f nginx
```

### Option 2: Build Image Separately

```bash
cd deployment/waf
docker build -t catalyst-nginx-waf -f Dockerfile.nginx-modsecurity .
```

## Configuration Files

| File | Purpose |
|------|---------|
| `modsecurity.conf` | Core ModSecurity configuration |
| `crs-setup.conf` | OWASP CRS tuning for ad-tech workloads |
| `catalyst-rules.conf` | Custom rules for OpenRTB/auction protection |
| `nginx-waf.conf` | Nginx configuration with ModSecurity enabled |

## Rule Data Files

These files can be updated at runtime without rebuilding:

| File | Purpose |
|------|---------|
| `bad-ips.txt` | Blocked IP addresses/ranges |
| `bad-user-agents.txt` | Blocked user agent patterns |
| `bad-publishers.txt` | Known fraudulent publisher IDs |
| `bad-domains.txt` | Known fraudulent domains |
| `trusted-ips.txt` | IPs to run in DetectionOnly mode |

### Updating Rule Data

1. Edit the file in `deployment/waf/`
2. Reload nginx: `docker compose exec nginx nginx -s reload`

## Custom Rules Overview

### Request Validation (9500000-9500099)
- Require Content-Type header on POST
- Require application/json on auction endpoint
- Block empty request bodies
- Block null bytes (injection attempts)

### OpenRTB Validation (9500100-9500199)
- Require `id` field in bid request
- Require `imp` array in bid request
- Block excessive impressions (>100)
- Block requests with both `site` and `app`

### Bid Manipulation Protection (9500200-9500299)
- Block suspiciously high bidfloor (>$1000 CPM)
- Block negative bidfloor values
- Block excessive tmax (>30 seconds)

### Bot/Fraud Detection (9500300-9500399)
- Block known scanner user agents
- Block curl/wget on auction without API key
- Detect device type spoofing patterns

### Rate Abuse Protection (9500400-9500499)
- Track requests per IP (60s window)
- Alert on >200 req/min from single IP
- Block on >500 req/min from single IP

### Known Bad Actors (9500500-9500599)
- Block IPs from `bad-ips.txt`
- Block publisher IDs from `bad-publishers.txt`
- Block domains from `bad-domains.txt`

## Testing

Run the test suite against a staging environment:

```bash
# Set target URL
export WAF_TEST_URL=https://staging.catalyst.springwire.ai

# Run tests
chmod +x test-waf.sh
./test-waf.sh
```

## Detection vs Blocking Mode

### Detection Only (Logging)

Set in `modsecurity.conf`:
```
SecRuleEngine DetectionOnly
```

This logs all would-be blocks without actually blocking. Use for:
- Initial deployment
- Testing new rules
- Debugging false positives

### Blocking Mode (Production)

Set in `modsecurity.conf`:
```
SecRuleEngine On
```

Blocks requests that match rules. Use after validating in DetectionOnly mode.

## Viewing Logs

### Audit Log (Detailed)
```bash
# All blocked requests with full details
tail -f /var/log/nginx/modsec_audit.log
```

### Debug Log (Rule Processing)
```bash
# Set SecDebugLogLevel to 3+ in modsecurity.conf first
tail -f /var/log/nginx/modsec_debug.log
```

### JSON Access Log
```bash
# Structured logs for analysis
tail -f /var/log/nginx/access_waf.json | jq .
```

## Tuning for False Positives

If legitimate traffic is being blocked:

1. Check audit log for the rule ID:
   ```bash
   grep "id \"9500" /var/log/nginx/modsec_audit.log
   ```

2. Add exclusion in `crs-setup.conf`:
   ```
   # Exclude specific field from rule
   SecRuleUpdateTargetById 942100 "!ARGS:your.field.name"
   ```

3. Or disable rule entirely:
   ```
   SecRuleRemoveById 942100
   ```

4. Reload nginx:
   ```bash
   docker compose exec nginx nginx -s reload
   ```

## Paranoia Level

The OWASP CRS paranoia level is set to 1 (low) by default. Increase for stricter protection:

In `crs-setup.conf`:
```
setvar:tx.blocking_paranoia_level=2
```

Levels:
- **1**: Low (recommended to start)
- **2**: Standard
- **3**: High (may cause false positives)
- **4**: Maximum (likely false positives)

## Performance Impact

Expected latency overhead:
- **Simple requests**: ~1-2ms
- **Large JSON payloads**: ~5-10ms
- **With full request logging**: ~10-20ms

To reduce impact:
- Set `SecAuditEngine RelevantOnly` (only log blocks)
- Disable response body inspection: `SecResponseBodyAccess Off`
- Use DetectionOnly for trusted IPs

## Maintenance

### Monthly Tasks
- Review audit logs for blocked patterns
- Update `bad-ips.txt` with new threat intelligence
- Check for CRS updates: https://coreruleset.org/

### When Adding New Endpoints
- Review if WAF rules need exclusions
- Test new endpoints with `test-waf.sh`
- Add endpoint-specific rules if needed
