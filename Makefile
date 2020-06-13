SDK_VERSION = v0.18.1
MACHINE = $(shell uname -m)
BUILD_DIR = "build"
YQ = $(BUILD_DIR)/yq
GOLANGCI_LINT = $(BUILD_DIR)/golangci-lint
OPERATOR_SDK = $(BUILD_DIR)/operator-sdk


# Generate APIs, CRD specs and CRD clientset.
go-gen:
	$(OPERATOR_SDK) generate k8s
	$(OPERATOR_SDK) generate crds

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
		curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $(BUILD_DIR) v1.27.0; \
	fi