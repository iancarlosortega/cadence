// Package dbusapi is the D-Bus adapter: it exposes session.State over the
// session bus and drives session.Apply from method calls and internal
// ticks. It is the only package in the daemon that imports godbus.
package dbusapi

import (
	"fmt"
	"reflect"
	"sync"
	"time"

	"github.com/godbus/dbus/v5"
	"github.com/godbus/dbus/v5/introspect"
	"github.com/godbus/dbus/v5/prop"

	"cadence/daemon/internal/session"
)

// Well-known identity. Durable across daemon, extension, and packaging —
// see design.md, "Bus name is durable across components".
const (
	BusName       = "dev.ian.Cadence"
	ObjectPath    = dbus.ObjectPath("/dev/ian/Cadence")
	InterfaceName = "dev.ian.Cadence1"
)

// Service exports the domain over D-Bus. It owns the only mutable copy of
// session.State outside of Apply's pure transformation.
type Service struct {
	mu    sync.Mutex
	conn  *dbus.Conn
	props *prop.Properties

	// published mirrors the values last announced on the bus, so publish
	// can emit only what actually changed. Nil until the first publish.
	published map[string]dbus.Variant

	state session.State
	clock session.Clock
	store session.Store
	tier  session.TierSource
	idle  session.IdleSource
}

// New wires a Service to conn at the well-known name, path, and interface,
// seeded with initial (either freshly loaded from config, or resumed from
// Store.Load). It requests the bus name and exports methods and
// properties. Callers still need to run Serve/Tick loops separately
// (cmd/cadenced).
func New(conn *dbus.Conn, initial session.State, store session.Store, clock session.Clock, tier session.TierSource, idle session.IdleSource) (*Service, error) {
	return newService(conn, BusName, initial, store, clock, tier, idle)
}

