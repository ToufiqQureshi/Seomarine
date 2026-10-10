# Goose Schema Baseline Activation Guide

## Overview
This document describes the validation and activation steps for the staged Goose version 1 PostgreSQL schema baseline (`backend/internal/database/baseline/00001_drizzle_baseline.sql`).

## Baseline Migration File
- Location: `backend/internal/database/baseline/00001_drizzle_baseline.sql`
- Contains: Full Drizzle DDL schema baseline including all legacy TypeScript tables, Go tables (`go_*`), indexes, and constraints.

## Activation Instructions

### Prerequisites
1. **TypeScript Writes Disabled**: Verify that all legacy TypeScript server functions and Drizzle ORM mutations have been completely disabled/deprecated or removed.
2. **Backup Database**: Take a full PostgreSQL snapshot before executing schema baseline modifications.

### Step 1: Copy Snapshot to Migrations Directory
When ready to activate:
```bash
cp backend/internal/database/baseline/00001_drizzle_baseline.sql backend/internal/database/migrations/00001_drizzle_baseline.sql
```

### Step 2: Validate Migration with Goose CLI
Run goose against a clean test PostgreSQL database:
```bash
goose -dir backend/internal/database/migrations postgres "$TEST_DATABASE_URL" up
```

### Step 3: Run Verification Test
Run the Go database package integration tests:
```bash
TEST_DATABASE_URL="$TEST_DATABASE_URL" go test -v ./backend/internal/database/...
```

## Rollback / Reset Procedure
If goose migration fails during staging:
```bash
goose -dir backend/internal/database/migrations postgres "$TEST_DATABASE_URL" down
```
