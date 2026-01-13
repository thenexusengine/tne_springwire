# TNE Catalyst Deployment Folder

**Master Guide and Documentation Index**

This folder contains everything needed to deploy TNE Catalyst to production at **catalyst.springwire.ai**.

---

## Quick Start

**First time deploying?** Start here: `../DEPLOYMENT_GUIDE.md`

**Updating existing deployment?** See "Common Tasks" section below.

---

## Local Development vs Server Deployment

### Local Development

For local development, Docker Compose automatically uses `docker-compose.override.yml` to build from your local source code:

```bash
# Create .env from dev template
cp .env.dev .env

# Start services (uses local source code via override file)
docker compose up -d

# Test the geo-consent filtering
cd ../examples
CATALYST_URL=http://localhost:8000 ./test-geo-consent.sh

# View logs
docker compose logs -f catalyst

# Stop services
docker compose down
```

The `docker-compose.override.yml` file (not committed to git) overrides the GitHub build context with your local code (`context: ..`).

### Server Deployment

On production/staging servers, explicitly use only `docker-compose.yml` to build from GitHub:

```bash
# Create .env from production template
cp .env.production .env

# Start services (builds from GitHub, ignoring override file)
docker compose -f docker-compose.yml up -d

# View logs
docker compose -f docker-compose.yml logs -f

# Stop services
docker compose -f docker-compose.yml down
```

**Why this works:**
- `docker-compose.yml` builds from GitHub (for servers)
- `docker-compose.override.yml` overrides to use local code (for development)
- Docker Compose automatically merges both files when running `docker compose up`
- Specifying `-f docker-compose.yml` explicitly uses only that file (server mode)

---

## Folder Structure

```
deployment/
├── README.md                      ← You are here (master guide)
│
├── .env.dev                       ← Development environment config
├── .env.production                ← Production environment config (100% traffic)
├── .env.staging                   ← Staging environment config (5% traffic)
├── README-env.md                  ← Complete environment variable reference
│
├── docker-compose.yml             ← Regular deployment (single environment)
├── docker-compose-split.yml       ← Traffic splitting (95% prod, 5% staging)
├── README-docker-compose.md       ← Docker Compose documentation
│
├── nginx.conf                     ← Nginx config (regular deployment)
├── nginx-split.conf               ← Nginx config (traffic splitting)
├── README-nginx.md                ← Nginx configuration guide
│
├── README-environments.md         ← Environment strategy explained
├── README-traffic-splitting.md    ← Traffic splitting guide (canary)
├── README-monitoring.md           ← Performance comparison guide
│
├── compare-performance.sh         ← Automated performance comparison tool
│
└── DEPLOYMENT-CHECKLIST.md        ← Pre-deployment checklist
```

**Generated at runtime:**
- `ssl/` - SSL certificates (create this, add certs)
- `nginx-logs/` - Nginx access and error logs
- `redis-data/` - Redis persistent storage (Docker volume)

---

## File Purposes

### Configuration Files

#### .env.dev
- **Purpose**: Local development configuration
- **Use when**: Working on your laptop/workstation
- **Characteristics**: Permissive CORS, debug logging, localhost URLs
- **Usage**: `docker compose --env-file .env.dev up -d`

#### .env.production
- **Purpose**: Production configuration (100% traffic)
- **Use when**: Normal production deployment
- **Characteristics**: Strict security, JSON logging, production URLs
- **Important**: Change passwords before deployment
- **Usage**: `docker compose --env-file .env.production up -d`

#### .env.staging
- **Purpose**: Staging configuration (5% traffic testing)
- **Use when**: Testing new features with real traffic
- **Characteristics**: Separate Redis, aggressive IVT testing, debug logging
- **Usage**: With `docker-compose-split.yml`

### Docker Compose Files

#### docker-compose.yml
- **Purpose**: Regular single-environment deployment
- **Services**: 1 Catalyst, 1 Redis, 1 Nginx
- **Use when**: Standard production deployment (100% traffic)
- **Usage**: `docker compose up -d`

