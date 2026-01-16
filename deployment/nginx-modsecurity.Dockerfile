# ModSecurity + Nginx WAF for Catalyst Auction Server
# Built on official OWASP ModSecurity CRS image with Alpine base

FROM owasp/modsecurity-crs:nginx-alpine

LABEL maintainer="thenexusengine"
LABEL description="Nginx + ModSecurity WAF for OpenRTB auction server"
LABEL version="1.0.0"

# Install additional tools for monitoring and debugging
USER root
RUN apk add --no-cache \
    curl \
    wget \
    tzdata \
    ca-certificates

# Set timezone
ENV TZ=UTC

# ModSecurity configuration
ENV MODSEC_RULE_ENGINE=On \
    MODSEC_AUDIT_ENGINE=RelevantOnly \
    MODSEC_AUDIT_LOG=/var/log/modsec/audit.log \
    MODSEC_AUDIT_LOG_FORMAT=JSON \
    MODSEC_AUDIT_LOG_TYPE=Serial \
    MODSEC_AUDIT_STORAGE=/var/log/modsec/ \
    MODSEC_DEBUG_LOG=/dev/null \
    MODSEC_DEBUG_LOGLEVEL=0 \
    MODSEC_PCRE_MATCH_LIMIT=100000 \
    MODSEC_PCRE_MATCH_LIMIT_RECURSION=100000

# OWASP CRS configuration
# Paranoia Level: 1 = Basic protection (lower false positives)
# 2 = Moderate (recommended for production after tuning)
# 3-4 = High/Paranoid (expect false positives, requires extensive tuning)
ENV PARANOIA=2 \
    ANOMALY_INBOUND=5 \
    ANOMALY_OUTBOUND=4 \
    BLOCKING_PARANOIA=2

# Performance tuning
ENV EXECUTING_PARANOIA=2 \
    ENFORCE_BODYPROC_URLENCODED=1 \
    ALLOWED_METHODS="GET HEAD POST OPTIONS PUT DELETE" \
    ALLOWED_REQUEST_CONTENT_TYPE="application/json|application/x-www-form-urlencoded|multipart/form-data|text/xml|application/xml|application/soap+xml" \
    ALLOWED_HTTP_VERSIONS="HTTP/1.0 HTTP/1.1 HTTP/2 HTTP/2.0" \
    RESTRICTED_EXTENSIONS=".asa/ .asax/ .ascx/ .axd/ .backup/ .bak/ .bat/ .cdx/ .cer/ .cfg/ .cmd/ .com/ .config/ .conf/ .cs/ .csproj/ .csr/ .dat/ .db/ .dbf/ .dll/ .dos/ .htr/ .htw/ .ida/ .idc/ .idq/ .inc/ .ini/ .key/ .licx/ .lnk/ .log/ .mdb/ .old/ .pass/ .pdb/ .pol/ .printer/ .pwd/ .rdb/ .resources/ .resx/ .sql/ .swp/ .sys/ .vb/ .vbs/ .vbproj/ .vsdisco/ .webinfo/ .xsd/ .xsx/" \
    RESTRICTED_HEADERS="/content-encoding/ /proxy/ /lock-token/ /content-range/ /if/ /x-http-method-override/ /x-http-method/ /x-method-override/" \
    STATIC_EXTENSIONS="/.jpg/ /.jpeg/ /.png/ /.gif/ /.js/ /.css/ /.ico/ /.svg/ /.webp/"

# Ad-tech specific settings
# Higher body limits for OpenRTB bid requests
ENV MAX_FILE_SIZE=2097152 \
    COMBINED_FILE_SIZES=2097152 \
    REQ_BODY_ACCESS=On \
    REQ_BODY_LIMIT=2097152 \
    REQ_BODY_LIMIT_ACTION=Reject \
    REQ_BODY_JSON_DEPTH_LIMIT=512 \
    REQ_BODY_NO_FILES_LIMIT=2097152 \
    RESP_BODY_ACCESS=On \
    RESP_BODY_LIMIT=524288 \
    RESP_BODY_LIMIT_ACTION=ProcessPartial

# CRS tuning for JSON-heavy traffic
ENV TX_ALLOWED_METHODS="GET HEAD POST OPTIONS PUT DELETE" \
    TX_ALLOWED_REQUEST_CONTENT_TYPE="application/json|application/x-www-form-urlencoded|multipart/form-data" \
    TX_ARG_LENGTH=8000 \
    TX_ARG_NAME_LENGTH=400 \
    TX_COMBINED_FILE_SIZES=2097152 \
    TX_CRITICAL_ANOMALY_SCORE=5 \
    TX_ERROR_ANOMALY_SCORE=4 \
    TX_HTTP_VIOLATION_SCORE=5 \
    TX_INBOUND_ANOMALY_SCORE_THRESHOLD=5 \
    TX_LAFI_SCORE=3 \
    TX_MAX_FILE_SIZE=2097152 \
    TX_MAX_NUM_ARGS=512 \
    TX_OUTBOUND_ANOMALY_SCORE_THRESHOLD=4 \
    TX_PARANOIA_LEVEL=2 \
    TX_RFI_SCORE=5 \
    TX_SESSION_FIXATION_SCORE=5 \
    TX_SQL_INJECTION_SCORE=5 \
    TX_TROJAN_SCORE=5 \
    TX_WARNING_ANOMALY_SCORE=3 \
    TX_XSS_SCORE=5

# Create log directory
RUN mkdir -p /var/log/modsec && \
    mkdir -p /etc/nginx/modsecurity.d/custom-rules && \
    chown -R nginx:nginx /var/log/modsec

# Copy custom rules (will be created next)
COPY modsecurity/custom-rules/*.conf /etc/nginx/modsecurity.d/custom-rules/

# Copy ModSecurity main config
COPY modsecurity/modsecurity.conf /etc/modsecurity.d/modsecurity.conf

# Copy CRS setup
COPY modsecurity/crs-setup.conf /etc/modsecurity.d/owasp-crs/crs-setup.conf

# Copy Nginx configuration
COPY nginx-modsecurity.conf /etc/nginx/conf.d/default.conf

# Health check
HEALTHCHECK --interval=30s --timeout=10s --start-period=20s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:80/health || exit 1

EXPOSE 80 443

STOPSIGNAL SIGQUIT

CMD ["nginx", "-g", "daemon off;"]
