# cadence — local install for Fedora + GNOME 48 (Wayland).
#
# Wayland cannot restart GNOME Shell in place, so a freshly installed or
# updated extension needs either a logout/login or the `nested` target.

UUID    := cadence@ian.dev
ROOT    := $(abspath $(dir $(lastword $(MAKEFILE_LIST)))/..)
PREFIX  ?= $(HOME)/.local
BINDIR  := $(PREFIX)/bin
UNITDIR := $(HOME)/.config/systemd/user
EXTDIR  := $(HOME)/.local/share/gnome-shell/extensions/$(UUID)

.PHONY: all install install-daemon install-extension uninstall nested test check-offset

all: install

install: install-daemon install-extension
	@echo "Installed. Log out and back in, then: gnome-extensions enable $(UUID)"

install-daemon:
	install -d $(BINDIR) $(UNITDIR)
	cd $(ROOT)/daemon && go build -o $(BINDIR)/cadenced ./cmd/cadenced
	cd $(ROOT)/daemon && go build -o $(BINDIR)/cadence  ./cmd/cadence
	install -m 644 $(ROOT)/packaging/cadenced.service $(UNITDIR)/cadenced.service
	systemctl --user daemon-reload

install-extension:
	install -d $(EXTDIR)
	install -m 644 $(ROOT)/extension/metadata.json   $(EXTDIR)/metadata.json
	install -m 644 $(ROOT)/extension/extension.js    $(EXTDIR)/extension.js
	install -m 644 $(ROOT)/extension/render.js       $(EXTDIR)/render.js
	install -m 644 $(ROOT)/extension/overlay.js      $(EXTDIR)/overlay.js
	install -m 644 $(ROOT)/extension/stylesheet.css  $(EXTDIR)/stylesheet.css

uninstall:
	-systemctl --user disable --now cadenced.service
	rm -f $(BINDIR)/cadenced $(BINDIR)/cadence
	rm -f $(UNITDIR)/cadenced.service
	rm -rf $(EXTDIR)
	systemctl --user daemon-reload

# Sample the client-derived countdown against the daemon's elapsed-derived truth.
# Needs an active unpaused session. Prints its own PASS/FAIL verdict.
check-offset:
	python3 $(ROOT)/packaging/check-offset.py

# Pure-logic checks for the render function. Runs outside the Shell.
test:
	cd $(ROOT)/daemon && go test ./...
	gjs -m $(ROOT)/extension/test-render.js

# Iterate without logging out. The nested Shell gets its own D-Bus session,
# so start a cadenced inside it if you want live state.
nested:
	dbus-run-session -- gnome-shell --nested --wayland
