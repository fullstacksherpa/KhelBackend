# -------------------------------
# Environment Configuration
# -------------------------------
MIGRATIONS_PATH = ./cmd/migrate/migrations
ENV ?= development

# Include environment file
ifeq ($(ENV),prod)
    include .env.prod
else ifeq ($(ENV),staging)
    include .env.staging
else
    include .env.development
endif

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

.PHONY: migrate-down
migrate-down:
	@echo "Rolling back migrations in $(ENV) environment (DB: $(DB_ADDR_NO_POOL))"
	@migrate -path=$(MIGRATIONS_PATH) -database="$(DB_ADDR_NO_POOL)" down $(filter-out $@,$(MAKECMDGOALS))

# -------------------------------
# Environment-Specific Shortcuts
# -------------------------------
.PHONY: prod-up
prod-up:
	@$(MAKE) migrate-up ENV=prod

.PHONY: prod-down
prod-down:
	@$(MAKE) migrate-down ENV=prod

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
# Other Commands
# -------------------------------
.PHONY: test
test:
	@go test -v ./...

.PHONY: seed
seed:
	@echo "Seeding $(ENV) database"
	@go run cmd/migrate/seed/main.go --env=$(ENV)

.PHONY: gen-docs
gen-docs:
	@swag init -g ./api/main.go -d cmd,internal && swag fmt

# -------------------------------
# Debug Helpers
# -------------------------------
.PHONY: show-env
show-env:
	@echo "Current Environment: $(ENV)"
	@echo "DB Connection: $(DB_ADDR_NO_POOL)"
	@echo "Migrations Path: $(MIGRATIONS_PATH)"

.PHONY: check-env-files
check-env-files:
	@echo "Checking environment files..."
	@echo "Development: $(if $(wildcard .env.development),exists,missing)"
	@echo "Staging: $(if $(wildcard .env.staging),exists,missing)" 
	@echo "Production: $(if $(wildcard .env.prod),exists,missing)"

# -------------------------------
# Help
# -------------------------------
.PHONY: help
help:
	@echo "Available commands:"
	@echo "  make migration <name>      Create new migration"
	@echo "  make migrate-up           Apply all pending migrations"
	@echo "  make migrate-down [N]     Rollback N migrations (default: all)"
	@echo ""
	@echo "Environment shortcuts:"
	@echo "  make dev-up/dev-down [N]  Development environment"
	@echo "  make staging-up/staging-down [N] Staging environment"
	@echo "  make prod-up/prod-down [N] Production environment"
	@echo ""
	@echo "Other commands:"
	@echo "  make test                 Run tests"
	@echo "  make seed                 Seed database"
	@echo "  make gen-docs             Generate API docs"
	@echo "  make show-env             Show current environment config"
	@echo "  make check-env-files      Check if environment files exist"