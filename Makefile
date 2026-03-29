VERSION ?= v$(shell python3 -c "import json; print(json.load(open('version.json'))['version'])" 2>/dev/null || echo "dev")
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
DATE    ?= $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")

MODULE  = github.com/shichao402/pkv
LDFLAGS = -s -w \
	-X '$(MODULE)/internal/version.Version=$(VERSION)' \
	-X '$(MODULE)/internal/version.Commit=$(COMMIT)' \
	-X '$(MODULE)/internal/version.Date=$(DATE)'

PLATFORMS = darwin/amd64 darwin/arm64 linux/amd64 linux/arm64 windows/amd64 windows/arm64
DIST_DIR  = dist

.PHONY: build clean release install

build:
	go build -ldflags "$(LDFLAGS)" -o pkv .

install: build
	mkdir -p $(HOME)/.local/bin
	cp pkv $(HOME)/.local/bin/pkv

clean:
	rm -rf pkv $(DIST_DIR)

release: clean
	@mkdir -p $(DIST_DIR)
	@for platform in $(PLATFORMS); do \
		os=$${platform%/*}; \
		arch=$${platform#*/}; \
		ext=""; \
		if [ "$$os" = "windows" ]; then ext=".exe"; fi; \
		output="$(DIST_DIR)/pkv_$${os}_$${arch}$${ext}"; \
		echo "Building $$output ..."; \
		GOOS=$$os GOARCH=$$arch go build -ldflags "$(LDFLAGS)" -o $$output . || exit 1; \
	done
	@echo "Release binaries in $(DIST_DIR)/"
	@ls -lh $(DIST_DIR)/
