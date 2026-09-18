// Package dbusapi is the D-Bus adapter: it exposes session.State over the
// session bus and drives session.Apply from method calls and internal
// ticks. It is the only package in the daemon that imports godbus.
package dbusapi

import (
	"fmt"
	"sync"

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

	propsMap := prop.Map{
		InterfaceName: {
			"SessionActive":    {Value: initial.Active, Writable: false, Emit: prop.EmitTrue},
			"Phase":            {Value: string(initial.Phase), Writable: false, Emit: prop.EmitTrue},
			"PhaseEndsAt":      {Value: int64(0), Writable: false, Emit: prop.EmitTrue},
			"RemainingSeconds": {Value: int64(0), Writable: false, Emit: prop.EmitTrue},
			"Paused":           {Value: initial.Paused, Writable: false, Emit: prop.EmitTrue},
			"Tier":             {Value: string(initial.Tier), Writable: false, Emit: prop.EmitTrue},
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

	reply, err := conn.RequestName(BusName, dbus.NameFlagDoNotQueue)
	if err != nil {
		return nil, fmt.Errorf("dbusapi: request name %s: %w", BusName, err)
	}
	if reply != dbus.RequestNameReplyPrimaryOwner {
		return nil, fmt.Errorf("dbusapi: name %s already owned (reply %d) — is cadenced already running?", BusName, reply)
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
	tier := s.tier.CurrentTier()
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

// publish writes the current state's public fields into the exported
// properties. prop.Properties.SetMust emits PropertiesChanged for any
// value that actually changed (Emit: EmitTrue) — see
// github.com/godbus/dbus/v5/prop: Set/SetMust call emitChange internally.
// publish is only ever called from within apply, i.e. only on a real
// transition, never from a quiet Tick.
// publish converts prop's panic-on-error contract into an error. godbus
// exposes only SetMust for an internal write: Set enforces the Writable flag,
// which is false for every property here, and the non-panicking p.set is
// unexported. SetMust panics on a closed connection, which is ordinary at
// logout, so that panic is caught here and handed to the caller instead of
// taking the process down (specs/daemon-control; cmd/cadenced logs it).
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
	endsAt := int64(0)
	if s.state.Active && !s.state.Paused {
		endsAt = now.Add(remaining).Unix()
	}

	s.props.SetMust(InterfaceName, "SessionActive", s.state.Active)
	s.props.SetMust(InterfaceName, "Phase", string(s.state.Phase))
	s.props.SetMust(InterfaceName, "PhaseEndsAt", endsAt)
	s.props.SetMust(InterfaceName, "RemainingSeconds", int64(remaining.Seconds()))
	s.props.SetMust(InterfaceName, "Paused", s.state.Paused)
	s.props.SetMust(InterfaceName, "Tier", string(s.state.Tier))
	return nil
}
