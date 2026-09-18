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
    DISCONNECTED,
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

// Paused publishes PhaseEndsAt as 0; reading it would render a huge negative.
check('paused reads RemainingSeconds, not PhaseEndsAt',
    computeDisplay(active({paused: true, phaseEndsAt: 0, remainingSeconds: 720}), NOW),
    {dimmed: false, label: '12:00', warning: false});

check('paused never warns even under the threshold',
    computeDisplay(active({paused: true, phaseEndsAt: 0, remainingSeconds: 30}), NOW).warning,
    false);

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

if (failures > 0) {
    print(`\n${failures} check(s) failed`);
    imports.system.exit(1);
}
print('\nall checks passed');
