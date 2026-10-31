# Variables from .env (gitignored) are exported to the app; the defaults match compose.dev.yaml so a fresh clone just works.
-include .env
export
# A GOROOT inherited from a long-running shell or IDE can name another Go version than go.mod asks for ("go1.26.2 does not match go1.27.3"); the go command finds its own root.
unexport GOROOT
# Development database (throwaway credentials, published on localhost). Production is the default compose.yaml: make up / prod-user.
DC = docker compose -f compose.dev.yaml
DATABASE_URL ?= postgres://bcm:bcm@localhost:5432/bcm?sslmode=disable
# make run, dev and demo listen on loopback only (no firewall prompt for each new build); make run LISTEN_HOST= opens them to the network,
# for a phone on the same Wi-Fi.
LISTEN_HOST ?= 127.0.0.1

CSS_IN  = ./tailwind.css
CSS_OUT = ./internal/web/static/app.css

.PHONY: hooks hooks-test validate concept-graph e2e e2e-groups e2e-check-groups e2e-enforce e2e-install e2e-clean perf test-db check-env-docs docs-check migrate up up-app prod-user prod-down test-backup-restore backuprestore restore-drill vendor-check dice3d user grant seed demo fuzz check-ruleset ruleset-reference db-reset run dev css css-watch generate audit test test-integration db-up db-down tidy get-tailwind

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

# make test-backup-restore   backup, restore and restore drill against a scratch PostgreSQL laid out like a server (Docker + Go; docs/operations.md)
test-backup-restore:
	scripts/test-backup-restore.sh

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

generate: concept-graph
	go generate ./...

# make validate   checks the four rules files: JSON Schemas, the Go validator on the Battle Companies file (cmd/rulecheck), the curator's own
# checks (tools/lean_checks.py), the glossary and the concept graph (tools/README.md). Needs no database; jsonschema is optional.
# make hooks   turns on the git hooks of .githooks/ for this clone: Conventional Commits subject, gofmt, a secret scan and no push to main.
# They run the beads hooks (.beads/hooks) first, so beads keeps working. make hooks-test tests them in a throwaway repository.
hooks:
	git config core.hooksPath .githooks
	@echo "git hooks: on (.githooks). Undo with: git config --unset core.hooksPath"

hooks-test:
	sh tests/ci/githooks.test.sh

validate:
	python3 tools/validate_definitions.py

# make concept-graph   regenerates definitions/concepts/concept_graph.json from the three rules files (run it after rebuilding
# any of them: the graph records their hashes) and checks the definitions (docs/definitions.md)
concept-graph:
	python3 tools/build_concept_graph.py
	python3 tools/validate_definitions.py

test: e2e-check-groups docs-check
	go test ./...
	cd tools && python3 -m unittest test_extract_tts_models test_to_wound test_lean_checks test_concept_graph test_ruleset_reference

# Runs everything, including the postgres integration tests, against the bcm_test database of the compose PostgreSQL: the tests empty it, the development database (bcm) is never touched.
TEST_DATABASE_URL ?= postgres://bcm:bcm@localhost:5432/bcm_test?sslmode=disable

# make test-db   makes sure the bcm_test database exists next to the development one (a volume made before it was added has none)
test-db: db-up
	@$(DC) exec -T postgres psql -U bcm -d postgres -tAc "SELECT 1 FROM pg_database WHERE datname = 'bcm_test'" | grep -q 1 || $(DC) exec -T postgres psql -U bcm -d postgres -c "CREATE DATABASE bcm_test"

# make check-ruleset FILE=battle_companies.json   checks a lean (schemaVersion 2) rules file like the upload does and prints every problem (exit 1 on errors; docs/definitions.md)
check-ruleset:
	@test -n "$(FILE)" || { echo "usage: make check-ruleset FILE=battle_companies.json"; exit 2; }
	go run ./cmd/rulecheck $(if $(VANILLA),-vanilla "$(VANILLA)") $(if $(GLOSSARY),-glossary "$(GLOSSARY)") "$(FILE)"

# make ruleset-reference   rewrites docs/customizing-rulesets-reference.md from battle_companies.schema.json (make test fails when it is stale; the schema itself: go test ./internal/core/lean -run TestSchemaUpToDate -update-schema)
ruleset-reference:
	python3 tools/gen_ruleset_reference.py

