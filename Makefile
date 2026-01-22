# -------------------------------
# Environment Configuration
# -------------------------------
MIGRATIONS_PATH = ./cmd/migrate/migrations
ENV ?= development

# Production rollback safety:
# Example: make prod-down 1 CONFIRM=yes
CONFIRM ?= no

# Include environment file
ifeq ($(ENV),prod)
    include .env.prod
else ifeq ($(ENV),staging)
    include .env.staging
else
    include .env.development
endif

# -------------------------------
# Helpers
# -------------------------------
# If user runs: make dev-down 1
# MAKECMDGOALS is: "dev-down 1"
# First goal is the target name; second is the step count.
DOWN_STEPS := $(word 2,$(MAKECMDGOALS))

# swallow extra args like "1" so make doesn't treat them as targets
%:
	@:

# -------------------------------
# Migration Targets
# -------------------------------
.PHONY: migration
migration:
	@echo "Creating migration in $(ENV) environment..."
	@migrate create -seq -ext sql -dir $(MIGRATIONS_PATH) $(filter-out $@,$(MAKECMDGOALS))

.PHONY: migrate-up
migrate-up:
	@echo "Applying migrations to $(ENV) environment (DB: $(DB_ADDR_NO_POOL))"
	@migrate -path=$(MIGRATIONS_PATH) -database="$(DB_ADDR_NO_POOL)" up

# SAFE DOWN:
# - dev/staging default to 1 step when no arg is provided
# - prod requires explicit step count AND CONFIRM=yes
.PHONY: migrate-down
migrate-down:
	@echo "Rolling back migrations in $(ENV) environment (DB: $(DB_ADDR_NO_POOL))"
	@steps="$(DOWN_STEPS)"; \
	if [ -z "$$steps" ]; then \
		if [ "$(ENV)" = "prod" ]; then \
			echo "ERROR: Refusing to run prod down without an explicit step count."; \
			echo "       Use: make prod-down 1 CONFIRM=yes"; \
			exit 1; \
		else \
			steps="1"; \
			echo "No step count provided; defaulting to 1 for $(ENV)."; \
		fi; \
	fi; \
	if [ "$(ENV)" = "prod" ] && [ "$(CONFIRM)" != "yes" ]; then \
		echo "ERROR: Production rollback blocked."; \
		echo "       Re-run with: make prod-down $$steps CONFIRM=yes"; \
		exit 1; \
	fi; \
	if [ "$(ENV)" = "prod" ]; then \
		echo "WARNING: PRODUCTION rollback approved (steps=$$steps)."; \
	fi; \
	migrate -path=$(MIGRATIONS_PATH) -database="$(DB_ADDR_NO_POOL)" down $$steps

# -------------------------------
# Environment Shortcuts
# -------------------------------
.PHONY: prod-up
prod-up:
	@$(MAKE) migrate-up ENV=prod

.PHONY: prod-down
prod-down:
	@$(MAKE) migrate-down ENV=prod CONFIRM=$(CONFIRM)

.PHONY: staging-up
staging-up:
	@$(MAKE) migrate-up ENV=staging

.PHONY: staging-down
staging-down:
	@$(MAKE) migrate-down ENV=staging

.PHONY: dev-up
dev-up:
	@$(MAKE) migrate-up ENV=development

.PHONY: dev-down
dev-down:
	@$(MAKE) migrate-down ENV=development

# -------------------------------
# Help
# -------------------------------
.PHONY: help
help:
	@echo "Migrations:"
	@echo "  make migration <name>                Create new migration"
	@echo "  make dev-up                          Apply all pending migrations (dev)"
	@echo "  make staging-up                      Apply all pending migrations (staging)"
	@echo "  make prod-up                         Apply all pending migrations (prod)"
	@echo ""
	@echo "Rollback (SAFE):"
	@echo "  make dev-down [N]                    Rollback N migrations (default: 1)"
	@echo "  make staging-down [N]                Rollback N migrations (default: 1)"
	@echo "  make prod-down N CONFIRM=yes         Rollback N migrations (required + confirm)"
	@echo ""
	@echo "Examples:"
	@echo "  make dev-down                        -> rolls back 1 migration"
	@echo "  make dev-down 2                      -> rolls back 2 migrations"
	@echo "  make prod-down 1 CONFIRM=yes         -> rolls back 1 migration in PROD"
