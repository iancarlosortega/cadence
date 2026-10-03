package dbusapi

import (
	"log"
	"sync"
	"time"

	"github.com/godbus/dbus/v5"
)

// Mutter's idle-monitor identity on the session bus. Verified live on
// GNOME Shell 48 under Wayland, exported by the gnome-shell process and
// reachable from an ordinary user process outside the Shell — which is
// what lets cadenced read it at all (research.md, claims C1-C2).
const (
	idleMonitorName   = "org.gnome.Mutter.IdleMonitor"
	idleMonitorPath   = dbus.ObjectPath("/org/gnome/Mutter/IdleMonitor/Core")
	idleMonitorMethod = idleMonitorName + ".GetIdletime"
)

// MutterIdleSource implements session.IdleSource against
// org.gnome.Mutter.IdleMonitor.
//
// The interface is undocumented upstream: its XML carries one line of
// prose, declares no unit, and says nothing about suspend or the lock
// screen (research.md, claim C5). Milliseconds is established by
// measurement, not by contract, so the conversion below is explicit and
// pinned by a test rather than left implicit.
type MutterIdleSource struct {
	obj dbus.BusObject

	mu        sync.Mutex
	available bool // last observed availability, for transition-only logging
}

// NewMutterIdleSource binds to the idle monitor on conn. It performs no
// call and cannot fail: the Shell may legitimately not be running yet,
// and that is not an error condition for the daemon
// (specs/session-timer, "Idle Source Availability").
func NewMutterIdleSource(conn *dbus.Conn) *MutterIdleSource {
	return &MutterIdleSource{
		obj:       conn.Object(idleMonitorName, idleMonitorPath),
		available: true, // assume reachable; the first failure logs the transition
	}
}

// IdleFor returns how long the user has been idle at the input level.
//
// The now parameter is unused: Mutter tracks the reference point itself
// and reports a duration directly. It stays in the signature because the
// port is shared with sources that would need it (ports.go).
//
// Any failure yields zero idle. Zero is the safe value, not merely a
// convenient one: it degrades cadence to charging all elapsed time as
// work — its behavior in every milestone before this one — rather than
// freezing a timer or crediting a break that was never earned. A missing
// idle source never terminates the daemon and never reaches a client
// (specs/session-timer, "Idle Source Availability").
func (m *MutterIdleSource) IdleFor(time.Time) time.Duration {
	var ms uint64
	err := m.obj.Call(idleMonitorMethod, 0).Store(&ms)
	if err != nil {
		m.setAvailable(false, err)
		return 0
	}
	m.setAvailable(true, nil)

	return idleFromMilliseconds(ms)
}

// idleFromMilliseconds converts Mutter's reading into a Duration. It is a
// named function so a test can pin the unit: upstream declares none
// (research.md, claim C5), so a silent change there would otherwise scale
// every configured threshold by a factor of a thousand without failing
// anything.
func idleFromMilliseconds(ms uint64) time.Duration {
	return time.Duration(ms) * time.Millisecond
}

// setAvailable logs only when availability changes. cadenced polls every
// tickInterval, so logging each failure would write thousands of
// identical lines a day while the Shell is down — which buries the one
// line that matters.
func (m *MutterIdleSource) setAvailable(now bool, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if now == m.available {
		return
	}
	m.available = now

	if now {
		log.Printf("cadenced: idle source available again (%s)", idleMonitorName)
		return
	}
	log.Printf("cadenced: idle source unavailable, reporting zero idle (%s): %v", idleMonitorName, err)
}
