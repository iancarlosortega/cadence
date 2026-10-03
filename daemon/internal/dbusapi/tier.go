package dbusapi

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/godbus/dbus/v5"
	"github.com/godbus/dbus/v5/introspect"

	"cadence/daemon/internal/session"
)

// Where each tier signal lives. Measured on GNOME Shell 48 / PipeWire under
// Wayland (openspec/changes/m5-tier-detection/research.md, claims C1-C8).
const (
	screenCastName = "org.gnome.Mutter.ScreenCast"
	screenCastPath = dbus.ObjectPath("/org/gnome/Mutter/ScreenCast/Session")

	// pipewireTimeout bounds one pw-dump run. The measured cost is about
	// 10ms (C7), so this only fires when PipeWire is wedged; the tick loop
	// must not hang behind it (design D4).
	pipewireTimeout = 2 * time.Second

	pwNodeType   = "PipeWire:Interface:Node"
	pwMicClass   = "Stream/Input/Audio"
	pwVideoClass = "Video/Source"
	pwRunning    = "running"
)

// videoDevice matches the kernel's V4L2 device nodes a process holds open
// while it is using a camera (research.md, claim C2).
var videoDevice = regexp.MustCompile(`^/dev/video[0-9]+$`)

// probe is one independent tier signal (design D1). sample reports the
// highest tier this signal can show right now: T0 means "nothing seen".
// Returning a tier rather than a bool lets the PipeWire probe answer both
// the microphone (T1) and the camera node (T2) from a single pw-dump run
// instead of spawning it twice per tick.
type probe interface {
	name() string
	sample() (session.Tier, error)
}

// TierDetector implements session.TierSource from independent probes. The
// highest tier any probe reports wins (specs/session-timer, "Tier Gating":
// "The highest tier wins"). A probe that errors reads as absent, so a broken
// signal can only lower the tier and can never cause a break to be skipped
// (specs/session-timer, "Tier Source Availability").
type TierDetector struct {
	probes []probe

	mu        sync.Mutex
	available map[string]bool // last observed availability per probe, for transition-only logging
}

// NewTierDetector wires the three production probes. Like
// NewMutterIdleSource it performs no call and cannot fail: Mutter, PipeWire
// and /proc may each be missing or unreadable, and none of that is an error
// condition for the daemon (design D9).
//
// procRoot is "/proc" in production; pwDump is the pw-dump binary.
func NewTierDetector(conn *dbus.Conn, procRoot, pwDump string) *TierDetector {
	obj := conn.Object(screenCastName, screenCastPath)
	introspectFn := func() (string, error) {
		ctx, cancel := context.WithTimeout(context.Background(), pipewireTimeout)
		defer cancel()
		var doc string
		err := obj.CallWithContext(ctx, "org.freedesktop.DBus.Introspectable.Introspect", 0).Store(&doc)
		return doc, err
	}
	return newTierDetector(
		&screencastProbe{introspect: introspectFn},
		&cameraProbe{procRoot: procRoot},
		&pipewireProbe{bin: pwDump},
	)
}

func newTierDetector(probes ...probe) *TierDetector {
	d := &TierDetector{probes: probes, available: make(map[string]bool, len(probes))}
	for _, p := range probes {
		d.available[p.name()] = true // assume reachable; the first failure logs the transition
	}
	return d
}

// CurrentTier samples every probe and returns the highest tier seen. Probes
// are independent: one failing never suppresses another (design D1).
func (d *TierDetector) CurrentTier() session.Tier {
	highest := session.TierT0
	for _, p := range d.probes {
		tier, err := p.sample()
		d.setAvailable(p.name(), err == nil, err)
		if err != nil {
			continue // absent is the safe value
		}
		if tierRank(tier) > tierRank(highest) {
			highest = tier
		}
	}
	return highest
}

// tierRank orders the tiers for the "highest wins" rule. It is explicit
// because Tier is a string, and string order happens to agree only by luck.
func tierRank(t session.Tier) int {
	switch t {
	case session.TierT3:
		return 3
	case session.TierT2:
		return 2
	case session.TierT1:
		return 1
	}
	return 0
}

// setAvailable logs once per probe per availability transition, never per
// tick, for the same reason as MutterIdleSource.setAvailable: the tick polls
// every few seconds and a down component would bury the one line that matters.
func (d *TierDetector) setAvailable(name string, now bool, err error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if now == d.available[name] {
		return
	}
	d.available[name] = now

	if now {
		log.Printf("cadenced: tier signal %q available again", name)
		return
	}
	log.Printf("cadenced: tier signal %q unavailable, reading as absent: %v", name, err)
}

