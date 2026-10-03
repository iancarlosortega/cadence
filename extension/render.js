/* Pure presentation logic for the cadence indicator.
 *
 * This module imports nothing — not St, not Gio, not Shell resources — so it
 * runs under plain `gjs` outside GNOME Shell. That is what makes invariant I1
 * testable: the countdown is a function of (state, now) holding no state
 * between calls, so a drifting client-side timer cannot be written here.
 */

export const WARNING_THRESHOLD_SECONDS = 120;
export const HOLD_TO_SKIP_SECONDS = 3;

export const DISCONNECTED = Object.freeze({
    available: false,
    sessionActive: false,
    phase: 'none',
    phaseEndsAt: 0,
    remainingSeconds: 0,
    paused: false,
    tier: 'T0',
    idle: false,
});

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
    if (state.paused || state.idle) {
        return {
            dimmed: false,
            label: formatMMSS(state.remainingSeconds),
            warning: false,
        };
    }

    const remaining = Math.max(0, state.phaseEndsAt - nowSeconds);
    return {
        dimmed: false,
        label: formatMMSS(remaining),
        warning: state.phase === 'focus' && remaining <= WARNING_THRESHOLD_SECONDS,
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
    // Paused is the only frozen condition reachable here: a break does not
    // freeze while idle (specs/session-timer, "Idle Credit"), so Idle is
    // false throughout one and PhaseEndsAt stays live.
    if (state.paused)
        return state.remainingSeconds > 0;

    return state.phaseEndsAt - nowSeconds > 0;
}
