override DEV_DIR := .lattice-dev
DEV_HANDLE ?= $(shell id -un | tr '[:upper:]' '[:lower:]' | tr -cs 'a-z0-9-' '-' | sed 's/^-//;s/-$$//')
DEV_PUBLISHER ?= dev.$(DEV_HANDLE)
DEVPLUGIN ?= go run github.com/LatticeNet/lattice-server/tools/devplugin@f98fe94e31da86296c7aa9b5bdb97d6e1f7a51c5
override DEV_SEED := $(DEV_DIR)/publisher.seed
override DEV_TRUST := $(DEV_DIR)/plugin-trust.local.json
override DEV_BUNDLE := $(DEV_DIR)/reference-plugin.tar.gz
override DEV_MANIFEST := $(DEV_DIR)/manifest.dev.json

.PHONY: dev-key dev-bundle dev-plugin

dev-key:
	$(DEVPLUGIN) keygen -publisher "$(DEV_PUBLISHER)" -seed "$(DEV_SEED)" -trust "$(DEV_TRUST)"

dev-bundle:
	mkdir -p "$(DEV_DIR)"
	set -eu; \
	stage=$$(mktemp -d "$(DEV_DIR)/bundle.XXXXXX"); \
	trap 'rm -rf "$$stage"' EXIT; \
	mkdir -p "$$stage/bin/linux-amd64" "$$stage/bin/linux-arm64" "$$stage/ui"; \
	(cd system-go && GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -buildvcs=false -o "../$$stage/bin/linux-amd64/plugin" .); \
	(cd system-go && GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -trimpath -buildvcs=false -o "../$$stage/bin/linux-arm64/plugin" .); \
	(cd ui && npm run build); \
	cp -R ui/dist/. "$$stage/ui"; \
	(cd tools/pluginpack && go run ./cmd/pluginpack -source "../../$$stage" -output "../../$(DEV_BUNDLE)")

dev-plugin: dev-bundle
	$(DEVPLUGIN) sign -publisher "$(DEV_PUBLISHER)" -seed "$(DEV_SEED)" -manifest manifest.json -artifact "$(DEV_BUNDLE)" -output "$(DEV_MANIFEST)"
