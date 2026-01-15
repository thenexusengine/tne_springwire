#!/bin/bash
# Run integration tests with Docker services

set -e

echo "=== Starting test services ==="
docker-compose -f docker-compose.test.yml up -d

echo "=== Waiting for services to be ready ==="
sleep 5

# Wait for Redis
echo "Waiting for Redis..."
until docker-compose -f docker-compose.test.yml exec -T redis redis-cli ping 2>/dev/null | grep -q PONG; do
    sleep 1
done
echo "Redis is ready!"

# Wait for PostgreSQL
echo "Waiting for PostgreSQL..."
until docker-compose -f docker-compose.test.yml exec -T postgres pg_isready -U test 2>/dev/null; do
    sleep 1
done
echo "PostgreSQL is ready!"

echo ""
echo "=== Running integration tests ==="
echo ""

# Run PostgreSQL integration tests
echo "--- Running stored package integration tests ---"
go test -tags=integration -v ./internal/stored/... -coverprofile=coverage_stored_integration.out || true

# Run Redis/Publisher integration tests
echo ""
echo "--- Running endpoints integration tests ---"
go test -tags=integration -v ./internal/endpoints/... -coverprofile=coverage_endpoints_integration.out || true

echo ""
echo "=== Integration test complete ==="
echo ""
echo "To stop services:"
echo "  docker-compose -f docker-compose.test.yml down"
echo ""
echo "To view coverage:"
echo "  go tool cover -func=coverage_stored_integration.out"
echo "  go tool cover -func=coverage_endpoints_integration.out"
