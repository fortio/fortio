# Makefile to build fortio's docker images as well as short cut
# for local test/install
#
# See also release/README.md
#

IMAGES=echosrv fcurl # plus the combo image / Dockerfile without ext.

DOCKER_PREFIX := docker.io/fortio/fortio
BUILD_IMAGE_TAG := v50@sha256:fe69c193d8ad40eb0d791984881f3678aead02660b8e3468c757f717892ada4c
BUILDX_PLATFORMS := linux/amd64,linux/arm64,linux/ppc64le,linux/s390x
BUILDX_POSTFIX :=
ifeq '$(shell echo $(BUILDX_PLATFORMS) | awk -F "," "{print NF-1}")' '0'
	BUILDX_POSTFIX = --load
endif
BUILD_IMAGE := $(DOCKER_PREFIX).build:$(BUILD_IMAGE_TAG)

TAG:=$(USER)$(shell date +%y%m%d_%H%M%S)

DOCKER_TAG = $(DOCKER_PREFIX)$(IMAGE):$(TAG)

CERT_TEMP_DIR := ./cert-tmp/

# go test ./... and others run in vendor/ and cause problems (!)
# so to avoid `can't load package: package fortio.org/fortio/...: no Go files in ...`
# note that only go1.8 needs the grep -v vendor but we are compatible with 1.8
# ps: can't use go list (and get packages as canonical fortio.org/fortio/x)
# as somehow that makes gometaliner silently not find/report errors...
PACKAGES ?= $(shell go list ./...)
# from fortio 1.4 we use go 1.14 (from go 1.8 up to fortio 1.3) and switched to go modules (from vendor)

# Local targets:
go-install:
	go install $(PACKAGES)

# Run/test dependencies
dependencies: certs

# Only generate certs if needed
certs: $(CERT_TEMP_DIR)/server.cert

# Generate certs for unit and release tests.
$(CERT_TEMP_DIR)/server.cert: cert-gen
	./cert-gen

# Remove certificates
certs-clean:
	rm -rf $(CERT_TEMP_DIR)

TEST_TIMEOUT:=90s

# Local test
test: dependencies
	go test -timeout $(TEST_TIMEOUT) -race $(PACKAGES)

# To debug strange linter errors, uncomment
# DEBUG_LINTERS="--debug"

local-lint:
	govulncheck $(LINT_PACKAGES)
	golangci-lint version
	golangci-lint --timeout 120s $(DEBUG_LINTERS) run $(LINT_PACKAGES)

# Lint everything by default but ok to "make lint LINT_PACKAGES=./fhttp"
LINT_PACKAGES:=./...
lint:
	docker run -v $(CURDIR):/build/fortio $(BUILD_IMAGE) bash -c \
		"cd /build/fortio \
		&& time make local-lint DEBUG_LINTERS=\"$(DEBUG_LINTERS)\" LINT_PACKAGES=\"$(LINT_PACKAGES)\""

docker-test:
	docker run -v $(CURDIR):/build/fortio $(BUILD_IMAGE) bash -c \
		"cd /build/fortio \
		&& time make test"

shell:
	docker run -ti -v $(CURDIR):/build/fortio $(BUILD_IMAGE)

# This really also tests the release process and build on windows,mac,linux
# and the docker images, not just "web" (ui) stuff that it also exercises.
release-test:
	./Webtest.sh

# old name for release-test
webtest: release-test

coverage: dependencies
	./.circleci/coverage.sh
	curl -s https://codecov.io/bash | bash

# Short cut for pulling/updating to latest of the current branch
pull:
	git pull


# Docker: Pushes the combo image and the smaller image(s)
all: test go-install lint docker-version docker-push-internal
	@for img in $(IMAGES); do \
		$(MAKE) docker-push-internal IMAGE=.$$img TAG=$(TAG); \
	done

# When changing the build image, this Makefile should be edited first
# (bump BUILD_IMAGE_TAG), also change this list if the image is used in
# more places.
FILES_WITH_IMAGE:= .circleci/config.yml Dockerfile Dockerfile.echosrv \
	Dockerfile.fcurl release/Dockerfile.in Webtest.sh
