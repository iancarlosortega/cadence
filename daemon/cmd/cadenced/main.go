// Command cadenced is the cadence daemon: it owns the timer domain and
// exposes it over D-Bus. It draws no pixels — see
// openspec/changes/m1-daemon-core/design.md.
package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/godbus/dbus/v5"

	"cadence/daemon/internal/config"
	"cadence/daemon/internal/dbusapi"
	"cadence/daemon/internal/session"
	"cadence/daemon/internal/store"
)

// tickInterval is how often the daemon evaluates idle-credit and phase
// transitions. It is unrelated to bus traffic: whether a tick produces a
// PropertiesChanged signal is decided entirely inside session.Apply
// (specs/daemon-control, "No per-second traffic").
const tickInterval = 5 * time.Second

// heartbeatInterval bounds crash-loss to about one minute
// (design.md, Decision "Persisted quantity").
const heartbeatInterval = 60 * time.Second

// noIdleSource is M1's stub: idle detection is M4 (Mutter IdleMonitor).
type noIdleSource struct{}

func (noIdleSource) IdleFor(time.Time) time.Duration { return 0 }

// fixedT0Tier is M1's stub: real tier detection is M5 (PipeWire, the
// ScreenCast portal, camera).
type fixedT0Tier struct{}

func (fixedT0Tier) CurrentTier() session.Tier { return session.TierT0 }

func main() {
	if err := run(); err != nil {
		log.Fatalf("cadenced: %v", err)
	}
}

func run() error {
	configPath, err := config.Path()
	if err != nil {
		return err
	}
	durations, err := config.Load(configPath)
	if err != nil {
		return err
	}
	log.Printf("cadenced: config loaded from %s (focus=%s break=%s)", configPath, durations.Focus, durations.Break)

	statePath, err := store.Path()
	if err != nil {
		return err
	}
	st := store.NewFileStore(statePath)

	clock := session.RealClock{}
	now := clock.Now()

	initial, found, err := st.Load()
	if err != nil {
		return err
	}
	if found {
		initial.Durations = durations // config may have changed since the last run
		log.Printf("cadenced: resumed session (active=%v phase=%s elapsed=%s)", initial.Active, initial.Phase, initial.ElapsedInPhase)
	} else {
		initial = session.State{Durations: durations, LastObserved: now}
		log.Print("cadenced: no prior session found, starting idle")
	}

	conn, err := dbus.SessionBus()
	if err != nil {
		return err
	}
	defer conn.Close()

	svc, err := dbusapi.New(conn, initial, st, clock, fixedT0Tier{}, noIdleSource{})
	if err != nil {
		return err
	}
	log.Printf("cadenced: exported %s at %s", dbusapi.BusName, dbusapi.ObjectPath)

	if ev, ok := startupGap(initial, found, now); ok {
		if err := svc.ApplySuspend(ev); err != nil {
			log.Printf("cadenced: startup gap: %v", err)
		}
	}

	watchSystemSleep(svc, clock)

	ticker := time.NewTicker(tickInterval)
	defer ticker.Stop()
	heartbeat := time.NewTicker(heartbeatInterval)
	defer heartbeat.Stop()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)

	for {
		select {
		case <-ticker.C:
			if err := svc.Tick(); err != nil {
				log.Printf("cadenced: tick: %v", err)
			}
		case <-heartbeat.C:
			if err := svc.Heartbeat(); err != nil {
				log.Printf("cadenced: heartbeat: %v", err)
			}
		case <-sig:
			log.Print("cadenced: shutting down")
			return nil
		}
	}
}

// startupGap reports the absence to replay on resume. The gap since
// LastObserved is time this daemon did not witness, which is what
// EventSuspended already describes — so downtime and suspend obey one rule
// (specs/session-persistence, "Downtime Is Time Away").
func startupGap(initial session.State, found bool, now time.Time) (session.EventSuspended, bool) {
	if !found || !initial.Active {
		return session.EventSuspended{}, false
	}
	return session.EventSuspended{From: initial.LastObserved, To: now}, true
}

// watchSystemSleep is best-effort: the system bus or login1 may be
// unavailable in some environments, and suspend handling is a refinement
// over the heartbeat, not a hard requirement (design.md, Open Questions).
// A failure here is logged, never fatal.
func watchSystemSleep(svc *dbusapi.Service, clock session.Clock) {
	system, err := dbus.SystemBus()
	if err != nil {
		log.Printf("cadenced: system bus unavailable, suspend detection disabled: %v", err)
		return
	}
	if err := dbusapi.WatchSleep(system, svc, clock); err != nil {
		log.Printf("cadenced: could not subscribe to login1 PrepareForSleep, suspend detection disabled: %v", err)
	}
}
