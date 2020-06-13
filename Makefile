SDK_VERSION = v0.18.1
MACHINE = $(shell uname -m)

operator-sdk:
	# Download sdk only if it's not available.
	@if [ ! -f operator-sdk ]; then \
		curl -Lo operator-sdk https://github.com/operator-framework/operator-sdk/releases/download/$(SDK_VERSION)/operator-sdk-$(SDK_VERSION)-$(MACHINE)-linux-gnu && \
		chmod +x operator-sdk; \
	fi