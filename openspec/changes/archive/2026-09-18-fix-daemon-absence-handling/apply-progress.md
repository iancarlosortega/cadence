# Apply Progress: Handling absences the daemon did not witness

Status: **20/21 complete.** Everything provable without a session cycle is proven. Task 5.10 — that
logout stops the unit rather than crashing it — needs a real logout and is left open deliberately.

## Change

| File | Diff |
|------|------|
| `daemon/internal/session/machine_test.go` | +61 |
| `daemon/internal/dbusapi/service_test.go` | +30 |
| `daemon/internal/dbusapi/service.go` | +21 / −2 |
| `daemon/cmd/cadenced/main.go` | +17 |
| `daemon/cmd/cadenced/main_test.go` | new, +38 |
| `packaging/cadenced.service` | +2 / −1 |

129 changed lines against a 250 budget. `internal/session/machine.go`, `internal/store/` and
`extension/` untouched.

## Deviation from design — Decision 1 was wrong and was corrected

The design said to swap `SetMust` for `prop.Properties.Set`, and explicitly rejected `recover()`.
Implementing it broke `New()` at once:

```
dbusapi: seed properties: publish SessionActive:
org.freedesktop.DBus.Properties.Error.ReadOnly
```

Reading `prop.go` shows why, and closes the option:

| API | Behaviour |
|-----|-----------|
| `Set` | Exported, returns `ErrReadOnly` unless `Writable` — false for all six properties, deliberately |
| `set` | Non-panicking internal setter — **unexported** |
| `SetMust` | `set` plus `panic(err)` |

There is no exported non-panicking internal write, and making the properties writable would let any
bus client set them — a worse defect than the one being fixed. So `publish()` keeps `SetMust` and
converts the panic at the boundary:

```go
func (s *Service) publish() (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("publish: %v", r)
		}
	}()
	...
}
```

The design has been amended. The original rejection of `recover` stands for a blanket recover around
unknown code; it simply did not apply to a narrow one at a boundary where a third-party API panics by
contract.

## Correction — task 5.2's mutation check was invalid

As written it said: remove the startup `ApplySuspend`, expect tests 4.1 and 4.3 to go red. Running it
revealed they do **not** — those tests call `Apply` directly and never exercise `main.go`'s wiring.
The rule was tested; the wiring was not.

Fixed by extracting `startupGap(initial, found, now)` into a testable helper with its own tests.
Mutating it to always skip now fails `TestStartupGapReplaysTheAbsence`. The mutation check found a
hole in its own test plan, which is the argument for running them rather than asserting them.

## Also corrected — a tautological test

`TestDowntimeAndSuspendAgree` was first written comparing `Apply` with the *same* event on both
sides. It compared a value to itself and could never fail. Rewritten to contrast the suspend path
against the tick path and assert they still differ, which pins why the startup indirection exists:

```
8h absence: suspend path -> focus, tick path -> break
```

If someone later teaches `applyTick` the idle-credit rule, that test tells them to delete the
indirection rather than leaving both.

## Evidence

- `go test ./...` in `daemon/` — all five packages pass. `go vet` clean.
- **Mutation, F3**: removing `recover` fails the publish test with "publish panicked on a closed
  connection instead of returning an error".
- **Mutation, F4**: making `startupGap` always skip fails `TestStartupGapReplaysTheAbsence`.
- **Live, short downtime**: 20s stop, remaining stayed **180s** — not charged. The old build would
  have shown ~148s.
- **Live, long downtime**: persisted elapsed 60s, stopped 70s against a 60s idle-credit, resumed at
  **elapsed 0s — a fresh focus**. The old build would have shown ~130s elapsed.
- **Unit re-wiring**: symlink moved from `default.target.wants` to `graphical-session.target.wants`;
  `PartOf` and `WantedBy` both report `graphical-session.target`.
- **Zero panics** since install, with `NRestarts=0` across every stop/start above.
- `gjs -m extension/test-render.js` — 17/17, extension unaffected.

## Left open

**5.10** — confirm a real logout stops the unit cleanly, no panic, and login restarts it with the
panel live. `systemctl --user stop` exercises the downtime rule but not the `PartOf` propagation,
which is the half of F3 that stops the daemon before its bus disappears. Runbook section 3 has the
commands and expected results.

## Observed, not touched

- `daemon/internal/session/state.go` and `internal/dbusapi/service_test.go` both fail `gofmt -l` on
  pre-existing comment-alignment nits, unrelated to this change.
