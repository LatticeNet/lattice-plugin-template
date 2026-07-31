DEV_DIR ?= .lattice-dev
DEV_HANDLE ?= $(shell id -un | tr '[:upper:]' '[:lower:]' | tr -cs 'a-z0-9-' '-' | sed 's/^-//;s/-$$//')
DEV_PUBLISHER ?= dev.$(DEV_HANDLE)
DEVPLUGIN ?= go run github.com/LatticeNet/lattice-server/tools/devplugin@integration
DEV_SEED ?= $(DEV_DIR)/publisher.seed
DEV_TRUST ?= $(DEV_DIR)/plugin-trust.local.json
DEV_BUNDLE_ROOT ?= $(DEV_DIR)/bundle
DEV_BUNDLE ?= $(DEV_DIR)/reference-plugin.tar.gz
DEV_MANIFEST ?= $(DEV_DIR)/manifest.dev.json

.PHONY: dev-key dev-bundle dev-plugin

dev-key:
	$(DEVPLUGIN) keygen -publisher "$(DEV_PUBLISHER)" -seed "$(DEV_SEED)" -trust "$(DEV_TRUST)"

dev-bundle:
	rm -rf "$(DEV_BUNDLE_ROOT)"
	mkdir -p "$(DEV_BUNDLE_ROOT)/bin/linux-amd64" "$(DEV_BUNDLE_ROOT)/bin/linux-arm64" "$(DEV_BUNDLE_ROOT)/ui"
	cd system-go && GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -buildvcs=false -o "../$(DEV_BUNDLE_ROOT)/bin/linux-amd64/plugin" .
	cd system-go && GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -trimpath -buildvcs=false -o "../$(DEV_BUNDLE_ROOT)/bin/linux-arm64/plugin" .
	cd ui && npm run build
	cp -R ui/dist/. "$(DEV_BUNDLE_ROOT)/ui"
	cd tools/pluginpack && go run ./cmd/pluginpack -source "../../$(DEV_BUNDLE_ROOT)" -output "../../$(DEV_BUNDLE)"

dev-plugin: dev-bundle
	$(DEVPLUGIN) sign -publisher "$(DEV_PUBLISHER)" -seed "$(DEV_SEED)" -manifest manifest.json -artifact "$(DEV_BUNDLE)" -output "$(DEV_MANIFEST)" -force
