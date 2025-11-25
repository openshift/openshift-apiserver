all: build
.PHONY: all

# Include the library makefile
include $(addprefix ./vendor/github.com/openshift/build-machinery-go/make/, \
	golang.mk \
	targets/openshift/deps.mk \
	targets/openshift/images.mk \
)

IMAGE_REGISTRY?=registry.svc.ci.openshift.org

# This will call a macro called "build-image" which will generate image specific targets based on the parameters:
# $0 - macro name
# $1 - target suffix
# $2 - Dockerfile path
# $3 - context directory for image build
# It will generate target "image-$(1)" for builing the image an binding it as a prerequisite to target "images".
$(call build-image,ocp-openshift-apiserver,$(IMAGE_REGISTRY)/ocp/4.3:openshift-apiserver,./images/Dockerfile.rhel,.)

$(call verify-golang-versions,images/Dockerfile.rhel)

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