#### docker-compose-split.yml
- **Purpose**: Traffic splitting deployment (canary testing)
- **Services**: 2 Catalyst (prod + staging), 2 Redis, 1 Nginx
- **Use when**: Testing new features with 5% traffic
- **Usage**: `docker compose -f docker-compose-split.yml up -d`

### Nginx Configuration

#### nginx.conf
- **Purpose**: Reverse proxy for regular deployment
- **Features**: SSL termination, rate limiting, CORS, security headers
- **Routes to**: Single Catalyst container
- **Use with**: docker-compose.yml

#### nginx-split.conf
- **Purpose**: Reverse proxy with traffic splitting
- **Features**: Same as nginx.conf + split_clients (95/5 split)
- **Routes to**: 2 Catalyst containers (prod + staging)
- **Use with**: docker-compose-split.yml

### Tools

#### compare-performance.sh
- **Purpose**: Compare production vs staging performance
- **Metrics**: Response time, errors, resource usage, IVT, auctions
- **Usage**: `./compare-performance.sh`
- **Requires**: Traffic splitting deployment

### Documentation

#### README-env.md
- **What**: Complete environment variable reference
- **Contains**: Every env var explained, valid values, security notes
- **Read when**: Configuring .env files

#### README-docker-compose.md
- **What**: Docker Compose configuration explained
- **Contains**: Service definitions, resource limits, networking
- **Read when**: Understanding container setup

#### README-nginx.md
- **What**: Nginx configuration explained
- **Contains**: Rate limits, timeouts, SSL setup, CORS
- **Read when**: Configuring reverse proxy

#### README-environments.md
- **What**: Environment strategy explained
- **Contains**: Dev vs prod vs staging differences
- **Read when**: Choosing environment approach

#### README-traffic-splitting.md
- **What**: Traffic splitting guide
- **Contains**: How split_clients works, deployment steps, monitoring
- **Read when**: Setting up canary deployment

#### README-monitoring.md
- **What**: Performance comparison and monitoring
- **Contains**: Manual commands, automated tools, decision criteria
- **Read when**: Monitoring split deployment

#### DEPLOYMENT-CHECKLIST.md
- **What**: Pre-deployment checklist
- **Contains**: All steps to verify before going live
- **Read when**: Before production deployment

---

## Deployment Scenarios

### Scenario 1: Regular Production (100% traffic)

**Files used:**
- `docker-compose.yml`
- `nginx.conf`
- `.env.production`

**Steps:**
```bash
cd /opt/catalyst
docker compose --env-file .env.production up -d
```

**Architecture:**
```
Internet → Nginx (443) → Catalyst (8000) → Redis
```

### Scenario 2: Traffic Splitting (95% prod, 5% staging)

**Files used:**
- `docker-compose-split.yml`
- `nginx-split.conf`
- `.env.production`
- `.env.staging`

**Steps:**
```bash
cd /opt/catalyst
docker compose -f docker-compose-split.yml up -d
```

**Architecture:**
```
Internet → Nginx (443) → split_clients (95/5)
                         ├─ Catalyst-Prod (8000) → Redis-Prod
                         └─ Catalyst-Staging (8001) → Redis-Staging
```

### Scenario 3: Local Development

**Files used:**
- `docker-compose.yml`
- `nginx.conf`
- `.env.dev`

**Steps:**
```bash
cd /path/to/project
docker compose --env-file deployment/.env.dev up -d
```

**Architecture:**
```
Localhost → Nginx (80/443) → Catalyst (8000) → Redis
```

---

## Common Tasks

### Deploy for First Time

**See:** `../DEPLOYMENT_GUIDE.md` for complete step-by-step guide.

