PREFIX ?= /usr/local
BINDIR ?= $(PREFIX)/bin
SUDO ?= sudo

build:
	go build -o wo -ldflags="-s -w" main.go

build-bin:
	mkdir -p bin
	go build -o bin/wo -ldflags="-s -w" main.go

run:
	go run main.go

install: build
	$(SUDO) mkdir -p $(BINDIR)
	$(SUDO) install -m 755 wo $(BINDIR)/wo

uninstall:
	$(SUDO) rm -f $(BINDIR)/wo

.PHONY: build build-bin run install uninstall
