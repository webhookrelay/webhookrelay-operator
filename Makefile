JOBDATE		?= $(shell date -u +%Y-%m-%dT%H%M%SZ)
GIT_REVISION	= $(shell git rev-parse --short HEAD)
VERSION		?= $(shell git describe --tags --abbrev=0)
OPERATOR_PREVIOUS_VERSION	?= $(shell git describe --abbrev=0 --tags $(VERSION)^)
OPERATOR_IMAGE ?= webhookrelay/webhookrelay-operator:test

GO_ENV = GOOS=linux CGO_ENABLED=0
GO_BUILD_CMD = go build
SDK_VERSION = v0.18.1
MACHINE = $(shell uname -m)
BUILD_DIR = "build"
YQ = $(BUILD_DIR)/yq
GOLANGCI_LINT = $(BUILD_DIR)/golangci-lint
OPERATOR_SDK = $(BUILD_DIR)/operator-sdk

# Build operator binary
.PHONY: build
build:
	@echo "Building Webhook Relay operator"
	$(GO_ENV) $(GO_BUILD_CMD) -ldflags "$(LDFLAGS)" \
		-o ./build/_output/bin/webhookrelay-operator \
		./cmd/manager

ko-build:
	ko build --platform=all --bare -t latest -t $(VERSION) ./cmd/*

##############################
#           DEV              #
##############################

# Generate APIs, CRD specs and CRD clientset.
go-gen:
	$(OPERATOR_SDK) generate k8s
	$(OPERATOR_SDK) generate crds
	cp deploy/crds/forward.webhookrelay.com_webhookrelayforwards_crd.yaml charts/webhookrelay-operator/crds/crd.yaml

# Run tests
test:
	go install github.com/mfridman/tparse@v0.9.0
	go test -json -v `go list ./... | egrep -v /tests` -cover | tparse -all -smallscreen

## Start local Webhook Relay operator
local-run:
	OPERATOR_NAME=webhookrelay-operator $(OPERATOR_SDK) run local --operator-flags="--zap-devel"

clean-crd:
	kubectl delete -f deploy/crds/forward.webhookrelay.com_webhookrelayforwards_crd.yaml

add-cr:
	kubectl apply -f deploy/crds

image-operator:
	docker build . -f build/Dockerfile -t $(OPERATOR_IMAGE)

# Cross-platform images
buildx-images:	
	docker buildx build --builder synpse \
	  --push \
		--cache-from=type=registry,ref=webhookrelay/webhookrelay-operator \
	  --platform linux/amd64,linux/arm64,linux/arm/v7 \
	  -t webhookrelay/webhookrelay-operator:dev \
	  -f build/Dockerfile .

# buildkit-images:
# 	buildctl build \
#   --frontend dockerfile.v0 \
# 	--local context=. \
#   --local dockerfile=./build \
#   --opt platform=linux/amd64,linux/arm64,linux/arm/v7 \
#   --output type=image,name=docker.io/webhookrelay/webhookrelay-operator:latest,push=true \
# 	...

lint:
	$(GOLANGCI_LINT) run --timeout 5m

##############################
#     Third-party tools      #
##############################

operator-sdk:
	# Download sdk only if it's not available.
	@if [ ! -f $(OPERATOR_SDK) ]; then \
		curl -Lo $(OPERATOR_SDK) https://github.com/operator-framework/operator-sdk/releases/download/$(SDK_VERSION)/operator-sdk-$(SDK_VERSION)-$(MACHINE)-linux-gnu && \
		chmod +x $(OPERATOR_SDK); \
	fi

yq: ## Install yq.
	@if [ ! -f $(YQ) ]; then \
		curl -Lo $(YQ) https://github.com/mikefarah/yq/releases/download/2.3.0/yq_linux_amd64 && \
		chmod +x $(YQ); \
	fi

golangci-lint: ## Install golangci-lint
	@if [ ! -f $(GOLANGCI_LINT) ]; then \
		curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $(BUILD_DIR) v1.50.1; \
	fi