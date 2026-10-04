#!/usr/bin/env -S gjs -m
/* Pure-logic checks for the render function. Runs outside GNOME Shell.
 *
 * Imports render.js, never extension.js: the latter pulls in Shell-only
 * resources that do not exist under plain gjs. Run with:
 *   gjs -m extension/test-render.js
 */

import {
    computeDisplay,
    formatMMSS,
    menuSensitivity,
    nextSuppression,
    promptSurface,
    shouldShowOverlay,
    DISCONNECTED,
    PANEL_SECONDS,
    WARNING_THRESHOLD_SECONDS,
} from './render.js';

let failures = 0;

function check(name, actual, expected) {
    const a = JSON.stringify(actual);
    const e = JSON.stringify(expected);
    if (a === e) {
        print(`ok   ${name}`);
    } else {
        print(`FAIL ${name}\n       expected ${e}\n       actual   ${a}`);
        failures++;
    }
}

const NOW = 1_000_000;
const active = (over = {}) => ({
    available: true,
    sessionActive: true,
    phase: 'focus',
    phaseEndsAt: NOW + 600,
    remainingSeconds: 600,
    paused: false,
    tier: 'T0',
    ...over,
});

check('formats minutes and seconds', formatMMSS(125), '2:05');
check('clamps negative to zero', formatMMSS(-5), '0:00');

check('disconnected is dimmed',
    computeDisplay(DISCONNECTED, NOW),
    {dimmed: true, label: '', warning: false});

check('inactive session is dimmed',
    computeDisplay(active({sessionActive: false}), NOW),
    {dimmed: true, label: '', warning: false});

// DISCONNECTED clears available and sessionActive together, so the test above
// passes even if the availability check is deleted. This isolates it.
check('unavailable alone is enough to dim',
    computeDisplay(active({available: false}), NOW),
    {dimmed: true, label: '', warning: false});

check('active focus counts down from PhaseEndsAt',
    computeDisplay(active(), NOW),
    {dimmed: false, label: '10:00', warning: false});

// I1: the deadline is absolute, so a late tick self-corrects instead of drifting.
check('late tick self-corrects',
    computeDisplay(active(), NOW + 300),
    {dimmed: false, label: '5:00', warning: false});

// I1: at zero the renderer shows 00:00 and says nothing about the phase.
check('zero remainder renders 0:00, never negative',
    computeDisplay(active(), NOW + 900),
    {dimmed: false, label: '0:00', warning: true});

check('warning boundary is inclusive at 120s',
    computeDisplay(active({phaseEndsAt: NOW + WARNING_THRESHOLD_SECONDS}), NOW).warning,
    true);

check('no warning at 121s',
    computeDisplay(active({phaseEndsAt: NOW + WARNING_THRESHOLD_SECONDS + 1}), NOW).warning,
    false);

check('no warning during break',
    computeDisplay(active({phase: 'break', phaseEndsAt: NOW + 60}), NOW).warning,
    false);

// specs/panel-indicator, "Break Warning": no warning while presenting, since
// no break will start; T1 and T2 still get it because a break will.
check('no warning in focus at 60s under T3',
    computeDisplay(active({tier: 'T3', phaseEndsAt: NOW + 60}), NOW).warning,
    false);

check('warning returns under T2',
    computeDisplay(active({tier: 'T2', phaseEndsAt: NOW + 60}), NOW).warning,
    true);

check('warning returns under T1',
    computeDisplay(active({tier: 'T1', phaseEndsAt: NOW + 60}), NOW).warning,
    true);

check('T3 does not change the label, only the warning',
    computeDisplay(active({tier: 'T3', phaseEndsAt: NOW + 60}), NOW).label,
    '1:00');

// Paused publishes PhaseEndsAt as 0; reading it would render a huge negative.
check('paused reads RemainingSeconds, not PhaseEndsAt',
    computeDisplay(active({paused: true, phaseEndsAt: 0, remainingSeconds: 720}), NOW),
    {dimmed: false, label: '12:00', warning: false});

check('paused never warns even under the threshold',
    computeDisplay(active({paused: true, phaseEndsAt: 0, remainingSeconds: 30}), NOW).warning,
    false);

// Idle publishes PhaseEndsAt as 0 exactly as Paused does; reading it would
// render a huge negative. specs/panel-indicator, "Idle freezes on
// RemainingSeconds".
check('idle reads RemainingSeconds, not PhaseEndsAt',
    computeDisplay(active({idle: true, phaseEndsAt: 0, remainingSeconds: 720}), NOW),
    {dimmed: false, label: '12:00', warning: false});

