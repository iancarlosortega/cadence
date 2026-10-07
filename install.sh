#!/usr/bin/env bash
# cadence installer for GNOME on Wayland.
#
#   ./install.sh              build and install the daemon, CLI and extension
#   ./install.sh --uninstall  remove everything install.sh put in place
#
# Installs into your home directory only. sudo is used solely to install
# missing build dependencies with dnf, and only after asking.

set -euo pipefail

UUID="cadence@ian.dev"
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
GO_MIN_MINOR=21 # older Go cannot fetch the 1.26 toolchain go.mod asks for

info() { printf '\033[1;34m==>\033[0m %s\n' "$*"; }
warn() { printf '\033[1;33mwarning:\033[0m %s\n' "$*" >&2; }
die()  { printf '\033[1;31merror:\033[0m %s\n' "$*" >&2; exit 1; }

confirm() {
  local reply
  read -r -p "$1 [Y/n] " reply
  [[ -z "$reply" || "$reply" =~ ^[Yy] ]]
}

supported_shell_versions() {
  grep -o '"shell-version"[^]]*' "$ROOT/extension/metadata.json" | grep -o '[0-9]\+'
}

check_environment() {
  [[ $EUID -ne 0 ]] || die "run as your normal user, not root: everything installs into \$HOME"

  command -v gnome-shell >/dev/null || die "GNOME Shell not found; cadence only runs on GNOME"

  local major
  major="$(gnome-shell --version | grep -o '[0-9]\+' | head -n1)"
  if ! supported_shell_versions | grep -qx "$major"; then
    die "GNOME Shell $major is not supported (supported: $(supported_shell_versions | tr '\n' ' '))"
  fi

  if [[ "${XDG_SESSION_TYPE:-}" != "wayland" ]]; then
    warn "session type is '${XDG_SESSION_TYPE:-unknown}', not wayland; cadence is only tested on Wayland"
  fi
}

go_is_usable() {
  command -v go >/dev/null || return 1
  local minor
  minor="$(go env GOVERSION | sed -E 's/^go1\.([0-9]+).*/\1/')"
  [[ "$minor" =~ ^[0-9]+$ ]] && (( minor >= GO_MIN_MINOR ))
}

install_dependencies() {
  local missing=()
  go_is_usable                  || missing+=(golang)
  command -v make    >/dev/null || missing+=(make)
  command -v pw-dump >/dev/null || missing+=(pipewire-utils)

  (( ${#missing[@]} == 0 )) && return

  if ! command -v dnf >/dev/null; then
    die "missing: ${missing[*]}. Install them with your package manager (Go >= 1.$GO_MIN_MINOR, make, pw-dump) and re-run."
  fi

  info "Missing dependencies: ${missing[*]}"
  confirm "Install them with 'sudo dnf install ${missing[*]}'?" || die "dependencies are required"
  sudo dnf install -y "${missing[@]}"
}

enable_extension() {
  # gnome-extensions enable only works once the Shell has loaded the extension,
  # which on Wayland needs a new login. Writing the setting directly makes the
  # Shell enable it on that next login.
  local current
  current="$(gsettings get org.gnome.shell enabled-extensions)"
  if [[ "$current" != *"'$UUID'"* ]]; then
    if [[ "$current" == "@as []" || "$current" == "[]" ]]; then
      gsettings set org.gnome.shell enabled-extensions "['$UUID']"
    else
      gsettings set org.gnome.shell enabled-extensions "${current%]}, '$UUID']"
    fi
  fi

  if [[ "$(gsettings get org.gnome.shell disable-user-extensions)" == "true" ]]; then
    warn "user extensions are disabled globally; enable them with:"
    warn "  gsettings set org.gnome.shell disable-user-extensions false"
  fi
}

disable_extension() {
  local current updated
  current="$(gsettings get org.gnome.shell enabled-extensions)"
  updated="$(sed -E "s/, '$UUID'|'$UUID', |'$UUID'//" <<<"$current")"
  [[ "$updated" == "[]" ]] && updated="@as []"
  [[ "$updated" == "$current" ]] || gsettings set org.gnome.shell enabled-extensions "$updated"
}

do_install() {
  check_environment
  install_dependencies

  info "Building and installing"
  make -C "$ROOT/packaging" install-daemon install-extension

  info "Starting the daemon (and on every login)"
  systemctl --user enable --now cadenced.service

  info "Enabling the extension for your next login"
  enable_extension

  if [[ ":$PATH:" != *":$HOME/.local/bin:"* ]]; then
    warn "~/.local/bin is not on your PATH; add it to use the 'cadence' command"
  fi

  cat <<EOF

cadence is installed.

  1. Log out and log back in (Wayland loads new extensions only at login).
  2. Run:  cadence start

Config (optional): ~/.config/cadence/config.toml — see README.md.
EOF
}

do_uninstall() {
  info "Removing cadence"
  disable_extension
  make -C "$ROOT/packaging" uninstall
  info "Done. Config and state are kept in ~/.config/cadence and ~/.local/state/cadence."
}

case "${1:-}" in
  "")            do_install ;;
  --uninstall)   do_uninstall ;;
  -h|--help)     sed -n '2,8p' "$0" | sed 's/^# \{0,1\}//' ;;
  *)             die "unknown option: $1 (try --help)" ;;
esac
