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

	phase := getString(obj, "Phase")
	paused := getBool(obj, "Paused")
	remaining := getInt64(obj, "RemainingSeconds")
	tier := getString(obj, "Tier")

	state := phase
	if paused {
		state = phase + " (paused)"
	}
	fmt.Printf("%s — %s remaining, tier %s\n", state, time.Duration(remaining)*time.Second, tier)
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
