# Variables from .env (gitignored) are exported to the app; the defaults match compose.dev.yaml so a fresh clone just works.
-include .env
export
# Development database (throwaway credentials, published on localhost). Production is the default compose.yaml: make up / prod-user.
DC = docker compose -f compose.dev.yaml
DATABASE_URL ?= postgres://bcm:bcm@localhost:5432/bcm?sslmode=disable

CSS_IN  = ./tailwind.css
CSS_OUT = ./internal/web/static/app.css

.PHONY: e2e e2e-install e2e-clean test-db check-env-docs migrate up up-app prod-user prod-down backup restore restore-drill vendor-check dice3d user grant seed demo db-reset run dev css css-watch generate audit test test-integration db-up db-down tidy get-tailwind

# make user EMAIL=you@example.com NAME="Your Name"   (asks for the password)
user: db-up
	go run ./cmd/server user create --email "$(EMAIL)" --name "$(NAME)"

# make grant EMAIL=you@example.com ROLE=superuser   (ROLE: user|admin|superuser; add CREATE=1 to let a user create campaigns)
grant: db-up
	go run ./cmd/server user grant --email "$(EMAIL)" --role "$(or $(ROLE),user)" $(if $(CREATE),--create-campaigns)

# make seed [PASSWORD=...]   demo accounts, campaign, companies and matches for local development (see docs/architecture.md)
seed: db-up
	go run ./cmd/server seed $(if $(PASSWORD),--password "$(PASSWORD)")

# make demo [PASSWORD=...]   DESTRUCTIVE: empties the development database (every account, campaign and match in it), loads the rich demo world (many campaigns in every state, see docs/demo-world.md)
# and starts the server on :8080. The accounts, invite links and gaps it printed are kept in tmp/demo-world.txt.
demo: css
	@echo "make demo EMPTIES the development database (all accounts, campaigns and matches in it)."
	$(MAKE) db-reset
	@mkdir -p tmp
	@go run ./cmd/server seed --world $(if $(PASSWORD),--password "$(PASSWORD)") > tmp/demo-world.txt 2>&1 || { cat tmp/demo-world.txt; exit 1; }
	@cat tmp/demo-world.txt
	APP_ENV=development CSP_MODE=report STYLEGUIDE=true go run ./cmd/server

# make migrate   applies pending migrations and exits (what a deploy runs when the server has MIGRATE_ON_START=false)
migrate: db-up
	go run ./cmd/server migrate

# make check-env-docs   fails when the server reads an environment variable the compose files and .env.example do not document
check-env-docs:
	go test ./cmd/server -run TestEveryEnvVarIsDocumented -count=1

# make up   production: the app image next to PostgreSQL, from compose.yaml and .env (cp .env.example .env first; docs/operations.md, "First start")
up:
	docker compose up -d --build --wait
up-app: up

# make prod-user EMAIL=you@example.com NAME="You" [ROLE=superuser]   the first account in the production container (asks for the password; ROLE grants it)
prod-user:
	docker compose run --rm app user create --email "$(EMAIL)" --name "$(NAME)"
	$(if $(ROLE),docker compose run --rm app user grant --email "$(EMAIL)" --role "$(ROLE)" --create-campaigns)

prod-down:
	docker compose down

# Drops the development database (all data) and starts an empty one; seed it again afterwards.
db-reset:
	$(DC) down -v
	$(MAKE) db-up

run: css db-up
	APP_ENV=development CSP_MODE=report STYLEGUIDE=true go run ./cmd/server

# Rebuilds Go on change (air) and Tailwind in watch mode.
dev: css db-up
	./tailwindcss build -i $(CSS_IN) -o $(CSS_OUT) --watch & APP_ENV=development CSP_MODE=report STYLEGUIDE=true go tool air

css:
	./tailwindcss build -i $(CSS_IN) -o $(CSS_OUT) --minified

# make backup [KEEP=14]   pg_dump of the production compose database into backups/ (COMPOSE_FILE=compose.dev.yaml for the development one; docs/operations.md)
backup:
	KEEP=$(or $(KEEP),14) scripts/backup.sh

# make restore FILE=backups/bcm-....dump CONFIRM=yes   replaces the database's contents; stop the app first
restore:
	CONFIRM=$(CONFIRM) scripts/restore.sh "$(FILE)"

# make restore-drill [FILE=...]   restores the newest (or the given) dump into a scratch database, smoke-checks it, drops it
restore-drill:
	scripts/restore-drill.sh $(FILE)

# make vendor-check [ONLINE=1]   the vendored scripts match internal/web/vendor.json (ONLINE=1 also compares with the upstream files); docs/vendored-js.md
vendor-check:
	$(if $(ONLINE),VENDOR_ONLINE=1) go test ./internal/web -run TestVendoredJSMatchesManifest -count=1

# Rebuilds static/js/dice3d.js (three.js and cannon-es cut down to what the 3D dice use); needs node and npm.
dice3d:
	cd tools/dice3d && npm install --no-audit --no-fund && node build.mjs

# make audit [OUT=tmp/audit]   one markdown report per Battle Company plus an index with anomaly flags (docs/definitions.md)
audit:
	go run ./cmd/server audit $(if $(OUT),--out "$(OUT)")

generate:
	go generate ./...

test:
	go test ./...

# Runs everything, including the postgres integration tests, against the bcm_test database of the compose PostgreSQL: the tests empty it, the development database (bcm) is never touched.
TEST_DATABASE_URL ?= postgres://bcm:bcm@localhost:5432/bcm_test?sslmode=disable

# make test-db   makes sure the bcm_test database exists next to the development one (a volume made before it was added has none)
test-db: db-up
	@$(DC) exec -T postgres psql -U bcm -d postgres -tAc "SELECT 1 FROM pg_database WHERE datname = 'bcm_test'" | grep -q 1 || $(DC) exec -T postgres psql -U bcm -d postgres -c "CREATE DATABASE bcm_test"

test-integration: test-db
	TEST_DATABASE_URL='$(TEST_DATABASE_URL)' go test -p 1 -count=1 ./...

# make e2e-install   installs the browser test dependencies (puppeteer-core, pinned by tests/e2e/package-lock.json); the tests use your Chrome or Chromium (CHROME_PATH)
e2e-install:
	cd tests/e2e && npm ci --no-audit --no-fund

# make e2e [ONLY=match]   browser smoke tests (docs/testing.md): own scratch database from a seeded template, own port, headless Chrome; failures leave screenshots in tests/e2e/out
e2e: db-up css
	@test -d tests/e2e/node_modules || $(MAKE) e2e-install
	cd tests/e2e && node run.mjs $(if $(ONLY),--only $(ONLY))

# make e2e-clean   drops the cached seeded template databases (bcm_e2e_tpl_*) and the test outputs
e2e-clean: db-up
	@for d in $$($(DC) exec -T postgres psql -U bcm -d postgres -tAc "SELECT datname FROM pg_database WHERE datname LIKE 'bcm_e2e_%'"); do $(DC) exec -T postgres psql -U bcm -d postgres -c "DROP DATABASE $$d WITH (FORCE)"; done
	rm -rf tests/e2e/out

db-up:
	$(DC) up -d --wait postgres

db-down:
	$(DC) down

tidy:
	go mod tidy

get-tailwind:
	curl -sL https://github.com/tailwindlabs/tailwindcss/releases/latest/download/tailwindcss-macos-arm64 -o tailwindcss
	chmod +x tailwindcss