**Quick version:**
```bash
# 1. Setup directory
sudo mkdir -p /opt/catalyst
cd /opt/catalyst

# 2. Copy files
git clone https://github.com/thenexusengine/tne_springwire.git
cp tne_springwire/deployment/* .

# 3. Setup SSL
mkdir ssl
cp /path/to/fullchain.pem ssl/
cp /path/to/privkey.pem ssl/

# 4. Configure
cp .env.production .env
nano .env  # Update passwords and CORS

# 5. Deploy
docker compose up -d

# 6. Verify
curl https://catalyst.springwire.ai/health
```

### Update to Latest Code

```bash
cd /opt/catalyst

# Pull latest images
docker compose pull

# Restart with new images
docker compose up -d

# Verify
docker compose ps
docker compose logs -f catalyst-prod
```

### Switch from Regular to Traffic Splitting

```bash
cd /opt/catalyst

# Stop regular deployment
docker compose down

# Start split deployment
docker compose -f docker-compose-split.yml up -d

# Verify both containers running
docker compose -f docker-compose-split.yml ps

# Monitor performance
./compare-performance.sh
```

### Switch from Traffic Splitting to Regular

```bash
cd /opt/catalyst

# Stop split deployment
docker compose -f docker-compose-split.yml down

# Start regular deployment
docker compose up -d

# Verify
docker compose ps
```

### Change Environment Variables

```bash
cd /opt/catalyst

# Edit .env file
nano .env

# Restart to apply changes
docker compose restart catalyst-prod

# Or restart all services
docker compose restart
```

### Adjust Traffic Split Percentage

**Example: Change from 95/5 to 90/10**

```bash
cd /opt/catalyst

# Edit nginx config
nano nginx-split.conf

# Find split_clients block, change:
#   95%     prod;      →  90%     prod;
#   *       staging;   →  *       staging; (now gets 10%)

# Reload nginx
docker compose -f docker-compose-split.yml exec nginx nginx -s reload
```

### View Logs

```bash
cd /opt/catalyst

# All logs
docker compose logs -f

# Specific service
docker compose logs -f catalyst-prod

# Recent errors only
docker compose logs --tail=100 catalyst-prod | grep error

# Nginx access logs
tail -f nginx-logs/access.log

# Nginx error logs
tail -f nginx-logs/error.log
```

### Check Performance

```bash
cd /opt/catalyst

# Automated comparison (if traffic splitting)
./compare-performance.sh

# Container resources
docker stats

# Response times (manual)
docker logs --since 60m catalyst-prod 2>&1 | \
  grep "duration_ms" | \
  grep -oP 'duration_ms":\K[0-9.]+' | \
  awk '{ sum += $1; n++ } END { print "Avg:", sum/n, "ms" }'

# Error count
docker logs --since 60m catalyst-prod 2>&1 | grep -c '"level":"error"'
```

### Restart Services

```bash
cd /opt/catalyst

# Restart all
docker compose restart

# Restart specific service
docker compose restart catalyst-prod

# Restart nginx (reload config)
docker compose exec nginx nginx -s reload
```

### Troubleshoot Issues

**Container won't start:**
```bash
# Check logs
docker compose logs catalyst-prod

# Common issues:
# - Database connection → Check DB_PASSWORD in .env
# - Redis connection → Check REDIS_PASSWORD in .env
# - Port conflict → Check if port 80/443 in use
```

**CORS errors:**
```bash
# Check CORS config
grep CORS_ALLOWED_ORIGINS .env

# Update and restart
nano .env
docker compose restart catalyst-prod
```

**Performance issues:**
```bash
# Check resource usage
docker stats

# Check for errors
docker compose logs catalyst-prod | grep -i error

# Review rate limits in nginx.conf
```

---

## Decision Matrix

### Which Docker Compose File?

| Scenario | Use | Reason |
|----------|-----|--------|
| Standard production | docker-compose.yml | Simple, 100% traffic |
| Testing new feature | docker-compose-split.yml | Safe, 5% traffic testing |
| A/B testing config | docker-compose-split.yml | Compare performance |
| Local development | docker-compose.yml | Simpler, one environment |

### Which Environment File?

