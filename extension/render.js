/* Pure presentation logic for the cadence indicator.
 *
 * This module imports nothing — not St, not Gio, not Shell resources — so it
 * runs under plain `gjs` outside GNOME Shell. That is what makes invariant I1
 * testable: the countdown is a function of (state, now) holding no state
 * between calls, so a drifting client-side timer cannot be written here.
 */

export const WARNING_THRESHOLD_SECONDS = 120;
export const HOLD_TO_SKIP_SECONDS = 3;

/* How long the corner panel stays up for each prompt (specs/camera-prompt,
 * "Corner Panel"; design D9). */
export const PANEL_SECONDS = 15;

export const DISCONNECTED = Object.freeze({
    available: false,
    sessionActive: false,
    phase: 'none',
    phaseEndsAt: 0,
    remainingSeconds: 0,
    paused: false,
    tier: 'T0',
    idle: false,
    hold: 'none',
    prompts: 0,
});

/* Whether the daemon reports a held break (design D9). Written against the
 * two held stages rather than `!== 'none'` so a snapshot without the field,
 * from an older daemon, reads as not held. */
function isHeld(state) {
    return state.hold === 'prompt' || state.hold === 'pill';
}

/* (prev, state, inFullscreen) -> {wasRunning, suppressed}
 *
 * Fullscreen suppression is decided once, when a break starts running, so
 * it cannot flicker as fullscreen toggles (specs/break-overlay, "Fullscreen
 * Suppression"). A held break is owed, not running: its overlay begins when
 * the hold lifts ("Overlay Presence"), so that is the edge. Deciding at hold
 * entry instead would suppress the running break whenever the call that
 * caused the hold was fullscreen, which is the usual case. */
export function nextSuppression(prev, state, inFullscreen) {
    const running = state.available && state.sessionActive &&
        state.phase === 'break' && !isHeld(state);
    if (!running)
        return {wasRunning: false, suppressed: false};
    if (!prev.wasRunning)
        return {wasRunning: true, suppressed: inFullscreen};
    return {wasRunning: true, suppressed: prev.suppressed};
}

export function formatMMSS(seconds) {
    const total = Math.max(0, Math.floor(seconds));
    return `${Math.floor(total / 60)}:${String(total % 60).padStart(2, '0')}`;
}

/* (state, nowSeconds) -> {dimmed, label, warning} */
export function computeDisplay(state, nowSeconds) {
    if (!state.available || !state.sessionActive)
        return {dimmed: true, label: '', warning: false};

    // Paused and Idle both publish PhaseEndsAt as 0, so the frozen value is
    // the only truthful input here. Idle is the daemon's to declare: the
    // extension never monitors input itself (specs/panel-indicator,
    // "Countdown Derivation").
    //
    // The warning is suppressed for both. It exists to catch the user's
    // attention before a break begins; while the daemon reports them idle
    // there is no attention to catch and the remainder is not moving.
    //
    // A held break is the third frozen interval: PhaseEndsAt is 0 and the
    // remainder is frozen at the hold (specs/panel-indicator, "Countdown
    // Derivation").
    if (state.paused || state.idle || isHeld(state)) {
        return {
            dimmed: false,
            label: formatMMSS(state.remainingSeconds),
            warning: false,
        };
    }

    // No warning while presenting (specs/panel-indicator, "Break Warning"):
    // under T3 the daemon skips the break at the deadline, so there is
    // nothing to warn about, and an amber flash would be visible to the
    // audience. T1 and T2 still start a break, so they keep the warning.
    const remaining = Math.max(0, state.phaseEndsAt - nowSeconds);
    return {
        dimmed: false,
        label: formatMMSS(remaining),
        warning: state.phase === 'focus' && state.tier !== 'T3' &&
            remaining <= WARNING_THRESHOLD_SECONDS,
    };
}

/* Sensitivity derives from `paused` and deliberately never from `idle`.
 *
 * An idle freeze is not a pause: the user never asked for it. Offering
 * Resume for one would invoke EventResume against a session that never set
 * PausedRemaining, recomputing elapsed from a value that was never stored.
 * That is a state-corruption path, not a cosmetic wrong label, and it is
 * why the daemon publishes Idle as its own property rather than reusing
 * Paused (specs/panel-indicator, "Control Actions").
 */
export function menuSensitivity(state) {
    const live = state.available && state.sessionActive;
    return {
        start: state.available && !state.sessionActive,
        stop: live,
        pause: live && !state.paused,
        resume: live && state.paused,
        skip: live && state.phase === 'break',
    };
}

/* Whether the break overlay should be covering the screen.
 *
 * Three of the four dismissal paths in specs/break-overlay "Self-Owned Exit"
 * reduce to this returning false: the derived remainder reaching zero, the
 * phase leaving break, and the daemon going away. The fourth, disable(), is
 * structural. suppressed is decided once at the break edge by the caller, so
 * this stays pure and does not re-evaluate fullscreen every tick.
 */
export function shouldShowOverlay(state, nowSeconds, suppressed) {
    if (suppressed)
        return false;
    if (!state.available || !state.sessionActive)
        return false;
    if (state.phase !== 'break')
        return false;
    // A held break is the corner prompt's, not the overlay's
    // (specs/break-overlay, "Overlay Presence"). When the hold lifts this
    // returns true on the next render, which is the "within one tick" the
    // spec asks for.
    if (isHeld(state))
        return false;
    // Paused is the only frozen condition reachable here: a break does not
    // freeze while idle (specs/session-timer, "Idle Credit"), so Idle is
    // false throughout one and PhaseEndsAt stays live.
    if (state.paused)
        return state.remainingSeconds > 0;

    return state.phaseEndsAt - nowSeconds > 0;
}

/* Which quiet surface, if any, stands in for the overlay (specs/camera-prompt;
 * design D9): 'panel', 'pill' or null.
 *
 * panelUntil is the wall-clock second the current panel expires, set by the
 * caller when Prompts rises. Taking it as an argument keeps this pure: the
 * 15 seconds are a function of (state, panelUntil, now) like the countdown is
 * of (state, now), so there is no timer to cancel at teardown.
 *
 * Paused hides the panel but not the pill. The render tick stops while paused,
 * so a panel shown just before a pause could never reach its 15 seconds and
 * would stay up for the length of the pause; the pill is persistent by
 * definition, so it has no clock to run out.
 */
export function promptSurface(state, panelUntil, nowSeconds) {
    if (!state.available || !state.sessionActive)
        return null;
    if (state.hold === 'pill')
        return 'pill';
    if (state.hold === 'prompt' && !state.paused && nowSeconds < panelUntil)
        return 'panel';
    return null;
}
