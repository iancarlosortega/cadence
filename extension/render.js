/* Pure presentation logic for the cadence indicator.
 *
 * This module imports nothing — not St, not Gio, not Shell resources — so it
 * runs under plain `gjs` outside GNOME Shell. That is what makes invariant I1
 * testable: the countdown is a function of (state, now) holding no state
 * between calls, so a drifting client-side timer cannot be written here.
 */

export const WARNING_THRESHOLD_SECONDS = 120;

export const DISCONNECTED = Object.freeze({
    available: false,
    sessionActive: false,
    phase: 'none',
    phaseEndsAt: 0,
    remainingSeconds: 0,
    paused: false,
    tier: 'T0',
});

export function formatMMSS(seconds) {
    const total = Math.max(0, Math.floor(seconds));
    return `${Math.floor(total / 60)}:${String(total % 60).padStart(2, '0')}`;
}

/* (state, nowSeconds) -> {dimmed, label, warning} */
export function computeDisplay(state, nowSeconds) {
    if (!state.available || !state.sessionActive)
        return {dimmed: true, label: '', warning: false};

    // Paused publishes PhaseEndsAt as 0, so the frozen value is the only
    // truthful input here.
    if (state.paused) {
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
