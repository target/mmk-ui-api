#!/bin/bash

# Verify development data seeding script
# This script connects to the dev database and checks if the seeded data exists

set -e

# Database connection parameters (from docker-compose dev profile)
DB_HOST=${DB_HOST:-localhost}
DB_PORT=${DB_PORT:-5432}
DB_USER=${DB_USER:-merrymaker}
DB_PASSWORD=${DB_PASSWORD:-merrymaker}
DB_NAME=${DB_NAME:-merrymaker}

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo "Verifying development data in database..."
echo "   Host: $DB_HOST:$DB_PORT"
echo "   Database: $DB_NAME"
echo ""

# Function to run SQL query and get count (robust to errors)
run_count_query() {
	local table=$1
	local count
	count=$(PGPASSWORD="$DB_PASSWORD" psql -h "$DB_HOST" -p "$DB_PORT" -U "$DB_USER" -d "$DB_NAME" -At -c "SELECT COUNT(*) FROM $table;" 2>/dev/null | tr -d '[:space:]')
	if ! [[ "$count" =~ ^[0-9]+$ ]]; then count=0; fi
	echo "$count"
}

# Function to check table data
check_table() {
	local table=$1
	local description=$2
	local expected_min=$3

	local count=$(run_count_query "$table")

	if [ "$count" -ge "$expected_min" ]; then
		echo -e "[OK] ${GREEN}$description${NC}: $count entries (expected >=$expected_min)"
		return 0
	else
		echo -e "[FAIL] ${RED}$description${NC}: $count entries (expected >=$expected_min)"
		return 1
	fi
}

# Function to show sample data
show_sample_data() {
	local table=$1
	local description=$2
	local columns=$3

	echo ""
	echo -e "${YELLOW}Sample $description:${NC}"
	PGPASSWORD=$DB_PASSWORD psql -h $DB_HOST -p $DB_PORT -U $DB_USER -d $DB_NAME -c "SELECT $columns FROM $table LIMIT 3;" 2>/dev/null || echo "   (No data or connection error)"
}

# Check if database is accessible
if ! PGPASSWORD=$DB_PASSWORD psql -h $DB_HOST -p $DB_PORT -U $DB_USER -d $DB_NAME -c "SELECT 1;" >/dev/null 2>&1; then
	echo -e "[ERROR] ${RED}Cannot connect to database${NC}"
	echo "   Make sure the dev database is running: make dev-db-up"
	exit 1
fi

echo "[OK] Database connection successful"
echo ""

# Check each table
all_good=true

if ! check_table "secrets" "Secrets" 4; then
	all_good=false
fi

if ! check_table "sources" "Sources" 3; then
	all_good=false
fi

if ! check_table "http_alert_sinks" "HTTP Alert Sinks" 3; then
	all_good=false
fi

if ! check_table "sites" "Sites" 3; then
	all_good=false
fi

# Show sample data if everything looks good
if [ "$all_good" = true ]; then
	show_sample_data "secrets" "Secrets" "name, type"
	show_sample_data "sources" "Sources" "name, test"
	show_sample_data "http_alert_sinks" "HTTP Alert Sinks" "name, method, uri"
	show_sample_data "sites" "Sites" "name, enabled, run_every_minutes"

	echo ""
	echo -e "[SUCCESS] ${GREEN}All development data verified successfully!${NC}"
	echo ""
	echo "You can now:"
	echo "  - Start the application: go run ./cmd/merrymaker"
	echo "  - Visit the UI: http://localhost:8080"
	echo "  - Test dry runs with the seeded sites"
else
	echo ""
	echo -e "[WARNING] ${YELLOW}Some data is missing. Run the seeding script:${NC}"
	echo "     make dev-db-seed"
	exit 1
fi