# then run make update-build-image and check the diff, etc... see release/README.md
update-build-image:
	docker buildx create --use
	$(MAKE) docker-push-internal IMAGE=.build TAG=$(BUILD_IMAGE_TAG)

SED:=sed
update-build-image-tag:
	@echo 'Need to use gnu sed (brew install gnu-sed; make update-build-image-tag SED=gsed)'
	$(SED) --in-place=.bak -e 's!$(DOCKER_PREFIX).build:v..!$(BUILD_IMAGE)!g' $(FILES_WITH_IMAGE)

docker-default-platform:
	@docker buildx --builder default inspect | tail -1 | sed -e "s/Platforms: //" -e "s/,//g" | awk '{print $$1}'

docker-version:
	@echo "### Docker is `which docker`"
	@docker version

docker-internal: dependencies
	@echo "### Now building $(DOCKER_TAG)"
	docker buildx build --platform $(BUILDX_PLATFORMS) --build-arg MODE=$(MODE) -f Dockerfile$(IMAGE) -t $(DOCKER_TAG) $(BUILDX_POSTFIX) .

docker-push-internal: docker-internal docker-buildx-push

docker-buildx-push:
	@echo "### Now pushing $(DOCKER_TAG)"
	docker buildx build --push --platform $(BUILDX_PLATFORMS) -f Dockerfile$(IMAGE) -t $(DOCKER_TAG) .

release: dist
	release/release.sh

.PHONY: all docker-internal docker-push-internal docker-version test dependencies

.PHONY: go-install lint install-linters coverage webtest release-test update-build-image

.PHONY: local-lint update-build-image-tag release pull certs certs-clean

# Targets used for official builds (initially from Dockerfile)
BUILD_DIR := /tmp/fortio_build
BUILD_DIR_ABS := $(abspath $(BUILD_DIR))
BUILD_DIR_BIN := $(BUILD_DIR_ABS)/bin
OFFICIAL_BIN ?= $(BUILD_DIR)/result/fortio
OFFICIAL_DIR ?= $(dir $(OFFICIAL_BIN))

GOOS :=
GO_BIN := go
GIT_TAG ?= $(shell git describe --tags --match 'v*' --dirty)
DIST_VERSION ?= $(shell echo $(GIT_TAG) | sed -e "s/^v//")
GIT_SHA ?= $(shell git rev-parse HEAD)
# Main/default binary to build: (can be changed to build fcurl or echosrv instead)
OFFICIAL_TARGET := fortio.org/fortio
MODE ?= install

debug-tags:
	@echo "GIT_TAG=$(GIT_TAG)"
	@echo "DIST_VERSION=$(DIST_VERSION)"

echo-version:
	@echo "$(DIST_VERSION)"

# FPM (for rpm...) converts - to _
echo-package-version:
	@echo "$(DIST_VERSION)" | sed -e "s/-/_/g"

$(BUILD_DIR):
	mkdir -p $(BUILD_DIR)

$(OFFICIAL_DIR):
	mkdir -p $(OFFICIAL_DIR)

.PHONY: official-build official-build-internal official-build-version official-build-clean

official-build: official-build-internal

official-build-internal: $(BUILD_DIR) $(OFFICIAL_DIR)
	$(GO_BIN) version
