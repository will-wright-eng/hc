REPO_ROOT := $(shell git rev-parse --show-toplevel)

.PHONY: $(shell sed -n -e '/^$$/ { n ; /^[^ .\#][^ ]*:/ { s/:.*$$// ; p ; } ; }' $(MAKEFILE_LIST))

.DEFAULT_GOAL := help

help: ## list make commands
	@echo ${MAKEFILE_LIST}
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "\033[36m%-30s\033[0m %s\n", $$1, $$2}' $(MAKEFILE_LIST)

build: ## build the hc binary
	go build -o hc $(REPO_ROOT)/cmd/hc

test: ## run tests
	go test $(REPO_ROOT)/...

clean: ## clean the build artifacts
	rm -f hc

install: ## install the hc binary
	go install $(REPO_ROOT)/cmd/hc

lint: ## run linting
	go vet $(REPO_ROOT)/...

e2e: ## run e2e tests with decay, indentation, and report
	./hc analyze --json | ./hc report

eval-ignore: ## eval `hc prompt ignore | claude -p` coverage (TRIALS=N, OUTDIR=path)
	uv run --script $(REPO_ROOT)/scripts/eval_ignore_prompt.py -n 5 -o /tmp/eval-ignore/
