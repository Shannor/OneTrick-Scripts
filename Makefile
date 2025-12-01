.PHONY: help deploy-server-tick deploy-migration execute-server-tick execute-migration

# ====================================================================================
# Variables
# ====================================================================================

PROJECT_ID   := gruntt-destiny
REGION       := us-central1

# Server Tick specific variables
SERVER_TICK_NAME         := server-tick
SERVER_TICK_SOURCE       := ./server-tick
SERVER_TICK_TASKS        := 1
SERVER_TICK_TIMEOUT      := 5m
SERVER_TICK_MEMORY       := 1Gi
SERVER_TICK_CPU          := 1
SERVER_TICK_RETRIES      := 2
SERVER_TICK_SECRETS      := D2_API_KEY=D2_API_KEY:latest

# Migration specific variables (assuming similar configuration)
MIGRATION_NAME           := migration
MIGRATION_SOURCE         := ./migration
MIGRATION_TASKS          := 1
MIGRATION_TIMEOUT        := 10m
MIGRATION_MEMORY         := 1Gi
MIGRATION_CPU            := 1
MIGRATION_RETRIES        := 2

# ====================================================================================
# Targets
# ====================================================================================

## Deploys the server-tick Cloud Run job.
deploy-server-tick:
	gcloud run jobs deploy $(SERVER_TICK_NAME) \
		--source $(SERVER_TICK_SOURCE) \
		--tasks $(SERVER_TICK_TASKS) \
		--task-timeout $(SERVER_TICK_TIMEOUT) \
		--memory $(SERVER_TICK_MEMORY) \
		--cpu $(SERVER_TICK_CPU) \
		--max-retries $(SERVER_TICK_RETRIES) \
		--set-secrets $(SERVER_TICK_SECRETS) \
		--region $(REGION) \
		--project=$(PROJECT_ID)

## Deploys the migration Cloud Run job.
deploy-migration:
	gcloud run jobs deploy $(MIGRATION_NAME) \
		--source $(MIGRATION_SOURCE) \
		--tasks $(MIGRATION_TASKS) \
		--task-timeout $(MIGRATION_TIMEOUT) \
		--memory $(MIGRATION_MEMORY) \
		--cpu $(MIGRATION_CPU) \
		--max-retries $(MIGRATION_RETRIES) \
		--region $(REGION) \
		--project=$(PROJECT_ID)