ifeq ($(MODE),install)
	GOPATH=$(BUILD_DIR_ABS) CGO_ENABLED=0 GOOS=$(GOOS) $(GO_BIN) install -a -ldflags -s $(OFFICIAL_TARGET)@v$(DIST_VERSION)
	# rename when building cross architecture (on windows it has .exe suffix thus the *)
	ls -lR $(BUILD_DIR_BIN)
	-mv -f $(BUILD_DIR_BIN)/*_*/fortio* $(BUILD_DIR_BIN)
	-rmdir $(BUILD_DIR_BIN)/*_*
	mv -f $(BUILD_DIR_BIN)/fortio* $(OFFICIAL_DIR)
else
	CGO_ENABLED=0 GOOS=$(GOOS) $(GO_BIN) build -a -ldflags -s -o $(OFFICIAL_BIN) $(OFFICIAL_TARGET)
endif

official-build-version: official-build
	$(OFFICIAL_BIN) version

official-build-clean:
	-$(RM) $(OFFICIAL_BIN) release/Makefile

# Create a complete source tree with naming matching debian package conventions
TAR ?= tar # on macos need gtar to get --owner
DIST_PATH:=release/fortio_$(DIST_VERSION).orig.tar

.PHONY: dist dist-sign distclean

release/Makefile: release/Makefile.dist
	echo "GIT_TAG := $(GIT_TAG)" > $@
	echo "GIT_SHA := $(GIT_SHA)" >> $@
	cat $< >> $@

dist: release/Makefile
	# put the source files where they can be used as gopath by go,
	# except leave the debian dir where it needs to be (below the version dir)
	git ls-files \
		| awk '{printf("fortio/%s\n", $$0)}' \
		| (cd ../ ; $(TAR) \
		--xform="s|^fortio/|fortio-$(DIST_VERSION)/src/fortio.org/fortio/|;s|^.*debian/|fortio-$(DIST_VERSION)/debian/|" \
		--owner=0 --group=0 -c -f - -T -) > $(DIST_PATH)
	# move the release/Makefile at the top (after the version dir)
	$(TAR) --xform="s|^release/|fortio-$(DIST_VERSION)/|" \
		--owner=0 --group=0 -r -f $(DIST_PATH) release/Makefile
	gzip -f $(DIST_PATH)
	@echo "Created $(CURDIR)/$(DIST_PATH).gz"

dist-sign:
	gpg --armor --detach-sign $(DIST_PATH)

distclean: official-build-clean
	-rm -f *.profile.* */*.profile.*
	-rm -rf $(CERT_TEMP_DIR)

# Install target more compatible with standard gnu/debian practices. Uses DESTDIR as staging prefix

install: official-install

.PHONY: install official-install

BIN_INSTALL_DIR = $(DESTDIR)/usr/bin
MAN_INSTALL_DIR = $(DESTDIR)/usr/share/man/man1
BIN_INSTALL_EXEC = fortio

official-install: official-build-clean official-build-version
	-mkdir -p $(BIN_INSTALL_DIR) $(MAN_INSTALL_DIR)
	cp $(OFFICIAL_BIN) $(BIN_INSTALL_DIR)/$(BIN_INSTALL_EXEC)
	cp docs/fortio.1 $(MAN_INSTALL_DIR)

# Test distribution (only used by maintainer)

.PHONY: debian-dist-common debian-dist-test debian-dist debian-sbuild

# warning, will be cleaned
TMP_DIST_DIR:=~/tmp/fortio-dist

# debian getting version from debian/changelog while we get it from git tags
# doesn't help making this simple: (TODO: unify or autoupdate the 3 versions)

debian-dist-common:
	$(MAKE) dist TAR=tar
	-mkdir -p $(TMP_DIST_DIR)
	rm -rf $(TMP_DIST_DIR)/fortio*
	cp $(CURDIR)/$(DIST_PATH).gz $(TMP_DIST_DIR)
	cd $(TMP_DIST_DIR); tar xfz *.tar.gz
	-cd $(TMP_DIST_DIR);\
		ln -s *.tar.gz fortio_`cd fortio-$(DIST_VERSION); dpkg-parsechangelog -S Version | sed -e "s/-.*//"`.orig.tar.gz

debian-dist-test: debian-dist-common
	cd $(TMP_DIST_DIR)/fortio-$(DIST_VERSION); FORTIO_SKIP_TESTS=Y dpkg-buildpackage -us -uc
	cd $(TMP_DIST_DIR)/fortio-$(DIST_VERSION); lintian

debian-dist: distclean debian-dist-common
	cd $(TMP_DIST_DIR)/fortio-$(DIST_VERSION); FORTIO_SKIP_TESTS=N dpkg-buildpackage -ap
	cd $(TMP_DIST_DIR)/fortio-$(DIST_VERSION); lintian

# assumes you ran one of the previous 2 target first
debian-sbuild:
	cd $(TMP_DIST_DIR)/fortio-$(DIST_VERSION); sbuild

info:
	@echo "GIT_SHA=$(GIT_SHA)"
	@echo "GIT_TAG=$(GIT_TAG)"
	pwd
	ls -la
