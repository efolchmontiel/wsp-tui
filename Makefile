.PHONY: build run tidy test install

APP ?= wsp-tui
PREFIX ?= $(HOME)/.local
BINDIR ?= $(PREFIX)/bin

build:
	go build -o $(APP) ./cmd/whatstui
	@# keep classic aliases for local runs
	cp -f $(APP) whatstui 2>/dev/null || cp -f $(APP).exe whatstui.exe 2>/dev/null || true

run: build
	./$(APP)

tidy:
	go mod tidy

test:
	go test ./...

# Installs wsp-tui, wstui and whatstui into $(BINDIR)
install: build
	mkdir -p "$(BINDIR)"
	install -m 755 $(APP) "$(BINDIR)/wsp-tui"
	install -m 755 $(APP) "$(BINDIR)/wstui"
	install -m 755 $(APP) "$(BINDIR)/whatstui"
	@echo "Installed: $(BINDIR)/wsp-tui"
	@echo "Aliases: wstui, whatstui"
	@echo "Run: wsp-tui"
