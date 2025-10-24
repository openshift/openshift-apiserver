all: build
.PHONY: all

# Include the library makefile
include $(addprefix ./vendor/github.com/openshift/build-machinery-go/make/, \
	golang.mk \
	targets/openshift/deps.mk \
	targets/openshift/images.mk \
)

IMAGE_REGISTRY?=registry.svc.ci.openshift.org

# -------------------------------------------------------------------
# OpenShift Tests Extension (OpenShift API Server)
# -------------------------------------------------------------------
TESTS_EXT_BINARY := openshift-apiserver-tests-ext
TESTS_EXT_DIR := ./test/extended/tests-extension
TESTS_EXT_OUTPUT := $(TESTS_EXT_DIR)/$(TESTS_EXT_BINARY)
TESTS_EXT_PACKAGE := ./cmd/openshift-apiserver-tests-ext

TESTS_EXT_GIT_COMMIT := $(shell git rev-parse --short HEAD)
TESTS_EXT_BUILD_DATE := $(shell date -u +'%Y-%m-%dT%H:%M:%SZ')
TESTS_EXT_GIT_TREE_STATE := $(shell if git diff --quiet; then echo clean; else echo dirty; fi)

TESTS_EXT_LDFLAGS := -X 'github.com/openshift-eng/openshift-tests-extension/pkg/version.CommitFromGit=$(TESTS_EXT_GIT_COMMIT)' \
                     -X 'github.com/openshift-eng/openshift-tests-extension/pkg/version.BuildDate=$(TESTS_EXT_BUILD_DATE)' \
                     -X 'github.com/openshift-eng/openshift-tests-extension/pkg/version.GitTreeState=$(TESTS_EXT_GIT_TREE_STATE)'

# This will call a macro called "build-image" which will generate image specific targets based on the parameters:
# $0 - macro name
# $1 - target suffix
# $2 - Dockerfile path
# $3 - context directory for image build
# It will generate target "image-$(1)" for builing the image an binding it as a prerequisite to target "images".
$(call build-image,ocp-openshift-apiserver,$(IMAGE_REGISTRY)/ocp/4.3:openshift-apiserver,./images/Dockerfile.rhel,.)

$(call verify-golang-versions,images/Dockerfile.rhel)

clean:
	$(RM) ./openshift-apiserver
.PHONY: clean

GO_TEST_PACKAGES := ./pkg/... ./cmd/...

update:
	hack/update-generated-conversions.sh
	hack/update-generated-deep-copies.sh
	hack/update-generated-defaulters.sh
	hack/update-generated-openapi.sh
.PHONY: update

verify:
	hack/verify-generated-conversions.sh
	hack/verify-generated-deep-copies.sh
	hack/verify-generated-defaulters.sh
	hack/verify-generated-openapi.sh
.PHONY: verify

# -------------------------------------------------------------------
# Build binary with metadata (CI-compliant)
# -------------------------------------------------------------------
.PHONY: tests-ext-build
tests-ext-build:
	cd $(TESTS_EXT_DIR) && \
	GOOS=$(GOOS) GOARCH=$(GOARCH) GO_COMPLIANCE_POLICY=exempt_all CGO_ENABLED=0 \
	go build -o $(TESTS_EXT_BINARY) -ldflags "$(TESTS_EXT_LDFLAGS)" $(TESTS_EXT_PACKAGE)

# -------------------------------------------------------------------
# Run "update" and strip env-specific metadata
# -------------------------------------------------------------------
.PHONY: tests-ext-update
tests-ext-update: tests-ext-build
	cd $(TESTS_EXT_DIR) && ./$(TESTS_EXT_BINARY) update
	for f in $(TESTS_EXT_DIR)/.openshift-tests-extension/*.json; do \
		jq 'map(del(.codeLocations))' "$$f" > tmpp && mv tmpp "$$f"; \
	done
