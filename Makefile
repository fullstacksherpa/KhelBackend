# -------------------------------
# Environment Configuration
# -------------------------------
MIGRATIONS_PATH = ./cmd/migrate/migrations
ENV ?= development  # Default to development if not specified

define load_env
$(info Loading $(1) environment...)
$(if $(filter $(1),prod staging development),,$(error Invalid ENV specified))
ifeq ($(1),prod)
include .env.prod
$(info Using production database configuration)
else ifeq ($(1),staging)
include .env.staging
$(info Using staging database configuration)
else
include .env.development
$(info Using development database configuration)
endif
endef

# Evaluate when ENV is finalized
$(eval $(call load_env,$(ENV)))

# -------------------------------
# Migration Targets
# -------------------------------
.PHONY: migration
migration:
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
