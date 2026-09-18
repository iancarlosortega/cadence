# M2 Verification Runbook

Everything here needs a GNOME Shell that has scanned the installed extension. Wayland cannot rescan
in place, so this runs after a logout/login. Expect ~10 minutes.

**Commands are written for fish**, the interactive shell on this machine. Fish has no heredocs, so
the multi-line pieces are committed as files rather than inlined.

The daemon fix this depends on (`fix-suspend-deadline-republish`) is archived and committed, so
task 6.3 should now pass.

## 0. Optional, do this BEFORE logging out

Task 6.4 waits for the amber threshold at T-2min. With the default 50-minute focus that is a
48-minute wait. To make it about a minute:

```fish
mkdir -p ~/.config/cadence
printf '[timer]\nfocus_minutes = 3\nbreak_minutes = 1\n' > ~/.config/cadence/config.toml
systemctl --user restart cadenced
```

Restore afterwards:

```fish
rm ~/.config/cadence/config.toml; systemctl --user restart cadenced
```

## 1. After logging back in

```fish
gnome-extensions enable cadence@ian.dev
gnome-extensions info cadence@ian.dev
```

Expect `State: ENABLED` and no error. If this errors, stop — nothing below is meaningful:

```fish
journalctl --user -b -o cat | grep -i cadence
```

## 2. Task 2.5 — dimmed with no daemon

```fish
systemctl --user stop cadenced
```

Expect: indicator present in the top bar, dimmed, no countdown label.
**Fails if** the indicator is missing entirely — the spec requires presence, not hiding.

## 3. Task 6.2 — I1 under a stopped daemon

```fish
systemctl --user start cadenced; and cadence start
systemctl --user stop cadenced
```

Expect: goes dimmed and the countdown **stops advancing**.
**Fails if** it keeps counting down on its own, or announces a phase change — that is the tick
concluding something, which invariant I1 forbids.

## 4. Task 5.5 — pause round trip

```fish
systemctl --user start cadenced; and cadence start
# click the indicator, choose Pause, then:
busctl --user get-property dev.ian.Cadence /dev/ian/Cadence dev.ian.Cadence1 Paused
```

Expect: label freezes **and** `Paused` reads `true`. The freeze must come from the daemon, not from
the panel deciding locally. Then choose Resume and confirm it starts again.

## 5. Task 6.4 — amber

With the short config from step 0, start a session and watch the last 2 minutes of focus.

Expect: label and icon turn amber at 120s remaining, and return to normal on the break transition.
Check both themes (Settings → Appearance) — they use different colours, `#ffb454` on dark and
`#8f5300` on light, because the panel background differs.

## 6. Task 6.3 — I1 across suspend

```fish
cadence start
systemctl suspend
```

Resume after ~15s, then:

```fish
python3 packaging/check-offset.py
```

The script samples three times and prints its own verdict. Expect **PASS**, with a sub-second
constant offset — that residual is `PhaseEndsAt` being a whole-second value, not drift.
**Fails if** it reports an offset of a whole second or more. That was the original bug.

## 7. Task 6.5 — leak check

In one terminal:

```fish
journalctl -f -o cat /usr/bin/gnome-shell
```

In another, twice:

```fish
gnome-extensions disable cadence@ian.dev; and gnome-extensions enable cadence@ian.dev
```

Then once more with the daemon stopped, which is the path where the name watch can survive.

Expect: no errors mentioning cadence, and the indicator returns each time.

## 8. Report back

Note which tasks passed. Then `/gentle-sdd-continue` on `m2-gnome-extension` runs verify against
real evidence, and M2 can archive.
