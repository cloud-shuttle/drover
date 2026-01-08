#!/bin/bash
# Script to start PostgreSQL and run the DBOS demo

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo "🐂 Drover DBOS Demo Setup"
echo "=========================="
echo ""

# Check if docker is available
if ! command -v docker &> /dev/null; then
    echo -e "${RED}Error: docker is not installed${NC}"
    echo "Please install Docker to run the DBOS demo"
    exit 1
fi

# Check if docker-compose is available
if ! command -v docker-compose &> /dev/null && ! docker compose version &> /dev/null; then
    echo -e "${RED}Error: docker-compose is not installed${NC}"
    echo "Please install Docker Compose to run the DBOS demo"
    exit 1
fi

# Function to get docker compose command
get_docker_compose_cmd() {
    if docker compose version &> /dev/null; then
        echo "docker compose"
    else
        echo "docker-compose"
    fi
}

DOCKER_COMPOSE=$(get_docker_compose_cmd)

# Start PostgreSQL
echo "🚀 Starting PostgreSQL..."
$DOCKER_COMPOSE -f docker-compose.yml up -d

# Wait for PostgreSQL to be ready
echo ""
echo "⏳ Waiting for PostgreSQL to be ready..."
MAX_ATTEMPTS=30
ATTEMPT=0

while [ $ATTEMPT -lt $MAX_ATTEMPTS ]; do
    if docker exec drover-postgres pg_isready -U postgres &> /dev/null; then
        echo -e "${GREEN}✓ PostgreSQL is ready!${NC}"
        break
    fi
    ATTEMPT=$((ATTEMPT + 1))
    echo -n "."
    sleep 1
done

if [ $ATTEMPT -eq $MAX_ATTEMPTS ]; then
    echo -e "\n${RED}Error: PostgreSQL failed to start${NC}"
    $DOCKER_COMPOSE -f docker-compose.yml down
    exit 1
fi

echo ""
echo "📊 PostgreSQL Status:"
docker exec drover-postgres psql -U postgres -d drover -c "SELECT version();" | grep -v "row"

# Set environment variable
export DBOS_SYSTEM_DATABASE_URL="postgres://postgres:postgres@localhost:5432/drover?sslmode=disable"

echo ""
echo "🎯 Environment variable set:"
echo "   export DBOS_SYSTEM_DATABASE_URL=\"$DBOS_SYSTEM_DATABASE_URL\""

echo ""
echo "✨ Setup complete! You can now run:"
echo ""
echo "   # Run demo (sequential mode)"
echo "   ./drover dbos-demo"
echo ""
echo "   # Run demo (parallel queue mode)"
echo "   ./drover dbos-demo --queue"
echo ""
echo "   # To stop PostgreSQL:"
echo "   $DOCKER_COMPOSE -f docker-compose.yml down"
echo ""
