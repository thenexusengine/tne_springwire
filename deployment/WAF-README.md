# ModSecurity WAF Protection for Catalyst Auction Server

## Overview

This deployment includes a **Web Application Firewall (WAF)** layer using **ModSecurity** + **OWASP Core Rule Set (CRS)** to protect your OpenRTB auction server from application-layer attacks.

### Why WAF for Ad-Tech?

1. **Financial Target**: Ad-tech servers process money (CPM bids), making them attractive for fraud and bid manipulation
2. **Complex JSON Payloads**: OpenRTB bid requests are vulnerable to JSON injection, malformed requests, and DoS attacks
3. **Bot Traffic**: Automated scanners probe for vulnerabilities, wasting resources
4. **Content Injection**: Malicious creatives can contain XSS, crypto miners, or phishing attempts

### Protection Features

- ✅ **OWASP Core Rule Set v3.3.5**: Industry-standard attack signatures
- ✅ **Custom OpenRTB Rules**: Ad-tech specific attack detection (bid manipulation, publisher enumeration, etc.)
- ✅ **JSON Deep Inspection**: Validates OpenRTB protocol compliance
- ✅ **Bot Detection**: Blocks known scanners and malicious user agents
- ✅ **DoS Protection**: Detects excessive impressions, deep JSON nesting, rapid requests
- ✅ **Creative Filtering**: Blocks malicious scripts, iframes, crypto miners, and eval()
- ✅ **SQL Injection / XSS / RCE Protection**: Blocks common web attacks
- ✅ **Anomaly Scoring**: Graduated response based on threat severity

## Architecture

```
Internet → Nginx + ModSecurity WAF → Catalyst Auction Server
                ↓
          OWASP CRS Rules
                ↓
        Custom OpenRTB Rules
                ↓
        Block/Allow/Log Decision
```

## Quick Start

### 1. Deploy with ModSecurity

```bash
cd deployment

# Use ModSecurity-enabled docker-compose
docker-compose -f docker-compose-modsecurity.yml up -d

# Check WAF status
docker logs catalyst-nginx-waf
```

### 2. Verify WAF is Active

```bash
# Test with a malicious request (should be blocked)
curl -X POST https://catalyst.springwire.ai/openrtb2/auction \
  -H "Content-Type: application/json" \
  -d '{"id":"test","imp":[{"id":"1"}],"site":{"domain":"example.com' OR 1=1--"}}'

# Expected: 403 Forbidden
```

### 3. Monitor WAF Logs

```bash
# Live ModSecurity audit log (JSON format)
tail -f modsec-logs/audit.log | jq .

# Nginx access log with ModSecurity transaction IDs
tail -f nginx-logs/access.log | grep modsec
```

## Configuration

### Paranoia Levels

ModSecurity uses "paranoia levels" to control strictness:

| Level | Description | Recommended For | False Positives |
|-------|-------------|----------------|-----------------|
| **1** | Basic protection | Minimal tuning | Very Low |
| **2** | Moderate protection (DEFAULT) | Production after tuning | Low |
| **3** | High protection | High-security environments | Medium |
| **4** | Paranoid | Post-extensive tuning | High |

**Default**: Paranoia Level 2 (balanced protection)

To change paranoia level:

```bash
# Edit .env file
PARANOIA=1  # Lower paranoia (fewer false positives)
BLOCKING_PARANOIA=1

# Restart
docker-compose -f docker-compose-modsecurity.yml up -d nginx
```

### Anomaly Scoring

ModSecurity uses **anomaly scoring** instead of blocking on first match:

- Each rule adds points when triggered
- **Inbound Threshold**: 5 points (block request)
- **Outbound Threshold**: 4 points (block response)

**Example**: SQL injection attempt = +5 points → BLOCKED

```bash
# Adjust thresholds in .env
ANOMALY_INBOUND=10  # More permissive (allows more points before blocking)
ANOMALY_OUTBOUND=8

# Restart
docker-compose -f docker-compose-modsecurity.yml up -d nginx
```

### Detection vs Blocking Mode

ModSecurity can run in three modes:

| Mode | Setting | Behavior |
|------|---------|----------|
| **DetectionOnly** | `MODSEC_RULE_ENGINE=DetectionOnly` | Logs attacks but doesn't block |
| **On** (DEFAULT) | `MODSEC_RULE_ENGINE=On` | Logs and blocks attacks |
| **Off** | `MODSEC_RULE_ENGINE=Off` | Completely disabled |

