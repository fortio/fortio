# Makefile to build fortio's docker images as well as short cut
# for local test/install

IMAGES=echosrv # plus the combo image / Dockerfile without ext.

DOCKER_PREFIX := docker.io/istio/fortio
BUILD_IMAGE_TAG := v5
BUILD_IMAGE := istio/fortio.build:$(BUILD_IMAGE_TAG)

TAG:=$(USER)$(shell date +%y%m%d_%H%M%S)

DOCKER_TAG = $(DOCKER_PREFIX)$(IMAGE):$(TAG)

# go test ./... and others run in vendor/ and cause problems (!)
PACKAGES:=$(shell find . -type d -print | egrep -v "/(\.|vendor|static|templates|release|docs)")

# Marker for whether vendor submodule is here or not already
GRPC_DIR:=./vendor/google.golang.org/grpc

# Local targets:
install: submodule
	go install $(PACKAGES)

# Local test
test: submodule
	go test -timeout 60s -race $(PACKAGES)

# To debug linters, uncomment
#DEBUG_LINTERS="--debug"

local-lint: submodule
	gometalinter $(DEBUG_LINTERS) \
	--deadline=180s --enable-all --aggregate \
	--exclude=.pb.go --disable=gocyclo --line-length=132 $(LINT_PACKAGES)

# Lint everything by default but ok to "make lint LINT_PACKAGES=./fhttp"
LINT_PACKAGES:=$(PACKAGES)
# TODO: do something about cyclomatic complexity
# Note CGO_ENABLED=0 is needed to avoid errors as gcc isn't part of the
# build image
lint: submodule
	docker run -v $(shell pwd):/go/src/istio.io/fortio $(BUILD_IMAGE) bash -c \
		"cd fortio && time go install $(LINT_PACKAGES) \
		&& time make local-lint LINT_PACKAGES=\"$(LINT_PACKAGES)\""

webtest:
	./Webtest.sh

coverage: submodule
	./.circleci/coverage.sh
	curl -s https://codecov.io/bash | bash

# Submodule handling when not already there
submodule: $(GRPC_DIR)

$(GRPC_DIR):
	$(MAKE) submodule-sync

# If you want to force update/sync, invoke 'make submodule-sync' directly
submodule-sync:
	git submodule sync
	git submodule update --init


# Docker: Pushes the combo image and the smaller image(s)
all: test install lint docker-version docker-push-internal
	@for img in $(IMAGES); do \
		$(MAKE) docker-push-internal IMAGE=.$$img TAG=$(TAG); \
	done

# Makefile should be edited first
FILES_WITH_IMAGE:= .circleci/config.yml Dockerfile Dockerfile.echosrv \
	Dockerfile.test release/Dockerfile.in
# Ran make update-build-image BUILD_IMAGE_TAG=v1 DOCKER_PREFIX=fortio/fortio
update-build-image:
	$(MAKE) docker-push-internal IMAGE=.build TAG=$(BUILD_IMAGE_TAG)

# Change . to .. when getting to v10 and up...
update-build-image-tag:
	sed -i .bak -e 's!istio/fortio.build:v.!$(BUILD_IMAGE)!g' $(FILES_WITH_IMAGE)

docker-version:
	@echo "### Docker is `which docker`"
	@docker version

docker-internal: submodule
	@echo "### Now building $(DOCKER_TAG)"
	docker build -f Dockerfile$(IMAGE) -t $(DOCKER_TAG) .

docker-push-internal: docker-internal
	@echo "### Now pushing $(DOCKER_TAG)"
	docker push $(DOCKER_TAG)

release: submodule
	release/release.sh

authorize:
	gcloud docker --authorize-only --project istio-testing

.PHONY: all docker-internal docker-push-internal docker-version authorize test

.PHONY: install lint install-linters coverage weblint update-build-image

.PHONY: local-lint update-build-image-tag release submodule submodule-sync
