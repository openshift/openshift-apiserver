# Get the directory where this Makefile is, so we can use it below for including
DIR := $(strip $(shell dirname $(realpath $(lastword $(MAKEFILE_LIST)))))

# Definitions for the extended tests

GO_PKG_NAME := github.com/openshift-eng/openshift-tests-extension

GIT_COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo 'unknown')
BUILD_DATE := $(shell date -u +'%Y-%m-%dT%H:%M:%SZ')
GIT_TREE_STATE := $(shell if git rev-parse --git-dir > /dev/null 2>&1; then if git diff --quiet; then echo clean; else echo dirty; fi; else echo unknown; fi)

LDFLAGS := -X '$(GO_PKG_NAME)/pkg/version.CommitFromGit=$(GIT_COMMIT)' \
           -X '$(GO_PKG_NAME)/pkg/version.BuildDate=$(BUILD_DATE)' \
           -X '$(GO_PKG_NAME)/pkg/version.GitTreeState=$(GIT_TREE_STATE)'


METADATA := $(shell pwd)/.openshift-tests-extension/openshift_payload_openshift-apiserver.json

.PHONY: help
help: #HELP Display essential help.
	@awk 'BEGIN {FS = ":[^#]*#HELP"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n\n"} /^[a-zA-Z_0-9-]+:.*#HELP / { printf "  \033[36m%-17s\033[0m %s\n", $$1, $$2 } ' $(MAKEFILE_LIST)

#SECTION Tests
TOOLS_BIN_DIR := $(CURDIR)/bin

.PHONY: run-suite #HELP Run tests by suite name and generate JUnit report (e.g., make run-suite SUITE=openshift/openshift-apiserver/all ARTIFACT_DIR=/tmp/artifacts)
run-suite: build #HELP Run tests by suite name and generate JUnit report
	@if [ -z "$(SUITE)" ]; then \
		echo "Error: SUITE variable is required. Available suites:"; \
		echo "  - openshift/openshift-apiserver/all"; \
		echo "  - openshift/openshift-apiserver/conformance/parallel"; \
		echo "  - openshift/openshift-apiserver/conformance/serial"; \
		echo "  - openshift/openshift-apiserver/optional/slow"; \
		echo ""; \
		echo "Usage: make run-suite SUITE=openshift/openshift-apiserver/all [ARTIFACT_DIR=/tmp/artifacts]"; \
		exit 1; \
	fi
	@if [ -z "$(ARTIFACT_DIR)" ]; then \
		echo "Warning: ARTIFACT_DIR not set, using current directory for JUnit report"; \
		ARTIFACT_DIR=.; \
	fi
	@mkdir -p $(ARTIFACT_DIR)
	$(TOOLS_BIN_DIR)/openshift-apiserver-tests-ext run-suite $(SUITE) -j $(ARTIFACT_DIR)/junit_$(shell date +%Y%m%d-%H%M%S).xml

#SECTION Development
.PHONY: verify #HELP To verify the code
verify: tidy fmt vet lint

.PHONY: tidy #HELP Run go mod tidy.
tidy:
	go mod tidy

.PHONY: fmt
fmt: #HELP Run go fmt against code.
	go fmt ./...

.PHONY: vet
vet: #HELP Run go vet against code.
	go vet ./...

.PHONY: lint
lint: #HELP Run golangci linter.
	@echo "Linting not configured for tests-extension"

.PHONY: fix-lint
fix-lint: #HELP Fix lint issues
	@echo "Linting not configured for tests-extension"

# GO_COMPLIANCE_POLICY="exempt_all" must only be used for test related binaries.
# It prevents various FIPS compliance policies from being applied to this compilation.
# Do not set globally.
.PHONY: build
build: #HELP Build the extended tests binary
	@mkdir -p $(TOOLS_BIN_DIR)
	GO_COMPLIANCE_POLICY="exempt_all" go build -ldflags "$(LDFLAGS)" -o $(TOOLS_BIN_DIR)/openshift-apiserver-tests-ext ./cmd/...

.PHONY: update-metadata
update-metadata: build #HELP Build and run 'update-metadata' to generate test metadata
	$(TOOLS_BIN_DIR)/openshift-apiserver-tests-ext update --component openshift:payload:openshift-apiserver
	$(MAKE) clean-metadata


# Ensure TestID is unique over time.
# TestID is built over Product:Type:ComponentName:TestDescription
# (i.e. openshift:payload:openshift-apiserver:TestName)
# Details:
# - https://github.com/openshift/enhancements/blob/master/enhancements/testing/openshift-tests-extension.md#test-id
# - https://github.com/openshift-eng/ci-test-mapping
#──────────────────────────────────────────────────────────────
# How to rename a test?
# 1. Run: make list-test-names
# 2. Find the current full test name (e.g. "[sig-abc] My test does XYZ")
# 3. Add a Ginkgo label: ginkgo.Label("original-name:[sig-abc] My test does XYZ"). if there is existing original-name label, please do not update the label again and keep it unchanged.
# 4. Change the test name string and run: make build-update
# **Example**
# It("should pass a renamed sanity check",
# 	Label("original-name:[sig-openshift-apiserver] OLMv1 should pass a trivial sanity check"),
# 	func(ctx context.Context) {
# 		Expect(len("test")).To(BeNumerically(">", 0))
# 	})
# Note: You only add the label once. Do not update it after future renames.
#──────────────────────────────────────────────────────────────
# How to delete a test?
# 1. Run: make list-test-names
# 2. In main.go add:
#    ext.IgnoreObsoleteTests(
#        "[sig-openshift-apiserver] My removed test name",
#    )
# 3. Delete the test code in your suite file (e.g. oas.go)
# 4. Run: make build-update.
# This will regenerate the metadata without the test entry.
#────────────────────────────────────────────────────────────────────
.PHONY: build-update
build-update: build update-metadata #HELP Build and update metadata and sanitize output

.PHONY: clean
clean: #HELP Remove build artifacts
	rm -rf $(TOOLS_BIN_DIR)

#SECTION Metadata

.PHONY: list-test-names
list-test-names: build #HELP Show current full test names
	@$(TOOLS_BIN_DIR)/openshift-apiserver-tests-ext list -o names

# Remove 'codeLocations' to avoid absolute paths like:
# "/Users/$(USER)/go/src/.../oas.go:12"
# These are machine-specific and make the metadata non-idempotent.
# More info: https://issues.redhat.com/browse/TRT-2186
.PHONY: clean-metadata
clean-metadata: #HELP Remove 'codeLocations' from metadata JSON
	@echo "Cleaning metadata (removing codeLocations)..."
	@jq 'map(del(.codeLocations))' $(METADATA) > $(METADATA).tmp && mv $(METADATA).tmp $(METADATA)

.PHONY: verify-metadata #HELP To verify that the metadata was properly updated
verify-metadata: update-metadata
	@if ! git diff --exit-code $(METADATA); then \
		echo "ERROR: Metadata is out of date. Please run 'make build-update' and commit the result."; \
		exit 1; \
	fi
