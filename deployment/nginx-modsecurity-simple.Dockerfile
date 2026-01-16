# Simplified ModSecurity + Nginx WAF for Catalyst Auction Server
# Uses OWASP ModSecurity CRS with minimal customization

FROM owasp/modsecurity-crs:nginx-alpine

LABEL maintainer="thenexusengine"
LABEL description="Nginx + ModSecurity WAF for OpenRTB auction server"

# Set timezone
ENV TZ=UTC

# ModSecurity configuration via environment variables
ENV MODSEC_RULE_ENGINE=On \
    MODSEC_AUDIT_ENGINE=RelevantOnly \
    MODSEC_AUDIT_LOG_FORMAT=JSON \
    PARANOIA=2 \
    ANOMALY_INBOUND=5 \
    ANOMALY_OUTBOUND=4 \
    BLOCKING_PARANOIA=2

# Ad-tech specific: Higher body limits for OpenRTB bid requests (2MB)
ENV MAX_FILE_SIZE=2097152 \
    COMBINED_FILE_SIZES=2097152 \
    REQ_BODY_LIMIT=2097152 \
    REQ_BODY_NO_FILES_LIMIT=2097152 \
    RESP_BODY_LIMIT=524288

# Backend application
ENV BACKEND=http://catalyst:8000 \
    PORT=80 \
    PROXY_TIMEOUT=120 \
    PROXY_SSL_VERIFY=off

# CRS tuning for JSON-heavy traffic
ENV TX_MAX_NUM_ARGS=512 \
    TX_ARG_LENGTH=8000 \
    TX_ARG_NAME_LENGTH=400

EXPOSE 80 443

HEALTHCHECK --interval=30s --timeout=10s --start-period=30s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:80/health || exit 1
