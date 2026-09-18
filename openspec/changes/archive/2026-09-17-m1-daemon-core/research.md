schema: gentle-ai.sdd-research/v1
revision: 1
request_id: res-m1-daemon-core-20260914-01
change: m1-daemon-core
project: cadence
outcome: done
accessed_at: 2026-09-14

# Research: M1 — Daemon Core

## Admission

| Field | Value |
| --- | --- |
| Capability schema | `gentle-ai.sdd-research-capability/v1` |
| Requested classes | `documentation`, `open-web` |
| Observed grants | `documentation` — Context7 `resolve-library-id` + `query-docs`; `open-web` — `WebSearch` + `WebFetch` |
| Admission result | granted for both requested classes |

Both requested classes were exercised. No class was denied, so no question was dropped for lack of admission.

## Questions

| ID | Question |
| --- | --- |
| Q1 | Does Go's monotonic clock continue to advance while the machine is suspended? |
| Q2 | How can a Linux daemon reliably detect that the system is about to suspend and that it has resumed? |
| Q3 | Is "authoritative daemon + GNOME Shell extension as pure consumer over the session bus" a validated architecture for a GNOME timer application, or is it novel? |
| Q4 | What does `org.freedesktop.DBus.Properties.PropertiesChanged` guarantee, and what are the values of the `EmitsChangedSignal` annotation? |
| Q5 | Does authoritative D-Bus documentation advise against exposing rapidly-changing values as properties? |

## Sources

| ID | Class | Title | Publisher | URL | Accessed |
| --- | --- | --- | --- | --- | --- |
| S1 | documentation | `time` package — Monotonic Clocks | Go / pkg.go.dev | https://pkg.go.dev/time | 2026-09-14 |
| S2 | documentation | `org.freedesktop.login1(5)` — D-Bus interface of systemd-logind | systemd / Debian Manpages | https://manpages.debian.org/testing/systemd/org.freedesktop.login1.5.en.html | 2026-09-14 |
| S3 | open-web | FocusTimer (formerly GNOME Pomodoro) — architecture overview | DeepWiki (focustimerhq/FocusTimer) | https://deepwiki.com/focustimerhq/FocusTimer | 2026-09-14 |
| S4 | documentation | godbus/dbus — properties and export API reference | godbus/dbus (via Context7 `/godbus/dbus`) | https://github.com/godbus/dbus/blob/master/_autodocs/api-reference/properties.md | 2026-09-14 |
| S5 | documentation | `EmitsChangedSignal` enum | docs.rs — `dbus-tree` crate | https://docs.rs/dbus-tree/latest/dbus_tree/enum.EmitsChangedSignal.html | 2026-09-14 |
| S6 | documentation | D-Bus API Design Guidelines | freedesktop.org | https://dbus.freedesktop.org/doc/dbus-api-design.html | 2026-09-14 |
| S7 | open-web | Focus Timer project page | gnomepomodoro.org | https://gnomepomodoro.org/ | 2026-09-14 |

### Excerpts

**S1** — "On some systems the monotonic clock will stop if the computer goes to sleep." and "On such a system, t.Sub(u) may not accurately reflect the actual time that passed between t and u." and "The canonical way to strip a monotonic clock reading is to use t = t.Round(0)."

**S2** — Signal signature `PrepareForSleep(b start);` on the Manager object at `/org/freedesktop/login1`, interface `org.freedesktop.login1.Manager`. "The **PrepareForSleep()** signals are sent right before (with the argument "true") or after (with the argument "false") the system goes down for reboot/poweroff and suspend/hibernate, respectively." Applications should "save data on disk, release memory, or do other jobs that should be done shortly before shutdown/sleep, in conjunction with delay inhibitor locks. After completion of this work they should release their inhibition locks in order to not delay the operation any further."

**S3** — The two processes "communicate exclusively through the D-Bus session bus. Neither process embeds the other; they are independently started and can be independently restarted." "The Vala GTK application is the authoritative owner of all timer state. It runs as a `Gtk.Application` (application ID `org.gnome.Pomodoro`) and is the sole process that writes to GLib.Settings state keys and the SQLite database." The extension is "a **pure consumer** of timer state: it subscribes to the `org.gnome.Pomodoro` D-Bus interface through the `PomodoroClient` proxy and re-emits events through the local `Timer` class." The extension receives updates through D-Bus signal subscriptions rather than polling.

**S4** — `prop.Export(conn, "/org/example/Object", props)` with `prop.Prop{Value, Writable, Emit: prop.EmitTrue, Callback}`; "Creates and exports a Properties object to a specified DBus path. Use this to manage property values and emit change signals." "Now clients can use org.freedesktop.DBus.Properties.Get/Set".

**S5** — `True`: "The Property emits a signal that includes the new value." `Invalidates`: "The Property emits a signal that does not include the new value." `Const`: "The Property cannot be changed." `False`: "The Property does not emit a signal when changed."

**S6** — On `org.freedesktop.DBus.Property.EmitsChangedSignal`: "Indicate whether a property is expected to emit change signals. This can affect code generation, but is also useful documentation, as client programs then know when to expect property change notifications and when they have to requery." The document emphasises using properties with `PropertiesChanged` to reduce IPC round trips.

**S7** — Project page for Focus Timer, maintainer Kamil Prusko, repository `github.com/focustimerhq/FocusTimer`; notes the application has been ported to GTK4. The page is promotional and contains no architectural or IPC detail.

## Validated Claims

