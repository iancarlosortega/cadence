# M2 Verification Runbook

Everything here needs a GNOME Shell that has scanned the installed extension. Wayland cannot rescan
in place, so this runs after a logout/login. Expect ~10 minutes.

The daemon fix this depends on (`fix-suspend-deadline-republish`) is archived and committed, so
task 6.3 should now pass.

## 0. Optional, do this BEFORE logging out

Task 6.4 waits for the amber threshold at T-2min. With the default 50-minute focus that is a
48-minute wait. To make it ~1 minute, write a temporary config:

```sh
mkdir -p ~/.config/cadence
cat > ~/.config/cadence/config.toml <<'EOF'
[timer]
focus_minutes = 3
break_minutes = 1
EOF
systemctl --user restart cadenced
```

Restore afterwards with `rm ~/.config/cadence/config.toml && systemctl --user restart cadenced`.

## 1. After logging back in

```sh
gnome-extensions enable cadence@ian.dev
gnome-extensions info cadence@ian.dev     # expect State: ENABLED, no error
```

If this errors, stop — nothing below is meaningful. Check `journalctl --user -b -o cat | grep -i cadence`.

## 2. Task 2.5 — dimmed with no daemon

```sh
systemctl --user stop cadenced
```

Expect: indicator present in the top bar, dimmed, no countdown label.
**Fails if** the indicator is missing entirely — the spec requires presence, not hiding.

## 3. Task 6.2 — I1 under a stopped daemon

```sh
systemctl --user start cadenced && cadence start    # countdown appears and runs
systemctl --user stop cadenced                      # watch the indicator
```

Expect: goes dimmed and the countdown **stops advancing**.
**Fails if** it keeps counting down on its own, or announces a phase change — that is the tick
concluding something, which invariant I1 forbids.

## 4. Task 5.5 — pause round trip

```sh
systemctl --user start cadenced && cadence start
# click the indicator, choose Pause
busctl --user get-property dev.ian.Cadence /dev/ian/Cadence dev.ian.Cadence1 Paused
```

Expect: label freezes **and** `Paused` reads `true`. The freeze must come from the daemon, not from
the panel deciding locally. Then choose Resume and confirm it starts again.

## 5. Task 6.4 — amber

With the short config from step 0, start a session and watch the last 2 minutes of focus.

Expect: label and icon turn amber at 120s remaining, and return to normal on the break transition.
Check legibility in both light and dark (Settings → Appearance) — they use different colours
(`#ffb454` dark, `#8f5300` light) because the panel background differs.

## 6. Task 6.3 — I1 across suspend

```sh
cadence start
systemctl suspend      # resume after ~15s
```

Then run the sampler:

```sh
python3 - <<'EOF'
import json, subprocess, time, datetime
def prop(n):
    return subprocess.run(['busctl','--user','get-property','dev.ian.Cadence',
        '/dev/ian/Cadence','dev.ian.Cadence1',n],capture_output=True,text=True).stdout.split()[-1].strip('"')
focus = float(prop('RemainingSeconds')) and None
s0 = json.load(open('/home/ian/.local/state/cadence/session.json'))
FOCUS = s0['focus_minutes'] * 60.0
for i in range(3):
    now = time.time(); ends = float(prop('PhaseEndsAt'))
    s = json.load(open('/home/ian/.local/state/cadence/session.json'))
    lo = datetime.datetime.fromisoformat(s['last_observed']).timestamp()
    true_rem = FOCUS - (s['elapsed_in_phase_ns']/1e9 + (now - lo))
    print(f"sample {i+1}: offset={true_rem-(ends-now):+.3f}s")
    if i < 2: time.sleep(15)
EOF
```

Expect: offset under 1 second and constant — that residual is `PhaseEndsAt` being a whole-second
value, not drift.
**Fails if** the offset is a whole second or more and constant. That was the original bug.

## 7. Task 6.5 — leak check

In one terminal:

```sh
journalctl -f -o cat /usr/bin/gnome-shell
```

In another, twice:

```sh
gnome-extensions disable cadence@ian.dev && gnome-extensions enable cadence@ian.dev
```

Then once more with the daemon stopped, which is the path where the name watch can survive.

Expect: no errors mentioning cadence, and the indicator returns each time.

## 8. Report back

Note which tasks passed. Then `/gentle-sdd-continue` on `m2-gnome-extension` runs verify against
real evidence, and M2 can archive.