// newService is New with the well-known name as a parameter. busName may be
// empty, in which case the service is exported and reachable on conn's unique
// name but claims no well-known name.
//
// That empty case exists for the tests. They used to export under the
// production BusName and call t.Skip when it was already held — which a
// running cadenced always does — so `go test ./...` reported ok while
// silently skipping every D-Bus test on a normal developer machine. Two real
// defects shipped behind that green result (F9).
func newService(conn *dbus.Conn, busName string, initial session.State, store session.Store, clock session.Clock, tier session.TierSource, idle session.IdleSource) (*Service, error) {
	s := &Service{
		conn:  conn,
		state: initial,
		clock: clock,
		store: store,
		tier:  tier,
		idle:  idle,
	}

	methods := map[string]any{
		"StartSession": s.StartSession,
		"StopSession":  s.StopSession,
		"Pause":        s.Pause,
		"Resume":       s.Resume,
		"SkipBreak":    s.SkipBreak,
	}
	// ExportMethodTable, not Export: it exposes exactly this table, never
	// any other exported Go method (such as the internal Tick below) that
	// happens to have a D-Bus-shaped signature.
	if err := conn.ExportMethodTable(methods, ObjectPath, InterfaceName); err != nil {
		return nil, fmt.Errorf("dbusapi: export methods: %w", err)
	}

	// Emit is EmitFalse on every property: prop.Properties emits one
	// PropertiesChanged per Set with a single-entry map and no old/new
	// comparison (godbus/dbus/v5/prop, set -> emitChange). That shape
	// violates specs/daemon-control "An idle window emits exactly twice",
	// which requires one signal per boundary. publish batches the changed
	// properties into a single Emit instead; prop.Properties remains the
	// store that answers Get/GetAll.
	propsMap := prop.Map{
		InterfaceName: {
			"SessionActive":    {Value: initial.Active, Writable: false, Emit: prop.EmitFalse},
			"Phase":            {Value: string(initial.Phase), Writable: false, Emit: prop.EmitFalse},
			"PhaseEndsAt":      {Value: int64(0), Writable: false, Emit: prop.EmitFalse},
			"RemainingSeconds": {Value: int64(0), Writable: false, Emit: prop.EmitFalse},
			"Paused":           {Value: initial.Paused, Writable: false, Emit: prop.EmitFalse},
			"Tier":             {Value: string(initial.Tier), Writable: false, Emit: prop.EmitFalse},
			"Idle":             {Value: initial.Idle, Writable: false, Emit: prop.EmitFalse},
			"Hold":             {Value: holdString(initial), Writable: false, Emit: prop.EmitFalse},
			"Prompts":          {Value: int32(initial.Prompts), Writable: false, Emit: prop.EmitFalse},
			// a{si}, present from the first connection with every key
			// (specs/daemon-control, "Configuration Publication").
			"Config": {Value: configMap(initial.Durations), Writable: false, Emit: prop.EmitFalse},
		},
	}
	props, err := prop.Export(conn, ObjectPath, propsMap)
	if err != nil {
		return nil, fmt.Errorf("dbusapi: export properties: %w", err)
	}
	s.props = props

	// Introspection is not automatic for ExportMethodTable/prop.Export —
	// verified empirically: without this block, `busctl --user
	// introspect` fails with "does not implement ... Introspectable".
	// Method args are omitted deliberately: all five methods take no
	// input and return only a possible *dbus.Error (no D-Bus out args).
	node := &introspect.Node{
		Name: string(ObjectPath),
		Interfaces: []introspect.Interface{
			introspect.IntrospectData,
			prop.IntrospectData,
			{
				Name: InterfaceName,
				Methods: []introspect.Method{
					{Name: "StartSession"},
					{Name: "StopSession"},
					{Name: "Pause"},
					{Name: "Resume"},
					{Name: "SkipBreak"},
				},
				Properties: props.Introspection(InterfaceName),
			},
		},
	}
	if err := conn.Export(introspect.NewIntrospectable(node), ObjectPath, "org.freedesktop.DBus.Introspectable"); err != nil {
		return nil, fmt.Errorf("dbusapi: export introspection: %w", err)
	}

	if busName != "" {
		reply, err := conn.RequestName(busName, dbus.NameFlagDoNotQueue)
		if err != nil {
			return nil, fmt.Errorf("dbusapi: request name %s: %w", busName, err)
		}
		if reply != dbus.RequestNameReplyPrimaryOwner {
			return nil, fmt.Errorf("dbusapi: name %s already owned (reply %d) — is cadenced already running?", busName, reply)
		}
	}

	if err := s.publish(); err != nil { // seed properties from the actual initial state
		return nil, fmt.Errorf("dbusapi: seed properties: %w", err)
	}
	return s, nil
}

// D-Bus methods. Each applies one event to the domain and performs the
// returned effects while holding the lock, so a concurrent method call or
// Tick never interleaves with an in-flight transition.

func (s *Service) StartSession() *dbus.Error { return s.apply(session.EventStartSession{}) }
func (s *Service) StopSession() *dbus.Error  { return s.apply(session.EventStopSession{}) }
func (s *Service) Pause() *dbus.Error        { return s.apply(session.EventPause{}) }
func (s *Service) Resume() *dbus.Error       { return s.apply(session.EventResume{}) }
func (s *Service) SkipBreak() *dbus.Error    { return s.apply(session.EventSkipBreak{}) }

// Tick drives idle-credit and phase-cycle rules. It is NOT exported over
// D-Bus (see New: ExportMethodTable lists only the five methods above).
// cmd/cadenced calls it on a short internal interval; whether that
// produces bus traffic is decided entirely inside session.Apply, per
// specs/daemon-control "No per-second traffic".
func (s *Service) Tick() *dbus.Error {
	now := s.clock.Now()
	idleFor := s.idle.IdleFor(now)

	// The tier is sampled only while a session is active and not paused
	// (specs/session-timer, "Tier Gating"). Sampling spawns pw-dump and
	// walks /proc, and applyTick ignores the result outside that window, so
	// an idle daemon would pay for it every tick for nothing. The probes
	// still run outside the mutex: only the check is taken under it.
	s.mu.Lock()
	sample := s.state.Active && !s.state.Paused
	tier := s.state.Tier
	s.mu.Unlock()
	if sample {
		tier = s.tier.CurrentTier()
	}

	return s.apply(session.EventTick{IdleFor: idleFor, Tier: tier})
}