| ID | Claim | Sources |
| --- | --- | --- |
| C1 | Go's monotonic clock may stop while the machine is asleep; this is explicitly system-dependent, not guaranteed on all platforms. Therefore `t.Sub(u)` can under-report real elapsed time across a suspend. | S1 |
| C2 | Because monotonic behaviour across suspend is system-dependent, a duration that must reflect real elapsed wall time across a suspend cannot rely on a monotonic reading alone; the monotonic reading can be stripped with `t.Round(0)`. | S1 |
| C3 | `systemd-logind` emits `PrepareForSleep(b start)` on `/org/freedesktop/login1`, interface `org.freedesktop.login1.Manager`, with `true` immediately before suspend/hibernate and `false` after resume. | S2 |
| C4 | The documented purpose of `PrepareForSleep(true)` is to let applications persist state before sleep, in conjunction with delay inhibitor locks, which must be released promptly afterwards. | S2 |
| C5 | A two-process design — an authoritative timer daemon plus a GNOME Shell extension acting as a pure consumer, communicating only over the D-Bus session bus, each independently restartable — is an established architecture in a mature GNOME timer application, not a novel one. | S3 |
| C6 | In that established design, the extension receives timer updates via D-Bus signal subscription rather than polling. | S3 |
| C7 | `godbus/dbus` `prop.Export` manages property values and emits change signals, exposing the standard `org.freedesktop.DBus.Properties` `Get`/`Set` to clients, with per-property `Emit` behaviour configured via `prop.EmitTrue`. | S4 |
| C8 | The `EmitsChangedSignal` annotation has four values: `true` (signal includes the new value), `invalidates` (signal omits the value), `const` (property cannot change), and `false` (no signal on change). | S5 |
| C9 | The D-Bus API Design Guidelines present `EmitsChangedSignal` primarily as a contract telling clients when to expect notifications and when they must re-query, and frame properties plus `PropertiesChanged` as a way to reduce IPC round trips. | S6 |
| C10 | The D-Bus API Design Guidelines contain no explicit guidance about rapidly-changing properties, excessive signal traffic, or chatty interfaces. | S6 |
| C11 | The Focus Timer public project page documents no architecture, D-Bus interface, or IPC design; architectural detail for that project is not available from the project's own landing page. | S7 |

## Contradictions

None observed. S3 and S7 differ in depth, not in substance: S7 simply does not address architecture, so it neither supports nor contradicts S3. This is a coverage gap in S7, recorded as C11, not a conflict.

## Gaps

| ID | Gap | Effect on M1 |
| --- | --- | --- |
| G1 | No authoritative D-Bus source was found that advises for or against exposing a rapidly-changing value as a property (Q5 answered in the negative, C10). | The decision to publish `PhaseEndsAt` and let clients tick locally remains an engineering judgement. It is *corroborated* by C6 — an established implementation uses signal subscription rather than polling — but it is not mandated by any specification. It must be justified in the design document on its own merits, not presented as a standards requirement. |
| G2 | The `PropertiesChanged` signal's exact signature was not excerpted from the D-Bus specification itself; the direct fetch of `dbus-specification.html` returned a truncated document. Annotation semantics were taken from a binding's reference documentation (S5). | Low. The annotation values in C8 are consistent across S5 and S6, and `godbus` handles emission internally (C7), so M1 does not hand-write the signal. |
| G3 | S3 is a generated wiki rather than the upstream repository's own documentation. | Low for M1. C5/C6 are used only as corroboration that the chosen architecture is established practice; no M1 behaviour depends on the details of that other project. |

## Uncertainty and Freshness

- All sources accessed 2026-09-14.
- S1, S2, S5, S6 are living reference documentation and reflect current published versions; no version pin was captured for the systemd manpage beyond the Debian *testing* channel.
- C10 and C11 are negative findings — absence of guidance in the specific documents fetched. They are not proof that no such guidance exists anywhere in D-Bus literature. Treat them as scoped to S6 and S7.
- S3 is community-generated documentation about a third-party project and was not cross-checked against that project's source tree. Its claims are used as corroboration only.
- C1 is explicitly hedged by its own source ("on some systems"). The concrete behaviour on Fedora 42 / Linux 6.19 was **not** empirically tested in this research pass. That test is cheap and is recommended before the design phase commits to a suspend strategy.

## Impact on the M1 Open Decisions

Evidence bearing on decisions recorded in `exploration.md`:

- **D1 (countdown over the bus)** — Supported but not mandated. C6 shows signal subscription over polling is established practice; C7 confirms `godbus` emits property changes without hand-written plumbing; C8/C9 give the vocabulary for declaring the contract. G1 records that no specification forbids per-second properties, so the design must argue the case rather than cite a rule.
- **D2 (inject a Clock)** — Strengthened materially. C1 and C2 show Go's own clock semantics are system-dependent across suspend. A `Clock` port is therefore not only a testability device but the seam where suspend behaviour is defined and can be tested deterministically.
- **D3 (persistence across restart)** — Strengthened and extended. C3 and C4 supply a concrete mechanism: subscribe to `PrepareForSleep`, persist on `true`, and re-evaluate elapsed time on `false`. This is a better trigger than the 60-second heartbeat alone, though the heartbeat still covers crashes, which emit no signal.
- **D4 (TOML library)** — No external evidence gathered; the choice rests on the library-version facts already verified locally in `exploration.md`.

## Product Choices (non-authoritative)

Recorded for the orchestrator's product discovery. These are **not** evidence-backed conclusions and carry no authority:

- Whether cadence should treat a suspended machine as "time away from the desk" (crediting a break) or as a paused session is a product decision, not a technical one. The evidence above only establishes how suspend can be detected and why naive elapsed-time arithmetic is unsafe.
- Whether M1 should subscribe to `PrepareForSleep` at all, or defer suspend handling to a later milestone, is a scope decision for the proposal.
