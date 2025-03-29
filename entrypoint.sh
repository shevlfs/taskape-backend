#!/bin/sh
set -e

# Wait for the database to be ready
echo "Waiting for database connection..."
RETRIES=5
until PGPASSWORD=$DB_PASSWORD psql -h "$DB_HOST" -U "$DB_USER" -p "$DB_PORT" -c '\q' > /dev/null 2>&1 || [ $RETRIES -eq 0 ]; do
  echo "Waiting for database connection, $((RETRIES--)) remaining attempts..."
  sleep 3
done

# Check if database exists
if PGPASSWORD=$DB_PASSWORD psql -h "$DB_HOST" -U "$DB_USER" -p "$DB_PORT" -lqt | cut -d \| -f 1 | grep -qw $DB_NAME; then
  echo "Database $DB_NAME already exists"
else
  echo "Database $DB_NAME does not exist, creating it..."
  PGPASSWORD=$DB_PASSWORD psql -h "$DB_HOST" -U "$DB_USER" -p "$DB_PORT" -c "CREATE DATABASE $DB_NAME;"
  echo "Database created, initializing schema..."
  PGPASSWORD=$DB_PASSWORD psql -h "$DB_HOST" -U "$DB_USER" -p "$DB_PORT" -d "$DB_NAME" -f /app/schema.sql
  echo "Schema initialization complete"
fi

# Start the application
echo "Starting taskape backend..."
exec "$@"