// The frozen label must not drift with the clock: that is the whole point
// of suppressing the deadline for the window.
check('idle label does not advance with time',
    computeDisplay(active({idle: true, phaseEndsAt: 0, remainingSeconds: 720}), NOW + 600),
    {dimmed: false, label: '12:00', warning: false});

check('idle never warns even under the threshold',
    computeDisplay(active({idle: true, phaseEndsAt: 0, remainingSeconds: 30}), NOW).warning,
    false);

// specs/panel-indicator, "Countdown resumes on return from idle".
check('countdown resumes from PhaseEndsAt once idle clears',
    computeDisplay(active({idle: false, phaseEndsAt: NOW + 720}), NOW),
    {dimmed: false, label: '12:00', warning: false});

// specs/panel-indicator, "Menu during an idle window". Offering Resume for
// a freeze the user never requested would drive EventResume against a
// session that never stored PausedRemaining.
check('idle offers Pause and withholds Resume',
    menuSensitivity(active({idle: true, paused: false})),
    {start: false, stop: true, pause: true, resume: false, skip: false});

check('menu when disconnected',
    menuSensitivity(DISCONNECTED),
    {start: false, stop: false, pause: false, resume: false, skip: false});

check('menu in running focus',
    menuSensitivity(active()),
    {start: false, stop: true, pause: true, resume: false, skip: false});

check('menu while paused',
    menuSensitivity(active({paused: true})),
    {start: false, stop: true, pause: false, resume: true, skip: false});

check('skip only offered during break',
    menuSensitivity(active({phase: 'break'})).skip,
    true);

check('menu with daemon up but no session',
    menuSensitivity({...DISCONNECTED, available: true}),
    {start: true, stop: false, pause: false, resume: false, skip: false});

// specs/break-overlay — the predicate carries three of the four Self-Owned
// Exit paths, so these are safety tests, not cosmetics.

const onBreak = (over = {}) => ({
    available: true,
    sessionActive: true,
    phase: 'break',
    phaseEndsAt: NOW + 300,
    remainingSeconds: 300,
    paused: false,
    tier: 'T0',
    ...over,
});

check('overlay shows during an unsuppressed break',
    shouldShowOverlay(onBreak(), NOW, false), true);

check('suppressed break shows nothing',
    shouldShowOverlay(onBreak(), NOW, true), false);

// Exit path 2: the phase left break.
check('focus shows nothing',
    shouldShowOverlay(onBreak({phase: 'focus'}), NOW, false), false);

// Exit path 3: the daemon went away. This is the F3 path — if it regresses,
// a vanished daemon leaves the screen covered.
check('unavailable daemon shows nothing',
    shouldShowOverlay(DISCONNECTED, NOW, false), false);

// DISCONNECTED clears available AND sessionActive together, so the check above
// passes even if the availability test is deleted. This isolates it: a state
// that still looks like a live break but is not available must not show.
check('unavailable alone is enough to hide, even mid-break',
    shouldShowOverlay(onBreak({available: false}), NOW, false), false);

check('daemon up but no session shows nothing',
    shouldShowOverlay(onBreak({sessionActive: false}), NOW, false), false);

// Exit path 1: the locally derived remainder reached zero, with no property
// change needed.
check('zero remaining shows nothing without any property change',
    shouldShowOverlay(onBreak({phaseEndsAt: NOW}), NOW, false), false);

check('one second left still shows',
    shouldShowOverlay(onBreak({phaseEndsAt: NOW + 1}), NOW, false), true);

check('paused break with time left still shows',
    shouldShowOverlay(onBreak({paused: true, phaseEndsAt: 0, remainingSeconds: 42}), NOW, false), true);

// A break does not freeze while idle, so the overlay keeps counting down
// from a live PhaseEndsAt for the whole break — including the part of it
// the user spends away from the desk, which is the point.
check('idle during a break still counts down from PhaseEndsAt',
    shouldShowOverlay(active({phase: 'break', idle: false, phaseEndsAt: NOW + 420}), NOW, false),
    true);

check('paused break with nothing left shows nothing',
    shouldShowOverlay(onBreak({paused: true, phaseEndsAt: 0, remainingSeconds: 0}), NOW, false), false);

// ---------------------------------------------------------------------------
// M6: held breaks (specs/panel-indicator "Countdown Derivation", specs/
// break-overlay "Overlay Presence", specs/camera-prompt).
// ---------------------------------------------------------------------------

// A held break publishes PhaseEndsAt as 0, exactly as Paused and Idle do.
const held = (over = {}) => onBreak({
    hold: 'prompt',
    prompts: 1,
    phaseEndsAt: 0,
    remainingSeconds: 600,
    ...over,
});

// specs/panel-indicator, "A held break freezes on RemainingSeconds".
check('held break reads RemainingSeconds, not PhaseEndsAt',
    computeDisplay(held(), NOW),
    {dimmed: false, label: '10:00', warning: false});

