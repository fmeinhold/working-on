PREFIX ?= /usr/local
BINDIR ?= $(PREFIX)/bin
SUDO ?= sudo
OUT ?= ./bin

build:
	go build -o $(OUT)/wo -ldflags="-s -w" main.go

run:
	go run main.go

install: build
	$(SUDO) mkdir -p $(BINDIR)
	$(SUDO) install -m 755 wo $(BINDIR)/wo

uninstall:
	$(SUDO) rm -f $(BINDIR)/wo

.PHONY: build run install uninstall
