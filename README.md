# cadence

A stand-up / break reminder for GNOME on Wayland. A small Go daemon keeps the
timer; a GNOME Shell extension shows the countdown in the top panel and puts up
a break overlay when it is time to stand up.

- **Focus / break cycle**: 50 min focus, 10 min break by default (configurable).
- **Idle aware**: pauses after 3 min without input; a long enough absence counts
  as the break.
- **Presentation aware**: while you share your screen the break is skipped
  silently, so it never appears in front of an audience.
- **Camera aware**: while your camera is on, the break waits and a small corner
  panel asks instead of covering the screen. After a few prompts it shrinks to a
  quiet pill until the camera turns off or you skip.
- **Live config**: edit `~/.config/cadence/config.toml` and the change applies
  within 5 seconds, without a restart.

## Requirements

| | |
|---|---|
| OS | Linux with GNOME Shell **48** on **Wayland** (developed on Fedora 42) |
| Go | 1.26 or newer, to build the daemon |
| PipeWire | `pw-dump` on `PATH` (microphone and camera detection) |
| systemd | user session (`systemctl --user`) |

## Install

```bash
git clone https://github.com/iancarlosortega/cadence.git
cd cadence
./install.sh
```

Then **log out and log back in** and run `cadence start`. The countdown appears
in the top panel.

The script:

1. Checks you are on a supported GNOME Shell version (and warns if not on Wayland).
2. Installs missing build dependencies (`golang`, `make`, `pipewire-utils`) with
   `sudo dnf`, after asking. On other distributions it lists what to install.
3. Builds and installs the daemon, CLI, systemd user unit and extension.
4. Enables and starts `cadenced.service`, so it also runs on every login.
5. Enables the extension for your next login.

The logout is needed because Wayland cannot reload GNOME Shell in place, so the
Shell only discovers a new or updated extension at login.

<details>
<summary>Manual install (what the script does)</summary>

```bash
sudo dnf install golang make pipewire-utils
make -C packaging install
systemctl --user enable --now cadenced.service
# log out and back in, then:
gnome-extensions enable cadence@ian.dev
cadence start
```

</details>

### What gets installed

| File | Location |
|---|---|
| `cadenced` (daemon), `cadence` (CLI) | `~/.local/bin/` |
| `cadenced.service` | `~/.config/systemd/user/` |
| Extension | `~/.local/share/gnome-shell/extensions/cadence@ian.dev/` |
| Config (optional, you create it) | `~/.config/cadence/config.toml` |
| Session state | `~/.local/state/cadence/session.json` |

Everything lives in your home directory; root is only used to install packages.
Make sure `~/.local/bin` is on your `PATH` to use the `cadence` CLI.

## Usage

```bash
cadence start    # begin a session
cadence status   # current phase and time left
cadence pause    # freeze the timer
cadence resume
cadence skip     # end the current break
cadence stop     # end the session
```

## Configuration

Optional. Create `~/.config/cadence/config.toml`; every key may be omitted.

```toml
[timer]
focus_minutes = 50
break_minutes = 10

[idle]
pause_after_minutes = 3          # no input for this long pauses the timer
credit_break_after_minutes = 10  # an absence this long counts as the break

[camera]
prompt_every_minutes = 5  # minutes between prompts while the camera holds a break
prompt_limit = 3          # prompts before the hold becomes a quiet pill
```

Values must be positive whole minutes; unknown keys are rejected. Saved changes
apply to the running session within 5 seconds. An invalid file is ignored at
runtime (the last good config stays active and the problem is logged), but it
stops the daemon from starting, so check the logs if the panel goes blank after
a reboot.

## Things to know before installing

- **GNOME 48 only.** `extension/metadata.json` declares `"shell-version": ["48"]`.
  GNOME 49 (Fedora 43) and later will refuse to load it until that list is
  updated and the extension is tested there. Check with `gnome-shell --version`.
- **Wayland, not X11.** Idle detection uses Mutter's idle monitor and screen
  sharing detection uses Mutter's ScreenCast D-Bus service; both are GNOME/Mutter
  specific. Other desktops (KDE, Sway, Hyprland) are not supported.
- **Go version.** The daemon needs Go 1.26. If your distribution ships an older
  `golang`, the Go toolchain downloads 1.26 automatically on first build (needs
  network access), or install a newer Go from <https://go.dev/dl/>.
- **Camera detection** looks for any process holding a `/dev/video*` device open
  and for PipeWire camera nodes. A camera kept open in the background by some
  other app will hold breaks as if you were on a call.
- **Screen sharing** detection covers any Mutter screencast, including GNOME's
  built-in screen recorder and OBS. Breaks are skipped while recording.
- **Locked screen.** Locking the screen counts as idle, so the timer pauses.
- **Updating.** `git pull && ./install.sh`, then
  `systemctl --user restart cadenced.service` and log out and back in for
  extension changes.
- **Local extension.** It is not published on extensions.gnome.org. If user
  extensions are disabled globally, enable them with
  `gsettings set org.gnome.shell disable-user-extensions false`.

## Troubleshooting

```bash
systemctl --user status cadenced.service    # is the daemon running?
journalctl --user -u cadenced.service -f    # daemon logs (config errors appear here)
gnome-extensions info cadence@ian.dev       # is the extension enabled / in error?
journalctl --user -f /usr/bin/gnome-shell   # extension errors
```

The daemon exposes its state on the session bus as `dev.ian.Cadence`; see
[`daemon/README.md`](daemon/README.md) for the D-Bus interface.

## Uninstall

```bash
./install.sh --uninstall
rm -rf ~/.config/cadence ~/.local/state/cadence   # optional: config and state
```

## Development

```bash
make -C packaging test     # Go tests + extension render tests (needs gjs)
make -C packaging nested   # run a nested GNOME Shell to try the extension without logging out
```

Architecture: the daemon (`daemon/`) owns all timer state and is the only source
of truth; the extension (`extension/`) and the CLI are thin clients over D-Bus.
Specifications live in `openspec/specs/`.

## License

[MIT](LICENSE)
