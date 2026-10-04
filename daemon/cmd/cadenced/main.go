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
	// The identity is taken before the load, so a save landing between the two
	// is seen as a change by the watcher rather than lost (design D3).
	loaded := config.Identify(configPath)
	durations, err := config.Load(configPath)
	if err != nil {
		return err // startup stays strict; only reloads are tolerant (design D8)
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

	idle := dbusapi.NewMutterIdleSource(conn)
	tier := dbusapi.NewTierDetector(conn, "/proc", "pw-dump")

	svc, err := dbusapi.New(conn, initial, st, clock, tier, idle)
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

	// Seeded with the file startup just read, so the first poll does not
	// reload it.
	watcher := config.NewWatcher(configPath, loaded)

	ticker := time.NewTicker(tickInterval)
	defer ticker.Stop()
	heartbeat := time.NewTicker(heartbeatInterval)
	defer heartbeat.Stop()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)

	for {
		select {
		case <-ticker.C:
			// Reload before the tick, so a phase shortened by the new config
			// ends on this very tick (design D4).
			reloadConfig(watcher, svc)
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

// reloadConfig applies a changed config file to the running session. A file
// that fails to load is logged once by the watcher's contract and leaves the
// active configuration untouched (specs/daemon-configuration, "Live Reload").
func reloadConfig(w *config.Watcher, svc *dbusapi.Service) {
	d, changed, err := w.Poll()
	if err != nil {
		log.Printf("cadenced: config reload ignored: %v", err)
		return
	}
	if !changed {
		return
	}
	if err := svc.ApplyConfig(d); err != nil {
		log.Printf("cadenced: apply config: %v", err)
		return
	}
	log.Printf("cadenced: config reloaded (focus=%s break=%s)", d.Focus, d.Break)
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