check('held label does not advance with time',
    computeDisplay(held(), NOW + 600),
    {dimmed: false, label: '10:00', warning: false});

check('pill-stage hold freezes the same way',
    computeDisplay(held({hold: 'pill', prompts: 3}), NOW + 60),
    {dimmed: false, label: '10:00', warning: false});

// specs/break-overlay, "Held break shows no overlay" and "Hold lifting
// shows the overlay".
check('held break shows no overlay',
    shouldShowOverlay(held(), NOW, false), false);

check('pill-stage hold shows no overlay',
    shouldShowOverlay(held({hold: 'pill'}), NOW, false), false);

check('a lifted hold shows the overlay while the break runs',
    shouldShowOverlay(onBreak({hold: 'none', prompts: 2}), NOW, false), true);

// specs/camera-prompt, "Corner Panel": 15s from the prompt, then gone.
const SHOWN = NOW + 1;
const UNTIL = SHOWN + PANEL_SECONDS;

check('PANEL_SECONDS is 15', PANEL_SECONDS, 15);

check('panel is shown before 15s',
    promptSurface(held(), UNTIL, UNTIL - 1), 'panel');

check('panel is gone at exactly 15s',
    promptSurface(held(), UNTIL, UNTIL), null);

check('panel is gone after 15s',
    promptSurface(held(), UNTIL, UNTIL + 60), null);

check('no panel before any prompt was seen',
    promptSurface(held(), 0, NOW), null);

// specs/camera-prompt, "Pill".
check('pill shows for the pill stage',
    promptSurface(held({hold: 'pill', prompts: 3}), 0, NOW), 'pill');

check('pill stays however long it has been shown',
    promptSurface(held({hold: 'pill', prompts: 3}), UNTIL, UNTIL + 3600), 'pill');

check('no surface when the hold is none',
    promptSurface(onBreak({hold: 'none'}), UNTIL, UNTIL - 1), null);

check('no surface when the hold field is absent (older daemon)',
    promptSurface(onBreak(), UNTIL, UNTIL - 1), null);

// specs/camera-prompt, "Prompt Surface Teardown": neither outlives its cause.
check('no surface when the daemon is unavailable',
    promptSurface(held({available: false}), UNTIL, UNTIL - 1), null);

check('no surface when disconnected',
    promptSurface(DISCONNECTED, UNTIL, UNTIL - 1), null);

check('no pill when the daemon is unavailable',
    promptSurface(held({hold: 'pill', available: false}), 0, NOW), null);

check('no surface when no session is active',
    promptSurface(held({sessionActive: false}), UNTIL, UNTIL - 1), null);

check('no pill when no session is active',
    promptSurface(held({hold: 'pill', sessionActive: false}), 0, NOW), null);

// The render tick stops while paused, so a panel could never reach its 15s
// and would stay up for as long as the pause lasts. Hidden instead.
check('no panel while paused',
    promptSurface(held({paused: true}), UNTIL, UNTIL - 1), null);

check('DISCONNECTED carries the hold fields',
    [DISCONNECTED.hold, DISCONNECTED.prompts], ['none', 0]);

// Fullscreen suppression is decided when a break starts RUNNING, not when it
// is merely owed (specs/break-overlay, "Fullscreen Suppression" and
// "Overlay Presence": a lifted hold is when the break begins). A held break
// entered during a fullscreen call must not leave the later, running break
// suppressed once the call has ended.
const RUN = {available: true, sessionActive: true, phase: 'break', hold: 'none'};
const HELD = {...RUN, hold: 'prompt'};
const FOCUS = {...RUN, phase: 'focus'};

check('running break edge decides from fullscreen',
    nextSuppression({wasRunning: false, suppressed: false}, RUN, true),
    {wasRunning: true, suppressed: true});
check('a held break is not the edge and is never suppressed',
    nextSuppression({wasRunning: false, suppressed: false}, HELD, true),
    {wasRunning: false, suppressed: false});
check('lifting a hold decides afresh: call ended, not fullscreen',
    nextSuppression({wasRunning: false, suppressed: false}, RUN, false),
    {wasRunning: true, suppressed: false});
check('mid-break the decision is kept even if fullscreen changes',
    nextSuppression({wasRunning: true, suppressed: true}, RUN, false),
    {wasRunning: true, suppressed: true});
check('leaving the break clears suppression',
    nextSuppression({wasRunning: true, suppressed: true}, FOCUS, false),
    {wasRunning: false, suppressed: false});

if (failures > 0) {
    print(`\n${failures} check(s) failed`);
    imports.system.exit(1);
}
print('\nall checks passed');