| Environment | Use | Characteristics |
|-------------|-----|-----------------|
| .env.dev | Local development | Permissive, debug logging |
| .env.production | Production (100%) | Secure, JSON logging |
| .env.staging | Staging (5%) | Test new configs, aggressive IVT |

### Which Nginx Config?

| Config | Use With | Features |
|--------|----------|----------|
| nginx.conf | docker-compose.yml | Single backend |
| nginx-split.conf | docker-compose-split.yml | Split traffic 95/5 |

---

## Monitoring & Maintenance

### Daily Checks

```bash
# 1. Check container health
docker compose ps

# 2. Check for errors
docker compose logs --since 24h catalyst-prod | grep -i error

# 3. Check response times
docker logs --since 24h catalyst-prod 2>&1 | grep duration_ms | tail -20

# 4. Check disk space
df -h

# 5. Check memory usage
docker stats --no-stream
```

### Weekly Tasks

```bash
# 1. Review error patterns
docker compose logs --since 7d catalyst-prod | grep error > weekly-errors.log

# 2. Check SSL expiry
openssl x509 -in ssl/fullchain.pem -text -noout | grep "Not After"

# 3. Clean old logs
find nginx-logs/ -name "*.log" -mtime +30 -delete

# 4. Update images
docker compose pull
docker compose up -d

# 5. Database backup
docker exec -i postgres pg_dump -U catalyst_prod catalyst_production > backup_$(date +%Y%m%d).sql
```

### Monthly Tasks

```bash
# 1. Security updates
sudo apt update && sudo apt upgrade -y

# 2. Review rate limits (adjust if needed)
nano nginx.conf

# 3. Review resource allocation
docker stats

# 4. Clean Docker system
docker system prune -a

# 5. Review monitoring data
./compare-performance.sh > monthly-report.txt
```

---

## Rollback Procedures

### Rollback to Previous Version

```bash
# If deployment went wrong

# 1. Stop current deployment
docker compose down

# 2. Restore previous config
cp .env.backup .env

# 3. Use previous Docker image tag
# Edit docker-compose.yml, change:
#   image: thenexusengine/catalyst:latest
#   to:
#   image: thenexusengine/catalyst:v1.2.3  # previous version

# 4. Restart
docker compose up -d
```

### Rollback from Traffic Split to Regular

```bash
# If staging has issues

# Stop split deployment (keeps production running during switch)
docker compose -f docker-compose-split.yml down

# Start regular deployment (100% production)
docker compose up -d

# Result: Back to 100% production, staging disabled
```

---

## Security Best Practices

### Before Production Deployment

- [ ] Changed DB_PASSWORD (strong, unique)
- [ ] Changed REDIS_PASSWORD (strong, unique)
- [ ] Set CORS_ALLOWED_ORIGINS (specific domains, not *)
- [ ] Enabled SSL (HTTPS only)
- [ ] Set DB_SSL_MODE=require (minimum)
- [ ] Disabled debug endpoints (PPROF_ENABLED=false)
- [ ] Set LOG_LEVEL=info (not debug)
- [ ] Configured firewall (allow 80, 443; block 8000)
- [ ] SSL auto-renewal configured
- [ ] Backup strategy in place

### Ongoing Security

```bash
# 1. Regular updates
sudo apt update && sudo apt upgrade -y

# 2. Monitor for vulnerabilities
docker scan thenexusengine/catalyst:latest

# 3. Review logs for suspicious activity
docker compose logs catalyst-prod | grep -i "suspicious\|blocked"

# 4. Keep SSL certificates current
certbot renew --dry-run

# 5. Rotate credentials periodically
# Update DB_PASSWORD and REDIS_PASSWORD every 90 days
```

---

## Performance Tuning

### Low-Traffic Site (< 1000 req/day)

```bash
# .env adjustments
DB_MAX_OPEN_CONNS=25
REDIS_POOL_SIZE=10
RATE_LIMIT_GENERAL=50
AUCTION_MAX_BIDDERS=8

# docker-compose.yml adjustments
resources:
  limits:
    cpus: '1.0'
    memory: 2G
```

