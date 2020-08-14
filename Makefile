# Makefile to build fortio's docker images as well as short cut
# for local test/install
#
# See also release/README.md
#

IMAGES=echosrv fcurl # plus the combo image / Dockerfile without ext.

DOCKER_PREFIX := docker.io/fortio/fortio
BUILD_IMAGE_TAG := v22
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
	golangci-lint $(DEBUG_LINTERS) run $(LINT_PACKAGES)

# Lint everything by default but ok to "make lint LINT_PACKAGES=./fhttp"
LINT_PACKAGES:=./...
lint:
	docker run -v $(CURDIR):/go/src/fortio.org/fortio $(BUILD_IMAGE) bash -c \
		"cd /go/src/fortio.org/fortio \
		&& time make local-lint DEBUG_LINTERS=\"$(DEBUG_LINTERS)\" LINT_PACKAGES=\"$(LINT_PACKAGES)\""

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
	Dockerfile.test Dockerfile.fcurl release/Dockerfile.in Webtest.sh
# then run make update-build-image and check the diff, etc... see release/README.md
update-build-image:
	docker pull ubuntu:focal
	$(MAKE) docker-push-internal IMAGE=.build TAG=$(BUILD_IMAGE_TAG)

update-build-image-tag:
	@echo 'Need to use gnu sed (brew install gnu-sed; PATH="/usr/local/opt/gnu-sed/libexec/gnubin:$$PATH")'
	sed --in-place=.bak -e 's!$(DOCKER_PREFIX).build:v..!$(BUILD_IMAGE)!g' $(FILES_WITH_IMAGE)

docker-version:
	@echo "### Docker is `which docker`"
	@docker version

docker-internal: dependencies
	@echo "### Now building $(DOCKER_TAG)"
	docker build -f Dockerfile$(IMAGE) -t $(DOCKER_TAG) .

docker-push-internal: docker-internal
	@echo "### Now pushing $(DOCKER_TAG)"
	docker push $(DOCKER_TAG)

release: dist
	release/release.sh

.PHONY: all docker-internal docker-push-internal docker-version test dependencies

.PHONY: go-install lint install-linters coverage webtest release-test update-build-image

.PHONY: local-lint update-build-image-tag release pull certs certs-clean

# Targets used for official builds (initially from Dockerfile)
BUILD_DIR := /tmp/fortio_build
LIB_DIR := /usr/share/fortio
DATA_DIR := .
OFFICIAL_BIN := ../fortio.bin
GOOS := 
GO_BIN := go
GIT_STATUS ?= $(strip $(shell git status --porcelain | wc -l))
GIT_TAG ?= $(shell git describe --tags --match 'v*')
GIT_SHA ?= $(shell git rev-parse HEAD)
# Main/default binary to build: (can be changed to build fcurl or echosrv instead)
OFFICIAL_TARGET := fortio.org/fortio

# Putting spaces in linker replaced variables is hard but does work.
# This sets up the static directory outside of the go source tree and
# the default data directory to a /var/lib/... volume
# + rest of build time/git/version magic.

$(BUILD_DIR)/build-info.txt:
	-mkdir -p $(BUILD_DIR)
	echo "$(shell date +'%Y-%m-%d %H:%M') $(GIT_SHA)" > $@

$(BUILD_DIR)/link-flags.txt: $(BUILD_DIR)/build-info.txt
	echo "-s -X fortio.org/fortio/ui.resourcesDir=$(LIB_DIR) -X main.defaultDataDir=$(DATA_DIR) \
  -X \"fortio.org/fortio/version.buildInfo=$(shell cat $<)\" \
  -X fortio.org/fortio/version.tag=$(GIT_TAG) \
  -X fortio.org/fortio/version.gitstatus=$(GIT_STATUS)" | tee $@

.PHONY: official-build official-build-version official-build-clean

official-build: $(BUILD_DIR)/link-flags.txt
	$(GO_BIN) version
	CGO_ENABLED=0 GOOS=$(GOOS) $(GO_BIN) build -a -ldflags '$(shell cat $(BUILD_DIR)/link-flags.txt)' -o $(OFFICIAL_BIN) $(OFFICIAL_TARGET)
	
official-build-version: official-build
	$(OFFICIAL_BIN) version

official-build-clean:
	-$(RM) $(BUILD_DIR)/build-info.txt $(BUILD_DIR)/link-flags.txt $(OFFICIAL_BIN) release/Makefile

# Create a complete source tree with naming matching debian package conventions
TAR ?= tar # on macos need gtar to get --owner
DIST_VERSION ?= $(shell echo $(GIT_TAG) | sed -e "s/^v//")
DIST_PATH:=release/fortio_$(DIST_VERSION).orig.tar

.PHONY: dist dist-sign distclean

release/Makefile: release/Makefile.dist
	echo "GIT_TAG := $(GIT_TAG)" > $@
	echo "GIT_STATUS := $(GIT_STATUS)" >> $@
	echo "GIT_SHA := $(GIT_SHA)" >> $@
	cat $< >> $@

dist: release/Makefile
	# put the source files where they can be used as gopath by go,
	# except leave the debian dir where it needs to be (below the version dir)
	git ls-files \
		| awk '{printf("src/fortio.org/fortio/%s\n", $$0)}' \
		| (cd ../../.. ; $(TAR) \
		--xform="s|^src|fortio-$(DIST_VERSION)/src|;s|^.*debian/|fortio-$(DIST_VERSION)/debian/|" \
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
LIB_INSTALL_DIR = $(DESTDIR)$(LIB_DIR)
MAN_INSTALL_DIR = $(DESTDIR)/usr/share/man/man1
#DATA_INSTALL_DIR = $(DESTDIR)$(DATA_DIR)
BIN_INSTALL_EXEC = fortio

official-install: official-build-clean official-build-version
	-mkdir -p $(BIN_INSTALL_DIR) $(LIB_INSTALL_DIR) $(MAN_INSTALL_DIR) # $(DATA_INSTALL_DIR)
	# -chmod 1777 $(DATA_INSTALL_DIR)
	cp $(OFFICIAL_BIN) $(BIN_INSTALL_DIR)/$(BIN_INSTALL_EXEC)
	cp -r ui/templates ui/static $(LIB_INSTALL_DIR)
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
