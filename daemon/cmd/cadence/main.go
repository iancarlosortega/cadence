// Command cadence is the CLI client for cadenced: start|stop|status|skip.
// It is a thin D-Bus client — all logic lives in the daemon.
package main

import (
	"fmt"
	"os"
	"time"

	"github.com/godbus/dbus/v5"

	"cadence/daemon/internal/dbusapi"
)

func main() {
	if len(os.Args) != 2 {
		usage()
		os.Exit(2)
	}

	conn, err := dbus.SessionBus()
	if err != nil {
		fail(err)
	}
	defer conn.Close()

	obj := conn.Object(dbusapi.BusName, dbusapi.ObjectPath)

	switch os.Args[1] {
	case "start":
		call(obj, "StartSession")
	case "stop":
		call(obj, "StopSession")
	case "skip":
		call(obj, "SkipBreak")
	case "pause":
		call(obj, "Pause")
	case "resume":
		call(obj, "Resume")
	case "status":
		printStatus(obj)
	default:
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: cadence start|stop|status|skip|pause|resume")
}

func call(obj dbus.BusObject, method string) {
	if c := obj.Call(dbusapi.InterfaceName+"."+method, 0); c.Err != nil {
		fail(c.Err)
	}
}

func printStatus(obj dbus.BusObject) {
	active := getBool(obj, "SessionActive")
	if !active {
		fmt.Println("idle — no active session")
		return
	}

	snap := statusSnapshot{
		Phase:            getString(obj, "Phase"),
		Paused:           getBool(obj, "Paused"),
		Idle:             getBool(obj, "Idle"),
		PhaseEndsAt:      getInt64(obj, "PhaseEndsAt"),
		RemainingSeconds: getInt64(obj, "RemainingSeconds"),
	}
	tier := getString(obj, "Tier")

	fmt.Printf("%s — %s remaining, tier %s\n", snap.label(), snap.remaining(time.Now()), tier)
}

// statusSnapshot is the subset of the daemon's properties that status
// renders, read once so the derivation below is pure and testable.
type statusSnapshot struct {
	Phase            string
	Paused           bool
	Idle             bool
	PhaseEndsAt      int64
	RemainingSeconds int64
}

// remaining mirrors the panel's derivation (specs/panel-indicator,
// "Countdown Derivation"). RemainingSeconds is republished only on
// transitions, so between them it is a stale snapshot; the live countdown
// is PhaseEndsAt minus now. Paused and Idle both publish PhaseEndsAt as 0,
// and only then is the frozen RemainingSeconds the truthful value.
func (s statusSnapshot) remaining(now time.Time) time.Duration {
	if s.Paused || s.Idle {
		return time.Duration(s.RemainingSeconds) * time.Second
	}
	return time.Duration(max(s.PhaseEndsAt-now.Unix(), 0)) * time.Second
}

func (s statusSnapshot) label() string {
	switch {
	case s.Paused:
		return s.Phase + " (paused)"
	case s.Idle:
		return s.Phase + " (idle)"
	}
	return s.Phase
}

func getBool(obj dbus.BusObject, prop string) bool {
	var v bool
	getProp(obj, prop, &v)
	return v
}

func getString(obj dbus.BusObject, prop string) string {
	var v string
	getProp(obj, prop, &v)
	return v
}

func getInt64(obj dbus.BusObject, prop string) int64 {
	var v int64
	getProp(obj, prop, &v)
	return v
}

func getProp(obj dbus.BusObject, prop string, out any) {
	variant, err := obj.GetProperty(dbusapi.InterfaceName + "." + prop)
	if err != nil {
		fail(fmt.Errorf("get %s: %w", prop, err))
	}
	if err := variant.Store(out); err != nil {
		fail(fmt.Errorf("get %s: %w", prop, err))
	}
}

func fail(err error) {
	fmt.Fprintf(os.Stderr, "cadence: %v (is cadenced running?)\n", err)
	os.Exit(1)
}
