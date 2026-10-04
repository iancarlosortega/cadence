package config

import (
	"os"
	"time"

	"cadence/daemon/internal/session"
)

// FileID is the cheap identity of the config file at one instant: whether it
// exists, its modification time and its size. Two polls with equal FileIDs are
// taken to be the same content, so the file is not re-read (design D3).
type FileID struct {
	exists bool
	failed bool // stat failed for a reason other than absence
	mod    time.Time
	size   int64
}

// Identify stats path. Absence is a valid identity (exists == false), not an
// error, because a missing file is a valid configuration.
func Identify(path string) FileID {
	info, err := os.Stat(path)
	switch {
	case err == nil:
		return FileID{exists: true, mod: info.ModTime(), size: info.Size()}
	case os.IsNotExist(err):
		return FileID{}
	default:
		return FileID{failed: true}
	}
}

// Watcher notices changes to the config file by polling its identity. It is
// driven by the daemon's tick, with no goroutine and no dependency: stat
// follows the path, so an editor that saves by renaming a new file over the
// old one is seen, where an inotify watch on the old inode would go silent
// (design D3). It is not safe for concurrent use; the main loop is its only
// caller.
type Watcher struct {
	path string
	last FileID
}

// NewWatcher returns a watcher whose baseline is active, the identity of the
// file the daemon has already loaded, so the first Poll does not reload what
// startup just read. Take active with Identify before Load.
func NewWatcher(path string, active FileID) *Watcher {
	return &Watcher{path: path, last: active}
}

// Poll reports whether the file changed since the last poll and, if so, what
// it now configures.
//
//	same identity            -> changed false, nothing read
//	different, file missing  -> Defaults(), changed true
//	different, file loads    -> the loaded values, changed true
//	different, load fails    -> err; the caller keeps its active configuration
//
// The new identity is recorded in every case, so a broken file is reported
// once and not re-parsed on every tick; the next save changes the identity
// and is loaded afresh (specs/daemon-configuration, "Live Reload").
func (w *Watcher) Poll() (d session.Durations, changed bool, err error) {
	id := Identify(w.path)
	if id == w.last {
		return session.Durations{}, false, nil
	}
	w.last = id

	if !id.exists && !id.failed {
		return Defaults(), true, nil
	}
	d, err = Load(w.path)
	if err != nil {
		return session.Durations{}, false, err
	}
	return d, true, nil
}
