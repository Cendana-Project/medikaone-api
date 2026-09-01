GO ?= go
APP_NAME ?= medikaone
CONFIRM ?=
GOVULNCHECK_VERSION ?= v1.7.0

.PHONY: run build clean fmt vet test test-race check vuln \
	seed migrate-up migrate-create migrate-status \
	staging-db-fingerprint staging-reset-all staging-reset-seed

run:
	$(GO) run . server

build:
	$(GO) build -o $(APP_NAME) .

clean:
	$(RM) $(APP_NAME) $(APP_NAME).exe

fmt:
	$(GO) fmt ./...

vet:
	$(GO) vet ./...

test:
	$(GO) test ./...

test-race:
	$(GO) test -race ./...

check: vet test

vuln:
	$(GO) run golang.org/x/vuln/cmd/govulncheck@$(GOVULNCHECK_VERSION) ./...

# Idempotent seed for local development. Staging resets must use one of the
# guarded targets below so an explicit environment and confirmation are checked.
seed:
	$(GO) run . seed

migrate-up:
	$(GO) run . migrate --action up

# Usage: make migrate-create name=add_example_table
migrate-create:
	$(GO) run . migrate --action create --name "$(name)"

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