### Medium-Traffic Site (1K-10K req/day)

```bash
# .env adjustments (default production values)
DB_MAX_OPEN_CONNS=100
REDIS_POOL_SIZE=50
RATE_LIMIT_GENERAL=100
AUCTION_MAX_BIDDERS=15

# docker-compose.yml adjustments (default)
resources:
  limits:
    cpus: '2.0'
    memory: 4G
```

### High-Traffic Site (> 10K req/day)

```bash
# .env adjustments
DB_MAX_OPEN_CONNS=200
REDIS_POOL_SIZE=100
RATE_LIMIT_GENERAL=500
AUCTION_MAX_BIDDERS=20

# docker-compose.yml adjustments
resources:
  limits:
    cpus: '4.0'
    memory: 8G

# Consider horizontal scaling (multiple instances + load balancer)
```

---

## File Relationships

```
┌─────────────────────────────────────────────────────────┐
│                   DEPLOYMENT OPTIONS                     │
└─────────────────────────────────────────────────────────┘
                          │
         ┌────────────────┴────────────────┐
         │                                 │
    ┌────▼────┐                      ┌────▼────┐
    │ Regular │                      │  Split  │
    └────┬────┘                      └────┬────┘
         │                                 │
    ┌────▼──────────┐              ┌──────▼──────────┐
    │ docker-       │              │ docker-compose- │
    │ compose.yml   │              │ split.yml       │
    └────┬──────────┘              └──────┬──────────┘
         │                                 │
    ┌────▼────┐                      ┌────▼────┐
    │ nginx.  │                      │ nginx-  │
    │ conf    │                      │split.conf│
    └────┬────┘                      └────┬────┘
         │                                 │
         └────────────┬────────────────────┘
                      │
              ┌───────▼────────┐
              │  Environment   │
              │     Files      │
              └───────┬────────┘
                      │
         ┌────────────┼────────────┐
         │            │            │
    ┌────▼────┐  ┌───▼────┐  ┌───▼────┐
    │ .env.   │  │ .env.  │  │ .env.  │
    │ dev     │  │ prod   │  │ staging│
    └─────────┘  └────────┘  └────────┘
```

---

## Getting Help

### Documentation Index

1. **Quick Start**: `../DEPLOYMENT_GUIDE.md`
2. **Environment Variables**: `README-env.md`
3. **Docker Compose**: `README-docker-compose.md`
4. **Nginx Config**: `README-nginx.md`
5. **Environments Strategy**: `README-environments.md`
6. **Traffic Splitting**: `README-traffic-splitting.md`
7. **Monitoring**: `README-monitoring.md`
8. **Pre-Deployment**: `DEPLOYMENT-CHECKLIST.md`

### Common Questions

**Q: Which file should I edit for production?**
A: `.env.production` (or `.env` if you copied it)

**Q: How do I test new features safely?**
A: Use traffic splitting (docker-compose-split.yml) with .env.staging

**Q: Where are SSL certificates stored?**
A: `./ssl/` directory (fullchain.pem and privkey.pem)

**Q: How do I change rate limits?**
A: Edit `nginx.conf` (or `nginx-split.conf`), then reload: `docker compose exec nginx nginx -s reload`

**Q: How do I add more memory to Redis?**
A: Edit docker-compose.yml, change `--maxmemory 1024mb` to desired value, then `docker compose restart redis-prod`

**Q: Can I run this locally for testing?**
A: Yes! Use `.env.dev` and docker-compose.yml

---

## Version History

| Date | Version | Changes |
|------|---------|---------|
| 2025-01-13 | 1.0.0 | Initial deployment folder structure |
| | | - Added docker-compose configs |
| | | - Added nginx configs |
| | | - Added environment templates |
| | | - Added comprehensive documentation |
| | | - Added performance comparison tool |

---

**Last Updated**: 2025-01-13
**Repository**: https://github.com/thenexusengine/tne_springwire
**Domain**: catalyst.springwire.ai
**Maintainer**: TNE Catalyst Team
