package session

import "time"

// Event is one thing that happened, fed into Apply. Representing every
// input as an event (rather than calling time.Now or dbus inside the
// machine) is what makes all 8 session-timer scenarios table-testable with
// a fake clock and no desktop session.
type Event interface{ isEvent() }

// EventTick asks Apply to advance elapsed time to now, given the caller's
// most recent observation of idle duration and tier at that instant.
type EventTick struct {
	IdleFor time.Duration
	Tier    Tier
}

type EventStartSession struct{}
type EventStopSession struct{}
type EventPause struct{}
type EventResume struct{}
type EventSkipBreak struct{}

// EventSuspended reports that the system was suspended between From and To
// (both wall-clock instants, from the login1 PrepareForSleep(true)/(false)
// pair). Suspend is treated as time away from the desk, using the same
// idle thresholds as EventTick's idle duration
// (specs/session-timer, "Suspend Is Time Away").
type EventSuspended struct {
	From, To time.Time
}

// EventConfigChanged replaces the configured policy while the daemon runs
// (specs/daemon-configuration, "Live Reload"; design D5). The adapter builds it
// from a successfully loaded config file; an invalid file never becomes one.
type EventConfigChanged struct{ Durations Durations }

func (EventTick) isEvent()          {}
func (EventStartSession) isEvent()  {}
func (EventStopSession) isEvent()   {}
func (EventPause) isEvent()         {}
func (EventResume) isEvent()        {}
func (EventSkipBreak) isEvent()     {}
func (EventSuspended) isEvent()     {}
func (EventConfigChanged) isEvent() {}
