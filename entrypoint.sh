#!/bin/sh
set -e

# Wait for the database to be ready - connect directly to the taskape database
echo "Waiting for database connection..."
RETRIES=5

until PGPASSWORD=$DB_PASSWORD psql -h "$DB_HOST" -U "$DB_USER" -p "$DB_PORT" -d "$DB_NAME" -c '\q' > /dev/null 2>&1 || [ $RETRIES -eq 0 ]; do
  echo "Waiting for database connection, $((RETRIES--)) remaining attempts..."
  sleep 3
done

if [ $RETRIES -eq 0 ]; then
  echo "Could not connect to database. Please check your credentials and that the database '$DB_NAME' exists."
  exit 1
fi

# Check if the tables exist (checking for users table as indicator)
if PGPASSWORD=$DB_PASSWORD psql -h "$DB_HOST" -U "$DB_USER" -p "$DB_PORT" -d "$DB_NAME" -c "SELECT 1 FROM information_schema.tables WHERE table_name = 'users'" | grep -q 1; then
  echo "Database schema already exists"
else
  echo "Tables don't exist yet, initializing schema..."
  PGPASSWORD=$DB_PASSWORD psql -h "$DB_HOST" -U "$DB_USER" -p "$DB_PORT" -d "$DB_NAME" -f /app/schema.sql
  echo "Schema initialization complete"
fi

# Start the application
echo "Starting taskape backend..."
exec "$@"