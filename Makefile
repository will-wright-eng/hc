REPO_ROOT := $(shell git rev-parse --show-toplevel)
HOTSPOTS_JSON ?= hotspots.json
CHANGED_TXT ?= changed.txt

VERSION ?= dev
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "none")
LDFLAGS  = -s -w -X main.version=$(VERSION) -X main.commit=$(COMMIT)

.PHONY: $(shell sed -n -e '/^$$/ { n ; /^[^ .\#][^ ]*:/ { s/:.*$$// ; p ; } ; }' $(MAKEFILE_LIST))

.DEFAULT_GOAL := help

help: ## list make commands
	@echo ${MAKEFILE_LIST}
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "\033[36m%-30s\033[0m %s\n", $$1, $$2}' $(MAKEFILE_LIST)

build: ## build the hc binary
	go build -ldflags "$(LDFLAGS)" -o hc $(REPO_ROOT)/cmd/hc

test: ## run tests
	go test $(REPO_ROOT)/...

clean: ## clean the build artifacts
	rm -f hc

install: ## install the hc binary
	go install $(REPO_ROOT)/cmd/hc

lint: ## run linting
	go vet $(REPO_ROOT)/...

e2e: ## run e2e tests with decay, indentation, and report
	./hc analyze --json | ./hc md report

pr-changed-files: ## write changed.txt for BASE_SHA...HEAD_SHA
	@test -n "$${BASE_SHA:-}" || (echo "BASE_SHA is required" >&2; exit 1)
	@test -n "$${HEAD_SHA:-}" || (echo "HEAD_SHA is required" >&2; exit 1)
	git diff --name-only --diff-filter=ACM "$${BASE_SHA}...$${HEAD_SHA}" -- > "$(CHANGED_TXT)"

pr-hotspots-json: ## write hotspots.json by analyzing ../hc-base restricted to changed.txt
	$(REPO_ROOT)/hc analyze --json --files-from "$(CHANGED_TXT)" ../hc-base > "$(HOTSPOTS_JSON)"

pr-hotspots-json-no-min-age: ## like pr-hotspots-json but disables the 14-day file age floor (for manual reruns)
	$(REPO_ROOT)/hc analyze --json --no-min-age --files-from "$(CHANGED_TXT)" ../hc-base > "$(HOTSPOTS_JSON)"

pr-file-comments: ## post/update PR file hotspot comments from hotspots.json
	$(REPO_ROOT)/scripts/post-pr-file-comments.sh "$(HOTSPOTS_JSON)"

eval-ignore: ## eval `hc md ignore | claude -p` coverage (TRIALS=N, OUTDIR=path)
	uv run --script $(REPO_ROOT)/scripts/eval_ignore_prompt.py -n 5 -o /tmp/eval-ignore/
