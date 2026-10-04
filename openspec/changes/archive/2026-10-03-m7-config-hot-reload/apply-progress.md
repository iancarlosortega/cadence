# Apply progress: m7-config-hot-reload (Phases 1-7)

Status: Phases 1-7 complete; Phase 8 (live verification) not started, by instruction.

## Tasks
1.1-1.2 config validation and `[camera]`; 2.1-2.3 policy into `Durations`; 3.1-3.2 `EventConfigChanged`;
4.1-4.2 `Watcher`; 5.1-5.3 `Config` property, `ApplyConfig`, main loop; 6.1 README; 7.1-7.2 gates. All `[x]`.

## Files (changed lines, from `git diff --stat`; new files by `wc -l`)
- daemon/internal/config/config.go +73/-: `Defaults()`, `[camera]` schema, `meta.IsDefined` validation
- daemon/internal/config/config_test.go +111
- daemon/internal/config/watch.go 78 (new), watch_test.go 158 (new)
- daemon/internal/session/{state,machine,event}.go: policy in `Durations`, `applyConfig`, `EventConfigChanged`
- daemon/internal/session/machine_test.go +175: one `testDurations()` helper now carries the prompt defaults
- daemon/internal/dbusapi/service.go +38, service_test.go +77
- daemon/cmd/cadenced/main.go +31, daemon/README.md +32
- Tracked diff: 555 insertions, 60 deletions; plus 236 new lines.

## Verification
- `go vet ./...`: clean
- `go test -count=1 -race ./...`: all 6 packages ok
- `go test -count=1 -v ./... | rg -c -- '--- SKIP'`: no match (0 skips)
- `gofmt -l daemon`: empty

## Red evidence
- 1.1: with a stub `Defaults()` and the new `Durations` fields, the zero cases (all of `timer.*`, `idle.*`,
  `camera.prompt_every_minutes`), the message-format test, defaults, camera load and unknown camera key failed.
  `camera.prompt_limit` cases passed only because the unknown `[camera]` key was rejected, naming it.
- 3.1: 6 of 7 new domain tests failed before `applyConfig` (equal-config passed trivially).
- 5.1: after adding `Config`/`ApplyConfig` with the old `==` diff, `StartSession` failed with
  `publish: runtime error: comparing uncomparable type map[string]int32`; green after `reflect.DeepEqual`.
- 2.3 was green on first run: 2.2 had already moved the policy into `Durations`.

## Deviations
- D3 sketches an unexported `fileID` and `NewWatcher(path, active fileID)`. main cannot name an unexported
  type, so it is exported as `FileID` with `Identify(path)`; main takes the identity before `Load`.
- `Load` returns `Defaults()` (not a partial overlay) alongside a validation error.
- `FileID` carries a `failed` flag so a non-ENOENT stat error is reported once rather than every tick.
- The store record is unchanged (D7), as designed; resume overwrites the whole `Durations` from config.

## Open items
- Phase 8 live checks (8.1-8.7) remain.
- A literal `0` in an existing config now fails startup, called out in the README; mention it in the commit.
