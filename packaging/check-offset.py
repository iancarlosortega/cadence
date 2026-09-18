#!/usr/bin/env python3
"""Sample the client-derived countdown against the daemon's elapsed-derived truth.

A sub-second constant offset is PhaseEndsAt's whole-second publish quantization.
A constant offset of a whole second or more is drift, and is the defect that
fix-suspend-deadline-republish addressed.
"""

import datetime
import json
import os
import subprocess
import sys
import time

STATE = os.path.expanduser("~/.local/state/cadence/session.json")
SAMPLES = 3
GAP_SECONDS = 15


def prop(name):
    out = subprocess.run(
        ["busctl", "--user", "get-property", "dev.ian.Cadence",
         "/dev/ian/Cadence", "dev.ian.Cadence1", name],
        capture_output=True, text=True,
    )
    if out.returncode != 0:
        sys.exit("cadenced is not on the session bus — start it first")
    return out.stdout.split()[-1].strip('"')


def main():
    if prop("SessionActive") != "true":
        sys.exit("no active session — run `cadence start` first")
    if prop("Paused") == "true":
        sys.exit("session is paused — resume it first")

    worst = 0.0
    for i in range(SAMPLES):
        # Re-read the phase every sample. A transition between samples changes
        # the phase length, and a total carried over from the previous phase
        # reports the difference between the two as drift.
        phase_before = prop("Phase")
        now = time.time()
        ends = float(prop("PhaseEndsAt"))
        s = json.load(open(STATE))
        phase_after = prop("Phase")

        if phase_before != phase_after:
            print(f"sample {i + 1}: skipped, phase changed mid-sample "
                  f"({phase_before} -> {phase_after})")
            if i < SAMPLES - 1:
                time.sleep(GAP_SECONDS)
            continue

        total = s[f"{phase_after}_minutes"] * 60.0
        observed = datetime.datetime.fromisoformat(s["last_observed"]).timestamp()
        true_remaining = total - (s["elapsed_in_phase_ns"] / 1e9 + (now - observed))
        offset = true_remaining - (ends - now)
        worst = max(worst, abs(offset))
        print(f"sample {i + 1}: phase={phase_after:5s} true={true_remaining:9.3f}s  "
              f"client={ends - now:9.3f}s  offset={offset:+.3f}s")
        if i < SAMPLES - 1:
            time.sleep(GAP_SECONDS)

    print()
    if worst < 1.0:
        print(f"PASS — worst offset {worst:.3f}s is below one second, which is PhaseEndsAt's")
        print("       whole-second publish quantization rather than drift.")
        return 0
    print(f"FAIL — worst offset {worst:.3f}s is a whole second or more.")
    print("       The client is counting to a deadline the daemon no longer agrees with.")
    return 1


if __name__ == "__main__":
    sys.exit(main())