// screencastProbe reports T3 while Mutter has any screencast session
// (design D2).
type screencastProbe struct {
	// introspect returns the Introspect XML of the Mutter ScreenCast session
	// directory. A field so tests need no bus.
	introspect func() (string, error)
}

func (p *screencastProbe) name() string { return "screencast" }

func (p *screencastProbe) sample() (session.Tier, error) {
	doc, err := p.introspect()
	if err != nil {
		return session.TierT0, err
	}
	present, err := screencastActive(doc)
	if err != nil || !present {
		return session.TierT0, err
	}
	return session.TierT3, nil
}

// screencastActive reports whether the Introspect XML of the session
// directory lists any child object. At rest the path exists with no
// children; a share adds one (`Session/u2`) and removes it when it ends
// (research.md, claim C4). It is a named function so tests can pin it on
// captured XML.
func screencastActive(doc string) (bool, error) {
	var node introspect.Node
	if err := xml.NewDecoder(strings.NewReader(doc)).Decode(&node); err != nil {
		return false, fmt.Errorf("parse screencast introspection: %w", err)
	}
	return len(node.Children) > 0, nil
}

// cameraProbe reports T2 while any process holds a /dev/video* node open
// (design D3). It walks <procRoot>/[0-9]*/fd in-process: about 14ms and no
// spawn (research.md, claim C8).
type cameraProbe struct {
	procRoot string
}

func (p *cameraProbe) name() string { return "camera-devices" }

func (p *cameraProbe) sample() (session.Tier, error) {
	present, err := videoDeviceOpen(p.procRoot)
	if err != nil || !present {
		return session.TierT0, err
	}
	return session.TierT2, nil
}

// videoDeviceOpen reports whether any <root>/<pid>/fd entry links to a
// /dev/videoN node. Process directories that cannot be read (another user's,
// or a process exiting mid-walk) are skipped: only failing to read root
// itself is an error.
func videoDeviceOpen(root string) (bool, error) {
	pids, err := os.ReadDir(root)
	if err != nil {
		return false, err
	}
	for _, pid := range pids {
		if !isPID(pid.Name()) {
			continue
		}
		fdDir := filepath.Join(root, pid.Name(), "fd")
		fds, err := os.ReadDir(fdDir)
		if err != nil {
			continue
		}
		for _, fd := range fds {
			target, err := os.Readlink(filepath.Join(fdDir, fd.Name()))
			if err != nil {
				continue
			}
			if videoDevice.MatchString(target) {
				return true, nil
			}
		}
	}
	return false, nil
}

func isPID(name string) bool {
	if name == "" {
		return false
	}
	return strings.Trim(name, "0123456789") == ""
}

// pipewireProbe runs pw-dump once per sample and reports the microphone
// (T1) and the camera node (T2) from that one snapshot (design D4).
type pipewireProbe struct {
	bin string
}

func (p *pipewireProbe) name() string { return "pipewire" }

func (p *pipewireProbe) sample() (session.Tier, error) {
	ctx, cancel := context.WithTimeout(context.Background(), pipewireTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, p.bin)
	// WaitDelay bounds the wait for stdout to close after the context kills
	// the process, so a descendant holding the pipe cannot stall the tick
	// past the timeout.
	cmd.WaitDelay = 500 * time.Millisecond
	out, err := cmd.Output()
	if err != nil {
		return session.TierT0, fmt.Errorf("run %s: %w", p.bin, err)
	}
	mic, camera, err := pipewireSignals(out)
	if err != nil {
		return session.TierT0, err
	}
	switch {
	case camera:
		return session.TierT2, nil
	case mic:
		return session.TierT1, nil
	}
	return session.TierT0, nil
}

// pwObject is the minimum of a pw-dump object the probe needs. Everything
// else in the dump is ignored by the decoder.
type pwObject struct {
	Type string `json:"type"`
	Info struct {
		State string `json:"state"`
		Props struct {
			MediaClass string `json:"media.class"`
		} `json:"props"`
	} `json:"info"`
}

// pipewireSignals decodes a pw-dump snapshot into the two signals M5 reads:
// a running audio-input stream (a microphone in use, research.md claim C1)
// and a running video source node (the camera as seen through PipeWire,
// claim U1). Suspended nodes are idle and do not count.
func pipewireSignals(dump []byte) (mic, camera bool, err error) {
	var objs []pwObject
	if err := json.NewDecoder(bytes.NewReader(dump)).Decode(&objs); err != nil {
		return false, false, fmt.Errorf("decode pw-dump output: %w", err)
	}
	for _, o := range objs {
		if o.Type != pwNodeType || o.Info.State != pwRunning {
			continue
		}
		switch o.Info.Props.MediaClass {
		case pwMicClass:
			mic = true
		case pwVideoClass:
			camera = true
		}
	}
	return mic, camera, nil
}
