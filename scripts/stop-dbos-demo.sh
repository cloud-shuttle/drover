#!/bin/bash
# Script to stop PostgreSQL for the DBOS demo

set -e

# Colors for output
GREEN='\033[0;32m'
NC='\033[0m' # No Color

echo "🛑 Stopping Drover DBOS Demo PostgreSQL..."

# Function to get docker compose command
get_docker_compose_cmd() {
    if docker compose version &> /dev/null; then
        echo "docker compose"
    else
        echo "docker-compose"
    fi
}

DOCKER_COMPOSE=$(get_docker_compose_cmd)

# Stop PostgreSQL
$DOCKER_COMPOSE -f docker-compose.yml down

echo -e "${GREEN}✓ PostgreSQL stopped${NC}"
echo ""
echo "💡 Tip: To remove the data volume, run:"
echo "   docker volume rm drover_postgres_data"