**Testing Strategy**:

```bash
# Phase 1: Detection only (1-2 weeks)
MODSEC_RULE_ENGINE=DetectionOnly

# Analyze logs, tune rules, reduce false positives

# Phase 2: Enable blocking
MODSEC_RULE_ENGINE=On
```

## Monitoring

### Key Metrics to Monitor

1. **Block Rate**: `grep "403" nginx-logs/access.log | wc -l`
2. **ModSec Transaction Rate**: `grep "modsec" nginx-logs/access.log | wc -l`
3. **Attack Types**: `jq '.transaction.messages[] | .details.ruleId' modsec-logs/audit.log | sort | uniq -c`
4. **Top Blocked IPs**: `grep "403" nginx-logs/access.log | awk '{print $1}' | sort | uniq -c | sort -rn | head`

### Dashboard Queries (Prometheus/Grafana)

```promql
# Block rate
rate(nginx_http_requests_total{status="403"}[5m])

# ModSec rule hits by ID
increase(modsec_rule_hits_total[1h])

# Top attack types
topk(10, sum by (attack_type) (modsec_attacks_total))
```

### Alert on Anomalies

```yaml
# Example Prometheus alert
- alert: HighWAFBlockRate
  expr: rate(nginx_http_requests_total{status="403"}[5m]) > 10
  for: 5m
  annotations:
    summary: "High WAF block rate detected"
    description: "More than 10 requests/sec being blocked by ModSecurity"
```

## Tuning for False Positives

### 1. Identify False Positives

```bash
# Find all blocked requests
jq 'select(.transaction.messages != null) | {
  ip: .transaction.client_ip,
  uri: .transaction.request.uri,
  rules: [.transaction.messages[] | .details.ruleId]
}' modsec-logs/audit.log

# Focus on your legitimate traffic patterns
```

### 2. Disable Specific Rules

Create `/deployment/modsecurity/custom-rules/exclusions.conf`:

```apache
# Disable rule 920100 for OpenRTB endpoint (example)
SecRuleRemoveById 920100

# Or disable by tag
SecRuleRemoveByTag "attack-sqli"

# Or disable for specific URI
SecRule REQUEST_URI "@beginsWith /openrtb2/auction" \
    "id:90000,phase:1,pass,nolog,ctl:ruleRemoveById=920100"
```

### 3. Whitelist Specific Parameters

```apache
# Don't inspect publisher.id for SQL injection (if causing FPs)
SecRule REQUEST_URI "@beginsWith /openrtb2/auction" \
    "id:90001,phase:2,pass,nolog,\
    ctl:ruleRemoveTargetByTag=attack-sqli;ARGS:publisher.id"
```

### 4. Adjust Anomaly Scores

```apache
# Reduce score for specific rule
SecRuleUpdateActionById 942100 "setvar:tx.sql_injection_score=2"

# Instead of default 5 points
```

## Custom Rules

### Adding Your Own Rules

Create `/deployment/modsecurity/custom-rules/my-rules.conf`:

```apache
# Block requests from specific country (example: North Korea)
SecRule GEO:COUNTRY_CODE "@streq KP" \
    "id:20000,\
    phase:1,\
    block,\
    msg:'Request from blocked country',\
    severity:WARNING"

# Require specific header for admin endpoints
SecRule REQUEST_URI "@beginsWith /admin/" \
    "id:20001,\
    phase:1,\
    pass,\
    chain"
    SecRule &REQUEST_HEADERS:X-Admin-Token "@eq 0" \
        "block,\
        msg:'Missing admin token',\
        severity:ERROR"

# Detect suspiciously high bid prices (>$100 CPM)
SecRule REQUEST_BODY "@rx \"price\":\\s*([1-9]\d{2,})" \
    "id:20002,\
    phase:2,\
    block,\
    msg:'Suspiciously high bid price',\
    severity:WARNING"
```

### Rule ID Ranges

| Range | Purpose |
|-------|---------|
| 1-99,999 | Reserved for CRS |
| 100,000-199,999 | Local site rules (yours) |
| 200,000-299,999 | Reserved for CRS |
| 300,000-399,999 | Reserved for CRS |
| 400,000-419,999 | Reserved for CRS |
| 420,000-429,999 | Experimental rules |
| 430,000-899,999 | Unsupported rules |
| 900,000-999,999 | Reserved for CRS |

**Use range 100,000-199,999 for your custom rules.**

## Performance Impact

