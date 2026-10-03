package dbusapi

import (
	"bytes"
	"errors"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"cadence/daemon/internal/session"
)

// fakeProbe returns a scripted result so the composite is tested without a
// bus, a /proc or PipeWire.
type fakeProbe struct {
	n    string
	tier session.Tier
	err  error
}

func (f *fakeProbe) name() string                  { return f.n }
func (f *fakeProbe) sample() (session.Tier, error) { return f.tier, f.err }

// captureLog redirects the standard logger for one test.
func captureLog(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })
	return &buf
}

// specs/session-timer, Requirement "Tier Gating", Scenario "The highest tier
// wins".
func TestHighestTierWins(t *testing.T) {
	cases := []struct {
		name   string
		probes []probe
		want   session.Tier
	}{
		{"none", []probe{&fakeProbe{n: "a", tier: session.TierT0}}, session.TierT0},
		{"mic only", []probe{&fakeProbe{n: "a", tier: session.TierT1}, &fakeProbe{n: "b", tier: session.TierT0}}, session.TierT1},
		{"camera over mic", []probe{&fakeProbe{n: "a", tier: session.TierT1}, &fakeProbe{n: "b", tier: session.TierT2}}, session.TierT2},
		{
			"share over camera and mic",
			[]probe{
				&fakeProbe{n: "a", tier: session.TierT1},
				&fakeProbe{n: "b", tier: session.TierT2},
				&fakeProbe{n: "c", tier: session.TierT3},
			},
			session.TierT3,
		},
		{
			"order does not matter",
			[]probe{&fakeProbe{n: "c", tier: session.TierT3}, &fakeProbe{n: "a", tier: session.TierT1}},
			session.TierT3,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := newTierDetector(c.probes...).CurrentTier(); got != c.want {
				t.Fatalf("CurrentTier = %s, want %s", got, c.want)
			}
		})
	}
}

// specs/session-timer, Requirement "Tier Source Availability", Scenario
// "Every signal unavailable". An error reads as absent, so a broken signal
// can only lower the tier, and it never suppresses a healthy one.
func TestFailingProbeReadsAbsent(t *testing.T) {
	captureLog(t)
	boom := errors.New("boom")

	allDown := newTierDetector(
		&fakeProbe{n: "a", err: boom}, &fakeProbe{n: "b", err: boom}, &fakeProbe{n: "c", err: boom},
	)
	if got := allDown.CurrentTier(); got != session.TierT0 {
		t.Fatalf("every probe down: tier = %s, want T0", got)
	}

	oneDown := newTierDetector(
		&fakeProbe{n: "a", err: boom, tier: session.TierT3}, // a result next to an error is ignored
		&fakeProbe{n: "b", tier: session.TierT2},
	)
	if got := oneDown.CurrentTier(); got != session.TierT2 {
		t.Fatalf("one probe down: tier = %s, want the healthy probe's T2", got)
	}
}

// specs/session-timer, Requirement "Tier Source Availability": "Its
// unavailability MUST be logged when it changes, not on every tick."
func TestProbeUnavailabilityLogsOnTransitionsOnly(t *testing.T) {
	buf := captureLog(t)
	p := &fakeProbe{n: "pipewire", err: errors.New("no such file")}
	d := newTierDetector(p)

	for i := 0; i < 5; i++ {
		d.CurrentTier()
	}
	if n := strings.Count(buf.String(), "unavailable"); n != 1 {
		t.Fatalf("logged %d unavailable lines over 5 failing ticks, want 1:\n%s", n, buf.String())
	}
	if !strings.Contains(buf.String(), `"pipewire"`) {
		t.Fatalf("the line must name the probe:\n%s", buf.String())
	}

	p.err = nil
	for i := 0; i < 5; i++ {
		d.CurrentTier()
	}
	if n := strings.Count(buf.String(), "available again"); n != 1 {
		t.Fatalf("logged %d recovery lines over 5 healthy ticks, want 1:\n%s", n, buf.String())
	}
}

// A healthy probe never logs anything.
func TestHealthyProbeIsSilent(t *testing.T) {
	buf := captureLog(t)
	d := newTierDetector(&fakeProbe{n: "a", tier: session.TierT1})
	for i := 0; i < 5; i++ {
		d.CurrentTier()
	}
	if buf.Len() != 0 {
		t.Fatalf("a healthy probe logged: %s", buf.String())
	}
}

// Real Introspect output of /org/gnome/Mutter/ScreenCast/Session on GNOME
// Shell 48 at rest (gdbus introspect, 2026-10-03): the path exists and has no
// children.
const screencastAtRestXML = `<!DOCTYPE node PUBLIC "-//freedesktop//DTD D-BUS Object Introspection 1.0//EN"
                      "http://www.freedesktop.org/standards/dbus/1.0/introspect.dtd">
<!-- GDBus 2.84.4 -->
<node>
</node>
`

