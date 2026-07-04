# CoreStack monorepo - delegates to each provider's own module.

# Each provider (aws/, azure/, gcp/) is an independent Go module with its own Makefile. These targets fan out to then so you can build/test everything from the root,
# or scope to one provider

# make ci              # full gate: fmt-check + build + vet + test (all three)
# make build-all       # build every provider
# make test-all        # test every provider
# make aws-test        # test just aws
# make azure-build     # build just azure

# `make ci` is the single command a CI runner (or you, pre-push) invokes. There is no platform-specific CI config yet - this local target IS the contract,
# so any future GitHub Actions / GitLab CI file need only run `make ci`.

.PHONY:  ci fmt-check build-all test-all vet-all \
			aws-build aws-test aws-smoke \
			azure-build azure-test \
			gcp-build gcp-test

# --- CI gate ---
# Ordered fail-fast: formatting, then compile, then vet, then tests. Nothing here needs network or Docker; it is safe to run in a clean container with only the
# Go toolchain (1.23+, matching go.work).
ci: 	fmt-check build-all vet-all test-all
		@echo "ci: all providers built, vetted, and tested"

fmt-check:
	@echo "==> gofmt check"
	@unformatted=$$(gofmt -l aws azure gcp); \
	if [ -n "$$unformatted" ]; then \
		echo "The following files are not gofmt-clean:"; \
		echo "$$unformatted";\
		echo "Run 'gofmt -w' (or 'make fmt' in the module) to fix."; \
		exit 1; \
	fi; \
	echo "gofmt: clean"

build-all: aws-build azure-build gcp-build
test-all: aws-test azure-test gcp-test
vet-all:
	cd aws && go vet ./...
	cd azure && go vet ./...
	cd gcp && go vet ./...

aws-build:
	$(MAKE) -C aws build
aws-test:
	$(MAKE) -C aws test
aws-smoke:
	$(MAKE) -C aws smoke
azure-build:
	$(MAKE) -C azure build
azure-test:
	$(MAKE) -C azure test
gcp-build:
	$(MAKE) -C gcp build
gcp-test:
	$(MAKE) -C gcp test