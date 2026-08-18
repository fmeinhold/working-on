PREFIX ?= /usr/local
BINDIR ?= $(PREFIX)/bin
SUDO ?= sudo
OUT ?= ./bin

build:
	go build -o $(OUT)/wo -ldflags="-s -w" main.go

run:
	go run main.go

test:
	go test ./...

install: build
	$(SUDO) mkdir -p $(BINDIR)
	$(SUDO) install -m 755 wo $(BINDIR)/wo

uninstall:
	$(SUDO) rm -f $(BINDIR)/wo

# Releasing
# ---------
# The version lives in the git tag: goreleaser reads it from there and stamps
# it into the binary, so there is no file to bump and cutting a release is
# tagging one. Pushing the tag is what the release workflow waits for.
#
#   make version           what the last release was
#   make release-patch     v0.3.1 -> v0.3.2
#   make release-minor     v0.3.1 -> v0.4.0
#   make release-major     v0.3.1 -> v1.0.0
#   make release VERSION=v1.2.3
#
# Each one checks the tree is clean, that you are on main and in step with the
# remote, and that the tests pass, then asks before pushing. YES=1 answers for
# an unattended run.

VERSION ?= patch

version:
	@git tag --list 'v*' --sort=-v:refname | head -n 1 | grep . || echo "nothing released yet"

release:
	@scripts/release.sh "$(VERSION)"

release-patch:
	@scripts/release.sh patch

release-minor:
	@scripts/release.sh minor

release-major:
	@scripts/release.sh major

.PHONY: build run test install uninstall version release release-patch release-minor release-major
