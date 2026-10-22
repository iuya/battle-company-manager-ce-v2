#!/bin/sh
# Runs once, when the development database volume is first created (compose.dev.yaml mounts it in /docker-entrypoint-initdb.d):
# makes the separate database the Go integration tests use, so they never touch the data of make run / demo. On an older volume,
# make test-db creates it.
set -e
psql -v ON_ERROR_STOP=1 -U "$POSTGRES_USER" -d postgres -c "CREATE DATABASE bcm_test"
