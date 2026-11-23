#!/bin/bash

# An explicit DATABASE_URL wins over the local .env file.
if [ -z "${DATABASE_URL:-}" ] && [ -f .env ]; then
    source .env
fi

cd sql/schema
goose turso $DATABASE_URL up
