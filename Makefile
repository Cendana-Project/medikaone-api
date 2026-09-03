GO ?= go
APP_NAME ?= medikaone
CONFIRM ?=
VERSION ?=
NAME ?=
name ?=
GOVULNCHECK_VERSION ?= v1.7.0
MIGRATION_NAME = $(strip $(if $(NAME),$(NAME),$(name)))

.DEFAULT_GOAL := help
.NOTPARALLEL: setup dev

ifeq ($(OS),Windows_NT)
APP_BIN ?= $(APP_NAME).exe
RUN_BIN = .\$(APP_BIN)
else
APP_BIN ?= $(APP_NAME)
RUN_BIN = ./$(APP_BIN)
endif

.PHONY: help run server dev setup build start render-build render-start clean \
	fmt vet test test-cover test-race check vuln seed migrate migrate-up \
	migrate-up-one migrate-up-to migrate-create migrate-status \
	staging-db-fingerprint staging-reset-all staging-reset-seed

help:
	@echo "MedikaOne development commands"
	@echo "  make server                 Run API with go run"
	@echo "  make dev                    Migrate, seed, then run API (local development)"
	@echo "  make setup                  Migrate and seed local database"
	@echo "  make build                  Build application binary"
	@echo "  make start                  Build and run application binary"
	@echo "  make migrate               Alias for migrate-up"
	@echo "  make migrate-up            Apply all pending migrations"
	@echo "  make migrate-up-one        Apply one pending migration"
	@echo "  make migrate-up-to VERSION=<version>"
	@echo "  make migrate-status        Show migration status"
	@echo "  make migrate-create NAME=<migration_name>"
	@echo "  make seed                   Idempotently seed development data"
	@echo "  make check                  Run go vet and unit tests"
	@echo "  make test-cover             Run tests with coverage"
	@echo "  make test-race              Run tests with race detector"
	@echo "  make vuln                   Scan reachable vulnerabilities"
	@echo "  make staging-db-fingerprint"
	@echo "  make staging-reset-all CONFIRM=RESET-ALL-STAGING-DATA"
	@echo "  make staging-reset-seed CONFIRM=RESET-DEMO-STAGING-DATA"

server:
	$(GO) run . server

run: server

# Local convenience only. Production/staging web processes must run migration
# separately so the server never receives owner/DDL database credentials.
setup: migrate-up seed

dev: setup server

build:
	$(GO) build -o $(APP_BIN) .

start: build
	$(RUN_BIN) server

# Commands matching the current Render service configuration.
render-build:
	$(GO) build -o main .

render-start:
	./main server

clean:
	$(RM) $(APP_NAME) $(APP_NAME).exe main main.exe

fmt:
	$(GO) fmt ./...

vet:
	$(GO) vet ./...

test:
	$(GO) test ./...

test-cover:
	$(GO) test -cover ./...

test-race:
	$(GO) test -race -cover ./...

check: vet test

vuln:
	$(GO) run golang.org/x/vuln/cmd/govulncheck@$(GOVULNCHECK_VERSION) ./...

# Idempotent seed for local development. Staging resets must use one of the
# guarded targets below so an explicit environment and confirmation are checked.
seed:
	$(GO) run . seed

migrate: migrate-up

migrate-up:
	$(GO) run . migrate --action up

migrate-up-one:
	$(GO) run . migrate --action up-by-one

migrate-up-to:
	$(if $(strip $(VERSION)),,$(error VERSION is required; example: make migrate-up-to VERSION=20260903090000))
	$(GO) run . migrate --action up-to --version "$(VERSION)"

# Both NAME=... and the legacy name=... form are accepted.
migrate-create:
	$(if $(MIGRATION_NAME),,$(error NAME is required; example: make migrate-create NAME=add_example_table))
	$(GO) run . migrate --action create --name "$(MIGRATION_NAME)"

migrate-status:
	$(GO) run . migrate --action status

# Store this value as STAGING_DATABASE_FINGERPRINT on the dedicated reset
# job/operator environment, never on the staging web process.
staging-db-fingerprint:
	$(GO) run . database-fingerprint

# Destructive: clears all known application data, applies Up migrations, then seeds.
# Usage: make staging-reset-all CONFIRM=RESET-ALL-STAGING-DATA
staging-reset-all:
	$(if $(filter command line,$(origin CONFIRM)),,$(error CONFIRM must be supplied explicitly on this make command line))
	$(GO) run . staging-reset-all --confirm="$(CONFIRM)"

# Recreates known demo users and restores seeded hospital/RBAC definitions.
# Real users, their role assignments, memberships, and hospitals are preserved.
# Usage: make staging-reset-seed CONFIRM=RESET-DEMO-STAGING-DATA
staging-reset-seed:
	$(if $(filter command line,$(origin CONFIRM)),,$(error CONFIRM must be supplied explicitly on this make command line))
	$(GO) run . staging-reset-seed --confirm="$(CONFIRM)"