### Expected Overhead

| Component | Overhead | Mitigation |
|-----------|----------|------------|
| CPU | +5-15% | Use lower paranoia level |
| Memory | +200-500MB | Adequate RAM allocation |
| Latency | +1-5ms | Optimize rule set |
| Throughput | -5-10% | Hardware acceleration |

### Optimization Tips

1. **Disable unnecessary CRS rules**:
   ```apache
   # If you don't use PHP
   SecRuleRemoveByTag "language-php"
   ```

2. **Reduce regex complexity**:
   ```apache
   # Increase PCRE limits for complex patterns
   SecPcreMatchLimit 500000
   ```

3. **Use sampling** (only inspect % of traffic):
   ```apache
   # Inspect 50% of requests
   SecRule TX:SAMPLING_PERCENTAGE "@ge 50" "id:90100,phase:1,pass,skipAfter:END_SAMPLING"
   ```

## Troubleshooting

### WAF Not Blocking

```bash
# Check ModSecurity is loaded
docker exec catalyst-nginx-waf nginx -V 2>&1 | grep modsecurity

# Check rule engine is On
docker exec catalyst-nginx-waf cat /etc/modsecurity.d/modsecurity.conf | grep SecRuleEngine

# Check logs for errors
docker logs catalyst-nginx-waf
tail -f nginx-logs/error.log
```

### Too Many False Positives

```bash
# 1. Switch to detection mode
MODSEC_RULE_ENGINE=DetectionOnly

# 2. Analyze what's being blocked
jq '.transaction.messages[] | {rule: .details.ruleId, msg: .details.message}' \
   modsec-logs/audit.log | sort | uniq -c | sort -rn

# 3. Disable problematic rules
echo "SecRuleRemoveById <rule_id>" >> modsecurity/custom-rules/exclusions.conf

# 4. Re-enable blocking
MODSEC_RULE_ENGINE=On
```

### High Latency

```bash
# Check which rules are slowest
grep "Stopwatch" modsec-logs/debug.log | sort -t: -k4 -rn | head

# Disable slow rules
SecRuleRemoveById <slow_rule_id>

# Or lower paranoia level
PARANOIA=1
```

### Memory Issues

```bash
# Increase container memory
docker-compose -f docker-compose-modsecurity.yml up -d --scale nginx=0
# Edit resources.limits.memory: '2G'
docker-compose -f docker-compose-modsecurity.yml up -d nginx

# Or reduce collection timeout
SecCollectionTimeout 300  # 5 minutes instead of 10
```

## Security Best Practices

### 1. Keep CRS Updated

```bash
# Pull latest CRS version
docker pull owasp/modsecurity-crs:3.3.5-nginx-alpine

# Rebuild
docker-compose -f docker-compose-modsecurity.yml build --no-cache nginx
docker-compose -f docker-compose-modsecurity.yml up -d nginx
```

### 2. Secure Audit Logs

```bash
# Rotate logs
logrotate /etc/logrotate.d/modsec

# Or use Docker logging driver
logging:
  driver: "syslog"
  options:
    syslog-address: "tcp://logstash:5000"
```

### 3. Integrate with SIEM

```bash
# Forward ModSec logs to Splunk/ELK/Datadog
filebeat -c filebeat-modsec.yml
```

### 4. Test Regularly

```bash
# Use OWASP ZAP or Burp Suite to scan your endpoint
zap-cli quick-scan --self-contained https://catalyst.springwire.ai/openrtb2/auction

# Verify WAF blocks attacks
```

## References

- [OWASP ModSecurity CRS Documentation](https://coreruleset.org/docs/)
- [ModSecurity Reference Manual](https://github.com/SpiderLabs/ModSecurity/wiki/Reference-Manual)
- [Nginx ModSecurity Module](https://github.com/SpiderLabs/ModSecurity-nginx)
- [OpenRTB 2.x Specification](https://www.iab.com/wp-content/uploads/2016/03/OpenRTB-API-Specification-Version-2-5-FINAL.pdf)

## Support

For issues or questions:
1. Check logs: `docker logs catalyst-nginx-waf`
2. Review audit log: `tail modsec-logs/audit.log`
3. Test with: `curl -v https://catalyst.springwire.ai/health`
4. File issue: [GitHub Issues](https://github.com/thenexusengine/tne_springwire/issues)

---

**Deployed**: `feature/modsecurity-waf-protection` branch
**Status**: Production-ready (test in staging first)