# make fuzz [FUZZTIME=1m]   explores the lean decoder and the dice parser with go's fuzzer (the seed corpus already runs in make test; docs/testing.md)
fuzz:
	go test ./internal/core/lean -run '^$$' -fuzz FuzzParseDice -fuzztime 20s
	go test ./internal/core/lean -run '^$$' -fuzz FuzzDecode -fuzztime $(or $(FUZZTIME),1m)

test-integration: test-db
	TEST_DATABASE_URL='$(TEST_DATABASE_URL)' go test -p 1 -count=1 ./...

# make e2e-install   installs the browser test dependencies (puppeteer-core, pinned by tests/e2e/package-lock.json); the tests use your Chrome or Chromium (CHROME_PATH)
e2e-install:
	cd tests/e2e && npm ci --no-audit --no-fund

# make e2e [ONLY=match] [GROUP=name]   browser smoke tests (docs/testing.md): own scratch database from a seeded template, own port, headless Chrome; failures leave screenshots in tests/e2e/out
e2e: db-up css
	@test -d tests/e2e/node_modules || $(MAKE) e2e-install
	cd tests/e2e && node run.mjs $(if $(ONLY),--only $(ONLY)) $(if $(GROUP),--group $(GROUP))

# make e2e-enforce [ONLY=match]   the same suite with the server in CSP_MODE=enforce (E2E_CSP_MODE=enforce): every page, swap, stream and the 3D dice run under the
# blocking Content-Security-Policy, and a violation (browser event or /csp-report line) fails the step (docs/operations.md, "Content-Security-Policy"). CI runs this one.
e2e-enforce:
	$(MAKE) e2e E2E_CSP_MODE=enforce

# make perf [LABEL=name] [REPS=3] [LATENCIES=0,100,300] [ONLY=match]   frontend performance baseline (docs/performance.md): the main flows in headless Chrome with 0/100/300 ms
# of emulated latency on a scratch copy of the demo world; prints a table and writes tests/e2e/out/perf-<LABEL>.json
perf: db-up css
	@test -d tests/e2e/node_modules || $(MAKE) e2e-install
	cd tests/e2e && node perf.mjs --label $(or $(LABEL),run) $(if $(REPS),--reps $(REPS)) $(if $(LATENCIES),--latencies $(LATENCIES)) $(if $(ONLY),--only "$(ONLY)")

# make e2e-groups   the suite one group of tests/e2e/groups.json after the other (what CI runs as parallel jobs; each group starts from a fresh copy of the seeded world);
# E2E_CSP_MODE=enforce make e2e-groups for the blocking policy. make e2e-check-groups only checks that every flow file is in exactly one group (part of make test).
e2e-groups:
	@for g in $$(node tests/e2e/lib/groups.mjs --names | tr -d '[]"' | tr ',' ' '); do $(MAKE) e2e GROUP=$$g || exit 1; done

e2e-check-groups:
	node tests/e2e/lib/groups.mjs

# make docs-check   every relative link of the Markdown files (README, CONTRIBUTING, docs/, tools/, examples/) resolves, fragments included (part of make test)
docs-check:
	node tests/ci/links.mjs

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

# make get-tailwind [TAILWIND_VERSION=v4.3.3]   the Tailwind standalone binary for this machine (macOS arm64/x64, Linux x64/arm64) into ./tailwindcss; default: the latest release
TAILWIND_VERSION ?= latest
get-tailwind:
	@os=$$(uname -s); arch=$$(uname -m); \
	case "$$os-$$arch" in \
	  Darwin-arm64) asset=macos-arm64 ;; \
	  Darwin-x86_64) asset=macos-x64 ;; \
	  Linux-x86_64) asset=linux-x64 ;; \
	  Linux-aarch64|Linux-arm64) asset=linux-arm64 ;; \
	  *) echo "get-tailwind: no standalone binary known for $$os $$arch; download one from https://github.com/tailwindlabs/tailwindcss/releases and save it as ./tailwindcss (chmod +x)" >&2; exit 1 ;; \
	esac; \
	if [ "$(TAILWIND_VERSION)" = latest ]; then url=https://github.com/tailwindlabs/tailwindcss/releases/latest/download/tailwindcss-$$asset; \
	else url=https://github.com/tailwindlabs/tailwindcss/releases/download/$(TAILWIND_VERSION)/tailwindcss-$$asset; fi; \
	echo "get-tailwind: $$url"; \
	curl -fsSL "$$url" -o tailwindcss && chmod +x tailwindcss
