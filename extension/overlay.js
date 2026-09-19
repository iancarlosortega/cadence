/* The break overlay actor and its lifecycle.
 *
 * This module knows nothing about session state. It is told show() or hide();
 * whether it should be up is decided by shouldShowOverlay() in render.js, which
 * is pure and testable outside the Shell.
 *
 * No modal grab: addTopChrome with the default affectsInputRegion absorbs mouse
 * input, while the keyboard stays free (specs/break-overlay, Overlay Presence).
 */

import Clutter from 'gi://Clutter';
import GLib from 'gi://GLib';
import GObject from 'gi://GObject';
import St from 'gi://St';

import * as Main from 'resource:///org/gnome/shell/ui/main.js';

import {HOLD_TO_SKIP_SECONDS} from './render.js';

const FADE_MS = 300;

export const CadenceOverlay = GObject.registerClass(
class CadenceOverlay extends St.Widget {
    _init(onSkipRequested) {
        // A plain St.Widget has no layout manager, so a child's x_align and
        // y_align are ignored and it lands at the top-left. BinLayout is what
        // makes centring work.
        super._init({
            style_class: 'cadence-overlay',
            reactive: true,
            can_focus: true,
            layout_manager: new Clutter.BinLayout(),
        });

        this._onSkipRequested = onSkipRequested;
        this._holdId = 0;

        const box = new St.BoxLayout({
            vertical: true,
            x_align: Clutter.ActorAlign.CENTER,
            y_align: Clutter.ActorAlign.CENTER,
            x_expand: true,
            y_expand: true,
            style_class: 'cadence-overlay-box',
        });

        // St.Label defaults to ActorAlign.FILL, which stretches each label to the
        // box width and draws its text from the left. The widest child only
        // looks centred by coincidence, so every child says so explicitly.
        this._heading = new St.Label({
            text: 'Time to stand up',
            style_class: 'cadence-overlay-heading',
            x_align: Clutter.ActorAlign.CENTER,
        });
        this._remaining = new St.Label({
            text: '',
            style_class: 'cadence-overlay-remaining',
            x_align: Clutter.ActorAlign.CENTER,
        });

        this._skip = new St.Button({
            label: `Hold to skip (${HOLD_TO_SKIP_SECONDS}s)`,
            style_class: 'cadence-overlay-skip',
            can_focus: true,
            reactive: true,
            x_align: Clutter.ActorAlign.CENTER,
        });
        // scale_x defaults to 1, which renders a full bar at rest and then
        // snaps to empty on the first press. The hold fills it from zero, so
        // zero is the resting state.
        this._progress = new St.Widget({
            style_class: 'cadence-overlay-progress',
            x_align: Clutter.ActorAlign.CENTER,
        });
        this._progress.set_pivot_point(0, 0.5);
        this._progress.scale_x = 0;

        this._skip.connect('button-press-event', () => {
            this._beginHold();
            return Clutter.EVENT_STOP;
        });
        this._skip.connect('button-release-event', () => {
            this._cancelHold();
            return Clutter.EVENT_STOP;
        });
        this._skip.connect('leave-event', () => {
            this._cancelHold();
            return Clutter.EVENT_PROPAGATE;
        });

        box.add_child(this._heading);
        box.add_child(this._remaining);
        box.add_child(this._skip);
        box.add_child(this._progress);
        this.add_child(box);

        this.connect('destroy', () => this._cancelHold());
    }

    setRemaining(text) {
        this._remaining.text = text;
    }

    _beginHold() {
        if (this._holdId)
            return;

        this._progress.remove_all_transitions();
        this._progress.scale_x = 0;
        this._progress.ease({
            scale_x: 1,
            duration: HOLD_TO_SKIP_SECONDS * 1000,
            mode: Clutter.AnimationMode.LINEAR,
        });

        // timeout_add, not timeout_add_seconds: the seconds variant coalesces to
        // second boundaries, so a 3s hold can fire up to a second early and
        // complete before the progress bar finishes filling. GLib's own docs
        // say to use timeout_add when the precision matters. Both the timeout
        // and the animation are now driven from the same millisecond value.
        this._holdId = GLib.timeout_add(
            GLib.PRIORITY_DEFAULT, HOLD_TO_SKIP_SECONDS * 1000, () => {
                this._holdId = 0;
                this._progress.remove_all_transitions();
                this._progress.scale_x = 0;
                // Request only. The daemon ends the break, and the resulting
                // property change dismisses this overlay — never dismiss here
                // (specs/break-overlay, Hold To Skip).
                this._onSkipRequested();
                return GLib.SOURCE_REMOVE;
            });
    }

    _cancelHold() {
        if (this._holdId) {
            GLib.Source.remove(this._holdId);
            this._holdId = 0;
        }
        this._progress.remove_all_transitions();
        this._progress.scale_x = 0;
    }
});

/* Owns at most one overlay actor. show() is idempotent for a given monitor;
 * hide() destroys rather than hides, so chrome tracking and the input region
 * are released with the actor (layout.js _trackActor connects destroy). */
export class OverlayController {
    constructor(onSkipRequested) {
        this._onSkipRequested = onSkipRequested;
        this._overlay = null;
        this._monitorIndex = -1;
    }

    get visible() {
        return this._overlay !== null;
    }

    show(monitor) {
        if (this._overlay && this._monitorIndex === monitor.index)
            return;
        if (this._overlay)
            this.hide();

        const overlay = new CadenceOverlay(this._onSkipRequested);
        overlay.set_position(monitor.x, monitor.y);
        overlay.set_size(monitor.width, monitor.height);

        Main.layoutManager.addTopChrome(overlay);
        this._overlay = overlay;
        this._monitorIndex = monitor.index;

        if (St.Settings.get().enable_animations) {
            overlay.opacity = 0;
            overlay.ease({opacity: 255, duration: FADE_MS, mode: Clutter.AnimationMode.EASE_OUT_QUAD});
        }
    }

    setRemaining(text) {
        this._overlay?.setRemaining(text);
    }

    hide() {
        if (!this._overlay)
            return;
        const overlay = this._overlay;
        this._overlay = null;
        this._monitorIndex = -1;
        overlay.remove_all_transitions();
        overlay.destroy();
    }

    destroy() {
        this.hide();
        this._onSkipRequested = null;
    }
}
