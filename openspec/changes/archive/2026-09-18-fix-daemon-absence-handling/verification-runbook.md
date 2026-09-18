# Verification Runbook — F3/F4

Commands are **fish**. Everything here except section 3 has already been run and passed; section 3
is the part only a real session cycle can prove.

## 1. Already verified without a logout

| Check | Result |
|---|---|
| `go test ./...` in `daemon/` | all packages pass |
| `go vet ./...` | clean |
| Mutation: remove `recover` from `publish` | `TestPublishOnClosedConnectionErrorsNotPanics` fails, reporting a panic |
| Mutation: make `startupGap` always skip | `TestStartupGapReplaysTheAbsence` fails |
| Live: 20s downtime | elapsed **not** charged — remaining stayed 180s |
| Live: 70s downtime, 60s idle-credit | **fresh focus**, elapsed 0s, where the old build gave ~130s |
| Unit re-wired | symlink moved to `graphical-session.target.wants` |
| Panics since install | 0, with `NRestarts=0` |
| Extension unaffected | 17/17 render checks pass |

## 2. One-time install step

`make install` is not enough: the unit's `[Install]` section changed, so it must be re-enabled once.
Already done on this machine; included for a fresh install.

```fish
make -C packaging install-daemon
systemctl --user disable cadenced.service
systemctl --user enable cadenced.service
systemctl --user restart cadenced.service
```

Confirm:

```fish
systemctl --user show cadenced.service -p PartOf -p WantedBy
```

Expect `PartOf=graphical-session.target` and `WantedBy=graphical-session.target`.

## 3. Needs a real logout — the remaining evidence

This is the half of F3 that cannot be simulated. `systemctl --user stop` exercises the downtime rule
but not the `PartOf` propagation that stops the daemon *before* its bus disappears.

**Before logging out**, start a session so there is something to resume:

```fish
cadence start
```

Log out, wait a couple of minutes, log back in. Then:

```fish
journalctl --user -u cadenced.service -b --no-pager -o cat | grep -c "^panic:"
```

Expect **0**. Any non-zero means `publish` still panicked somewhere the recover does not cover.

```fish
journalctl --user -u cadenced.service -b -o short-iso --no-pager | tail -20
```

Expect to see the unit **stopped** cleanly at logout — a `shutting down` line, not a crash — and
**started** again at login without you doing anything.

```fish
systemctl --user show cadenced.service -p NRestarts
```

Expect `NRestarts=0`. A non-zero count means it crashed and was restarted rather than stopped.

```fish
busctl --user get-property dev.ian.Cadence /dev/ian/Cadence dev.ian.Cadence1 Phase
busctl --user get-property dev.ian.Cadence /dev/ian/Cadence dev.ian.Cadence1 RemainingSeconds
```

If you were logged out longer than the idle-credit threshold (10 minutes by default, 1 minute if you
kept a test config), expect a **fresh focus** — full remaining — not a break. That is F4 under real
conditions.

Finally, check the panel is live with no manual restart. That is the whole point of the unit change.

## 4. When you are done testing

```fish
rm ~/.config/cadence/config.toml
systemctl --user restart cadenced.service
```

Back to the 50-minute focus and 10-minute break defaults.