// Heartbeat persists the current state unconditionally. cmd/cadenced calls
// it on a ~60s interval so a daemon crash between transitions loses at
// most that much position — design.md, Decision "Persisted quantity",
// approach 2: "worst-case loss is bounded at one minute".
func (s *Service) Heartbeat() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.store.Save(s.state)
}

// ApplySuspend is sleep.go's entry point after a resume. It is a plain Go
// method, not a D-Bus method (time.Time is not a D-Bus-representable type,
// and it is never listed in New's ExportMethodTable).
func (s *Service) ApplySuspend(ev session.EventSuspended) *dbus.Error {
	return s.apply(ev)
}

// ApplyConfig is the config watcher's entry point after a successful reload
// (specs/daemon-configuration, "Live Reload"; design D4). Like ApplySuspend it
// is a plain Go method and never listed in New's ExportMethodTable: the file
// is the configuration's only writer, so D-Bus offers no way to set it.
func (s *Service) ApplyConfig(d session.Durations) *dbus.Error {
	return s.apply(session.EventConfigChanged{Durations: d})
}

// configMap renders the active policy as the published Config property: each
// key as written in the file, with its section, mapped to its value in the
// file's units (minutes, or a count for prompt_limit). Every key is always
// present, including those at their defaults (specs/daemon-control,
// "Configuration Publication"). int32 is 'i' on the wire, so the property is
// a{si}.
func configMap(d session.Durations) map[string]int32 {
	return map[string]int32{
		"timer.focus_minutes":             int32(d.Focus / time.Minute),
		"timer.break_minutes":             int32(d.Break / time.Minute),
		"idle.pause_after_minutes":        int32(d.IdlePause / time.Minute),
		"idle.credit_break_after_minutes": int32(d.IdleCredit / time.Minute),
		"camera.prompt_every_minutes":     int32(d.PromptRetry / time.Minute),
		"camera.prompt_limit":             int32(d.PromptCap),
	}
}

func (s *Service) apply(ev session.Event) *dbus.Error {
	s.mu.Lock()
	defer s.mu.Unlock()

	next, effects := session.Apply(s.state, ev, s.clock.Now())
	s.state = next

	for _, eff := range effects {
		switch eff.(type) {
		case session.EffectPersist:
			if err := s.store.Save(s.state); err != nil {
				return dbus.MakeFailedError(fmt.Errorf("dbusapi: persist: %w", err))
			}
		case session.EffectNotify:
			if err := s.publish(); err != nil {
				return dbus.MakeFailedError(fmt.Errorf("dbusapi: publish: %w", err))
			}
		}
	}
	return nil
}

// holdString maps the domain's hold stage onto the published Hold property.
// The stages already are the wire values; only the zero value needs
// normalising, so a State built without the field publishes "none".
func holdString(s session.State) string {
	if s.Hold == "" {
		return string(session.HoldNone)
	}
	return string(s.Hold)
}

