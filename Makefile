# ── source repositories ────────────────────────────────────────────────────────
MODELSRV_REPO   := https://github.com/emeland-io/modelsrv
SENSOR_REPO     := https://github.com/emeland-io/modelsrv-git-sensor

# Branch or tag to check out — override on the command line:
#   make build-bins MODELSRV_REF=v1.2.3
#   make build-bins SENSOR_REF=main
# CI (.github/workflows/ci.yml) uses the same defaults via: make -s print-MODELSRV_REF / print-SENSOR_REF
MODELSRV_REF    ?= main
SENSOR_REF      ?= main

# ── local paths ────────────────────────────────────────────────────────────────
SRC_DIR         := _src
MODELSRV_SRC    := $(SRC_DIR)/modelsrv
SENSOR_SRC      := $(SRC_DIR)/modelsrv-git-sensor

MODELSRV_BIN    := $(abspath bin/modelsrv)
SENSOR_BIN      := $(abspath bin/modelsrv-git-sensor)

.PHONY: build-bins test e2e clean \
        clone-modelsrv clone-sensor \
        build-modelsrv build-sensor \
        print-MODELSRV_REF print-SENSOR_REF

# CI and humans: default clone refs (override on CLI: make build-bins MODELSRV_REF=...)
print-MODELSRV_REF:
	@echo $(MODELSRV_REF)

print-SENSOR_REF:
	@echo $(SENSOR_REF)

# ── public targets ─────────────────────────────────────────────────────────────

## build-bins: clone (or update) both repos at the configured refs, then compile.
build-bins: build-modelsrv build-sensor
	@echo "✓ binaries ready in bin/"

## test / e2e: build binaries, then run the full E2E suite.
test e2e: build-bins
	MODELSRV_BIN=$(MODELSRV_BIN) \
	SENSOR_BIN=$(SENSOR_BIN) \
	go test -v -count=1 -timeout=120s ./e2e/...

## clean: remove compiled binaries and cloned source trees.
clean:
	rm -rf bin/ $(SRC_DIR)/

# ── clone / update helpers ─────────────────────────────────────────────────────

# _clone-or-update TARGET_DIR REPO REF
# If the directory has no .git, do a shallow clone at REF.
# If it already exists, fetch REF and check it out (handles both branches and tags).
define _clone-or-update
	@if [ ! -d "$(1)/.git" ]; then \
		echo "→ cloning $(2) @ $(3)"; \
		mkdir -p $(SRC_DIR); \
		git clone --depth=1 --branch "$(3)" "$(2)" "$(1)"; \
	else \
		echo "→ updating $(1) → $(3)"; \
		git -C "$(1)" fetch --depth=1 origin "$(3)"; \
		git -C "$(1)" checkout FETCH_HEAD; \
	fi
endef

clone-modelsrv:
	$(call _clone-or-update,$(MODELSRV_SRC),$(MODELSRV_REPO),$(MODELSRV_REF))

clone-sensor:
	$(call _clone-or-update,$(SENSOR_SRC),$(SENSOR_REPO),$(SENSOR_REF))

# ── build helpers ──────────────────────────────────────────────────────────────

build-modelsrv: clone-modelsrv
	@echo "→ building modelsrv ($(MODELSRV_REF))"
	@mkdir -p bin
	cd $(MODELSRV_SRC) && go build -o $(MODELSRV_BIN) ./cmd/modelsrv
	@echo "  ✓ $(MODELSRV_BIN)"

build-sensor: clone-sensor
	@echo "→ building modelsrv-git-sensor ($(SENSOR_REF))"
	@mkdir -p bin
	cd $(SENSOR_SRC) && go build -o $(SENSOR_BIN) ./cmd/modelsrv-git-sensor
	@echo "  ✓ $(SENSOR_BIN)"
