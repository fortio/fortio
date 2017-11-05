# Makefile to build fortio's docker images as well as short cut
# for local test/install

IMAGES=echosrv # plus the combo image / Dockerfile without ext.

DOCKER_PREFIX := docker.io/istio/fortio

TAG:=$(USER)$(shell date +%y%m%d_%H%M%S)

DOCKER_TAG = $(DOCKER_PREFIX)$(IMAGE):$(TAG)

# Local targets:
install: test
	go install ./...

test:
	go test -timeout 30s -race ./...

lint:
	gometalinter ./...

# Docker: Pushes the combo image and the smaller image(s)
all: install docker-version docker-push-internal
	@for img in $(IMAGES); do \
		$(MAKE) docker-push-internal IMAGE=.$$img TAG=$(TAG); \
	done

docker-version:
	@echo "### Docker is `which docker`"
	@docker version

docker-internal:
	@echo "### Now building $(DOCKER_TAG)"
	docker build -f Dockerfile$(IMAGE) -t $(DOCKER_TAG) .

docker-push-internal: docker-internal
	@echo "### Now pushing $(DOCKER_TAG)"
	docker push $(DOCKER_TAG)

authorize:
	gcloud docker --authorize-only --project istio-testing

.PHONY: all docker-internal docker-push-internal docker-version authorize test install lint