// The same document while a share is live: Mutter exports one child object
// per session (research.md, claim C4).
const screencastSharingXML = `<!DOCTYPE node PUBLIC "-//freedesktop//DTD D-BUS Object Introspection 1.0//EN"
                      "http://www.freedesktop.org/standards/dbus/1.0/introspect.dtd">
<!-- GDBus 2.84.4 -->
<node>
  <node name="u2"/>
</node>
`

// specs/session-timer, Requirement "Tier Gating" (T3 = any screencast),
// design D2.
func TestScreencastActive(t *testing.T) {
	cases := []struct {
		name string
		xml  string
		want bool
	}{
		{"at rest has no children", screencastAtRestXML, false},
		{"a share adds a child", screencastSharingXML, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := screencastActive(c.xml)
			if err != nil {
				t.Fatalf("screencastActive: %v", err)
			}
			if got != c.want {
				t.Fatalf("screencastActive = %v, want %v", got, c.want)
			}
		})
	}

	if _, err := screencastActive("not xml <"); err == nil {
		t.Fatal("malformed XML must be an error so the probe reads as unavailable")
	}
}

func TestScreencastProbeMapsToT3(t *testing.T) {
	sharing := &screencastProbe{introspect: func() (string, error) { return screencastSharingXML, nil }}
	if got, err := sharing.sample(); err != nil || got != session.TierT3 {
		t.Fatalf("sharing: tier=%s err=%v, want T3", got, err)
	}
	rest := &screencastProbe{introspect: func() (string, error) { return screencastAtRestXML, nil }}
	if got, err := rest.sample(); err != nil || got != session.TierT0 {
		t.Fatalf("rest: tier=%s err=%v, want T0", got, err)
	}
}

// An unreachable bus name (no Shell, or no ScreenCast service) is an error,
// which the detector then reads as absent.
func TestScreencastProbeUnreachableIsAnError(t *testing.T) {
	down := &screencastProbe{introspect: func() (string, error) {
		return "", errors.New("The name org.gnome.Mutter.ScreenCast was not provided by any .service files")
	}}
	if _, err := down.sample(); err == nil {
		t.Fatal("an unreachable service must be an error, not a silent T0")
	}
}

// procFixture builds a fake /proc: procFixture(t, map[pid]map[fd]target).
func procFixture(t *testing.T, procs map[string]map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for pid, fds := range procs {
		fdDir := filepath.Join(root, pid, "fd")
		if err := os.MkdirAll(fdDir, 0o755); err != nil {
			t.Fatal(err)
		}
		for fd, target := range fds {
			if err := os.Symlink(target, filepath.Join(fdDir, fd)); err != nil {
				t.Fatal(err)
			}
		}
	}
	return root
}

// design D3: a process holding /dev/video* open is a camera in use.
func TestVideoDeviceOpen(t *testing.T) {
	cases := []struct {
		name  string
		procs map[string]map[string]string
		want  bool
	}{
		{"video0 held", map[string]map[string]string{"100": {"3": "/dev/video0", "4": "/dev/null"}}, true},
		{"video12 held", map[string]map[string]string{"100": {"3": "/dev/video12"}}, true},
		{
			"other targets only",
			map[string]map[string]string{
				"100": {"0": "/dev/null", "1": "/dev/pts/3"},
				"200": {"5": "/home/u/video0", "6": "/dev/video0-not", "7": "/dev/videoX"},
			},
			false,
		},
		{"no processes", map[string]map[string]string{}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := videoDeviceOpen(procFixture(t, c.procs))
			if err != nil {
				t.Fatalf("videoDeviceOpen: %v", err)
			}
			if got != c.want {
				t.Fatalf("videoDeviceOpen = %v, want %v", got, c.want)
			}
		})
	}
}

// Process directories that cannot be read are skipped, never errors: they are
// other users' processes or ones that exited mid-walk (design D3).
func TestVideoDeviceOpenSkipsUnreadableProcesses(t *testing.T) {
	root := procFixture(t, map[string]map[string]string{"300": {"3": "/dev/video0"}})

	// A pid directory whose fd directory is gone (a process that exited).
	if err := os.MkdirAll(filepath.Join(root, "100"), 0o755); err != nil {
		t.Fatal(err)
	}
	// A pid "directory" that is a plain file, and non-pid entries.
	if err := os.WriteFile(filepath.Join(root, "200"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "self", "fd"), 0o755); err != nil {
		t.Fatal(err)
	}

	got, err := videoDeviceOpen(root)
	if err != nil {
		t.Fatalf("unreadable entries must be skipped, got error: %v", err)
	}
	if !got {
		t.Fatal("the readable process holding /dev/video0 must still be found")
	}

	// Remove it: with only unreadable entries left the answer is absent, not an error.
	if err := os.RemoveAll(filepath.Join(root, "300")); err != nil {
		t.Fatal(err)
	}
	got, err = videoDeviceOpen(root)
	if err != nil || got {
		t.Fatalf("only unreadable entries: got=%v err=%v, want absent and no error", got, err)
	}
}

