package dbusapi

import (
	"github.com/godbus/dbus/v5"

	"cadence/daemon/internal/session"
)

// login1's suspend-notification identity (see
// openspec/changes/m1-daemon-core/research.md, claim C3).
const (
	login1Path      = dbus.ObjectPath("/org/freedesktop/login1")
	login1Interface = "org.freedesktop.login1.Manager"
	login1Signal    = login1Interface + ".PrepareForSleep"
)

// WatchSleep subscribes to org.freedesktop.login1.Manager.PrepareForSleep
// on the SYSTEM bus and drives svc.ApplySuspend across it. The signal
// carries a bool: true immediately before suspend, false after resume
// (research.md claim C3). Persisting around that boundary, and
// re-evaluating elapsed time on resume, is the mechanism recorded in
// design.md — the periodic heartbeat elsewhere still covers a daemon
// crash, which emits no signal at all.
//
// It runs its dispatch loop in its own goroutine and returns once the
// subscription is registered.
func WatchSleep(system *dbus.Conn, svc *Service, clock session.Clock) error {
	call := system.BusObject().Call(
		"org.freedesktop.DBus.AddMatch", 0,
		"type='signal',interface='"+login1Interface+"',member='PrepareForSleep',path='"+string(login1Path)+"'",
	)
	if call.Err != nil {
		return call.Err
	}

	ch := make(chan *dbus.Signal, 8)
	system.Signal(ch)

	go func() {
		sleepStarted := clock.Now()
		for sig := range ch {
			if sig.Name != login1Signal || len(sig.Body) != 1 {
				continue
			}
			starting, ok := sig.Body[0].(bool)
			if !ok {
				continue
			}
			if starting {
				sleepStarted = clock.Now()
				continue
			}
			resumedAt := clock.Now()
			svc.ApplySuspend(session.EventSuspended{From: sleepStarted, To: resumedAt})
		}
	}()

	return nil
}
