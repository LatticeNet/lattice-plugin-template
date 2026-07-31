DEV_DIR ?= .lattice-dev
DEV_HANDLE ?= $(shell id -un | tr '[:upper:]' '[:lower:]' | tr -cs 'a-z0-9-' '-' | sed 's/^-//;s/-$$//')
DEV_PUBLISHER ?= dev.$(DEV_HANDLE)
DEVPLUGIN ?= go run github.com/LatticeNet/lattice-server/tools/devplugin@a559b14a278fc4e77052966452fbd04bdc693880
DEV_SEED ?= $(DEV_DIR)/publisher.seed
DEV_TRUST ?= $(DEV_DIR)/plugin-trust.local.json
DEV_BUNDLE ?= $(DEV_DIR)/reference-plugin.tar.gz
DEV_MANIFEST ?= $(DEV_DIR)/manifest.dev.json

.PHONY: dev-key dev-bundle dev-plugin

dev-key:
	$(DEVPLUGIN) keygen -publisher "$(DEV_PUBLISHER)" -seed "$(DEV_SEED)" -trust "$(DEV_TRUST)"

dev-bundle:
	mkdir -p "$(DEV_DIR)"
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