// Failing to read the root itself is the one real error.
func TestVideoDeviceOpenMissingRootIsAnError(t *testing.T) {
	if _, err := videoDeviceOpen(filepath.Join(t.TempDir(), "no-such-proc")); err == nil {
		t.Fatal("a missing /proc must be an error")
	}
}

// Hand-written to the shape of real pw-dump output (a JSON array of objects
// with type, info.state and info.props), keeping only the fields the probe
// reads plus the noise it must ignore: a non-Node object, a node with no
// media.class, and suspended streams.
const pwDumpRest = `[
  {"id": 0, "type": "PipeWire:Interface:Core", "info": {"props": {"core.name": "pipewire-0"}}},
  {"id": 30, "type": "PipeWire:Interface:Node", "info": {"state": "suspended", "props": {}}},
  {"id": 41, "type": "PipeWire:Interface:Node", "info": {"state": "suspended", "props": {"media.class": "Audio/Sink"}}},
  {"id": 52, "type": "PipeWire:Interface:Node", "info": {"state": "suspended", "props": {"media.class": "Video/Source"}}}
]`

const pwDumpMic = `[
  {"id": 41, "type": "PipeWire:Interface:Node", "info": {"state": "running", "props": {"media.class": "Audio/Sink"}}},
  {"id": 77, "type": "PipeWire:Interface:Node", "info": {"state": "running", "props": {"media.class": "Stream/Input/Audio"}}}
]`

const pwDumpCamera = `[
  {"id": 77, "type": "PipeWire:Interface:Node", "info": {"state": "suspended", "props": {"media.class": "Stream/Input/Audio"}}},
  {"id": 88, "type": "PipeWire:Interface:Node", "info": {"state": "running", "props": {"media.class": "Video/Source"}}}
]`

// design D4 and research.md claims C1/U1: one snapshot answers both signals.
func TestPipewireSignals(t *testing.T) {
	cases := []struct {
		name       string
		dump       string
		mic, video bool
	}{
		{"rest: neither", pwDumpRest, false, false},
		{"mic only", pwDumpMic, true, false},
		{"running Video/Source is the camera", pwDumpCamera, false, true},
		{"empty graph", `[]`, false, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			mic, video, err := pipewireSignals([]byte(c.dump))
			if err != nil {
				t.Fatalf("pipewireSignals: %v", err)
			}
			if mic != c.mic || video != c.video {
				t.Fatalf("mic=%v video=%v, want mic=%v video=%v", mic, video, c.mic, c.video)
			}
		})
	}

	if _, _, err := pipewireSignals([]byte("not json")); err == nil {
		t.Fatal("unparseable output must be an error")
	}
}

// A missing binary reads as an error (and so as absent in the detector),
// never as T0-by-success and never a crash.
func TestPipewireProbeMissingBinaryIsAnError(t *testing.T) {
	p := &pipewireProbe{bin: filepath.Join(t.TempDir(), "no-such-pw-dump")}
	if _, err := p.sample(); err == nil {
		t.Fatal("a missing pw-dump must be an error")
	}
}

// The probe's tier mapping end to end, with a script standing in for pw-dump.
func TestPipewireProbeMapsSignalsToTiers(t *testing.T) {
	cases := []struct {
		name string
		dump string
		want session.Tier
	}{
		{"rest", pwDumpRest, session.TierT0},
		{"mic", pwDumpMic, session.TierT1},
		{"camera", pwDumpCamera, session.TierT2},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dir := t.TempDir()
			data := filepath.Join(dir, "dump.json")
			if err := os.WriteFile(data, []byte(c.dump), 0o644); err != nil {
				t.Fatal(err)
			}
			script := filepath.Join(dir, "pw-dump")
			if err := os.WriteFile(script, []byte("#!/bin/sh\ncat '"+data+"'\n"), 0o755); err != nil {
				t.Fatal(err)
			}
			got, err := (&pipewireProbe{bin: script}).sample()
			if err != nil || got != c.want {
				t.Fatalf("tier=%s err=%v, want %s", got, err, c.want)
			}
		})
	}
}