// publish writes the current state's public fields into the exported
// properties and announces the ones that changed as a single
// PropertiesChanged.
//
// It emits the signal itself rather than letting prop.Properties do it.
// godbus emits one signal per Set, carrying a single-entry map, and never
// compares the old value to the new one (v5/prop, set -> emitChange), so a
// SetMust-per-property publish produced seven signals per idle boundary and
// republished four properties that had not changed. specs/daemon-control
// requires exactly one signal as an idle window opens and one as it closes,
// and requires that a tick changing no published value emit nothing at all.
//
// publish is only ever called from within apply, i.e. only on a real
// transition, never from a quiet Tick.
//
// SetMust panics on a closed connection, which is ordinary at logout, so that
// panic is caught here and handed to the caller instead of taking the process
// down (specs/daemon-control; cmd/cadenced logs it). godbus exposes only
// SetMust for an internal write: Set enforces the Writable flag, which is
// false for every property here, and the non-panicking p.set is unexported.
func (s *Service) publish() (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("publish: %v", r)
		}
	}()

	now := s.clock.Now()
	remaining := s.state.Remaining()
	if s.state.Paused {
		remaining = s.state.PausedRemaining
	}

	// Paused and Idle are both frozen intervals: no published deadline can
	// stay true for their duration, so none is published and the client
	// reads the frozen RemainingSeconds instead (specs/daemon-control,
	// "Idle Publication"). Idle needs no PausedRemaining equivalent —
	// ElapsedInPhase is frozen while the window is open, so Remaining()
	// is already constant throughout it.
	//
	// A held break is the third (specs/daemon-control, "Hold Publication";
	// design D7): elapsed is frozen while the user is on camera, so Remaining()
	// is the break's remainder at the hold and no deadline can stay true.
	frozen := s.state.Paused || s.state.Idle || s.state.Held()

	// RemainingSeconds and PhaseEndsAt are derived from one truncated
	// integer on one integer-second clock, never from a float now. A client
	// counting down as PhaseEndsAt minus its own floor(now) must reproduce
	// the frozen RemainingSeconds exactly at the instant a frozen interval
	// ends; deriving the deadline from a float now left the two up to a
	// second apart, so the countdown rendered one second HIGHER on leaving
	// idle than it had shown while frozen (specs/daemon-control, "A client
	// never drifts across suspends, idle windows and transitions").
	remainingSecs := int64(remaining.Seconds())
	endsAt := int64(0)
	if s.state.Active && !frozen {
		endsAt = now.Unix() + remainingSecs
	}

	desired := map[string]dbus.Variant{
		"SessionActive":    dbus.MakeVariant(s.state.Active),
		"Phase":            dbus.MakeVariant(string(s.state.Phase)),
		"PhaseEndsAt":      dbus.MakeVariant(endsAt),
		"RemainingSeconds": dbus.MakeVariant(remainingSecs),
		"Paused":           dbus.MakeVariant(s.state.Paused),
		"Tier":             dbus.MakeVariant(string(s.state.Tier)),
		"Idle":             dbus.MakeVariant(s.state.Idle),
		"Hold":             dbus.MakeVariant(holdString(s.state)),
		// int32 on the wire ('i'), matching the extension's interface XML.
		// It changes exactly once per prompt, so a client sees each new
		// prompt as a change even while Hold stays "prompt".
		"Prompts": dbus.MakeVariant(int32(s.state.Prompts)),
		// A map, so the diff below must not use ==: comparing two maps as
		// interface values panics (design D6).
		"Config": dbus.MakeVariant(configMap(s.state.Durations)),
	}

	// DeepEqual, not ==: Config is a map, and == on two interface values
	// holding maps panics. It treats every scalar property exactly as ==
	// did (design D6).
	changed := make(map[string]dbus.Variant, len(desired))
	for name, v := range desired {
		if prev, ok := s.published[name]; ok && reflect.DeepEqual(prev.Value(), v.Value()) {
			continue
		}
		s.props.SetMust(InterfaceName, name, v.Value())
		changed[name] = v
	}
	if len(changed) == 0 {
		return nil
	}

	if s.published == nil {
		s.published = make(map[string]dbus.Variant, len(desired))
	}
	for name, v := range changed {
		s.published[name] = v
	}

	return s.conn.Emit(ObjectPath, "org.freedesktop.DBus.Properties.PropertiesChanged",
		InterfaceName, changed, []string{})
}
