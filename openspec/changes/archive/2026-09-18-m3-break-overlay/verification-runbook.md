# Verification Runbook — M3 break overlay

Commands are **fish**. Sections 1 and 2 are already done. Section 3 needs a logout, because the
Shell cannot reload an extension in place on Wayland.

## 1. Already verified without a Shell

| Check | Result |
|---|---|
| `gjs -m extension/test-render.js` | 28/28 pass, including 10 overlay-predicate cases |
| Mutation: predicate ignores `suppressed` | `suppressed break shows nothing` fails |
| Mutation: predicate ignores `available` | `unavailable alone is enough to hide` fails — the F3 path |
| Mutation: `computeDisplay` ignores `available` | `unavailable alone is enough to dim` fails |
| `node --check` on all four JS files | parse clean |
| `git diff --stat -- daemon/` | empty — consumer-only |
| `go test ./...` in `daemon/` | five packages green |
| Installed files match source | all five identical |

## 2. Before logging out

**Install first.** Editing `extension/` changes nothing a running Shell can see, and a logout spent
on a stale build is a wasted logout — this happened once already:

```fish
make -C packaging install-extension
diff -q ~/.local/share/gnome-shell/extensions/cadence@ian.dev/overlay.js extension/overlay.js
```

The `diff` must print nothing. Then make breaks arrive quickly:

```fish
mkdir -p ~/.config/cadence
printf '[timer]\nfocus_minutes = 3\nbreak_minutes = 2\n' > ~/.config/cadence/config.toml
systemctl --user restart cadenced
cadence start
```

A 2-minute break gives time to try hold-to-skip without rushing.

## 3. After logging back in

```fish
gnome-extensions info cadence@ian.dev
```

Expect `State: ACTIVE`. If not, stop and check:

```fish
journalctl --user -b -o cat | grep -i cadence
```

### 3.1 — run this one FIRST (task 6.11, the F3 check)

If a vanished daemon can leave the screen covered, nothing else matters.

Wait for a break so the overlay is up, then:

```fish
systemctl --user stop cadenced
```

Expect: **the overlay disappears on its own** and the screen is usable with no further action.
Then bring it back:

```fish
systemctl --user start cadenced
```

### 3.2 — coverage and input (6.6, 6.7)

With the overlay up:

- It covers the whole primary monitor, above all windows.
- Clicking where a window is does **not** reach that window.
- **Alt+Tab still switches windows.** This is deliberate, not a defect — it is product decision P1.
  If it ever stops working, that is a regression.

### 3.3 — hold to skip (6.8, 6.9)

Press and hold the skip control. The progress bar fills over 3 seconds.

- **Release early**: no skip, the bar resets, the overlay stays.
- **Hold to completion**, then check the daemon agreed:

```fish
busctl --user get-property dev.ian.Cadence /dev/ian/Cadence dev.ian.Cadence1 Phase
```

Expect `s "focus"` and the overlay gone. The overlay must dismiss because the *daemon* moved to
focus, not because the button decided to.

### 3.4 — natural end (6.10)

Let a break run out. The overlay dismisses on its own.

### 3.5 — fullscreen suppression (6.12)

Put something fullscreen on the primary monitor and leave it there across a break boundary.

Expect: **no overlay for that break**, and the break still runs and completes — check the panel
countdown keeps moving and returns to focus afterwards.

### 3.6 — teardown (6.13)

With the overlay up, in another terminal or a tty:

```fish
gnome-extensions disable cadence@ian.dev; and gnome-extensions enable cadence@ian.dev
```

Expect: the overlay is removed, the indicator returns, and no cadence errors in:

```fish
journalctl --user -b -o cat | grep -i cadence
```

## 4. When done testing

```fish
rm ~/.config/cadence/config.toml; systemctl --user restart cadenced
```
