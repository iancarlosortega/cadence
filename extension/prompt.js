/* The quiet surfaces that stand in for the break overlay while the user is on
 * camera: a corner panel that asks, and a pill that remembers
 * (specs/camera-prompt).
 *
 * Like overlay.js, this module knows nothing about session state. It is told
 * showPanel(), showPill() or hide(); which one is up is decided by
 * promptSurface() in render.js, which is pure and testable outside the Shell.
 *
 * No modal grab: addTopChrome absorbs pointer input only over the actor's own
 * rectangle, so the call window behind the panel stays usable.
 */

import Clutter from 'gi://Clutter';
import St from 'gi://St';

import * as Main from 'resource:///org/gnome/shell/ui/main.js';

// Gap between the surface and the monitor's right edge and the top bar.
const MARGIN = 16;

/* The panel: a message and a Skip button. A plain St.BoxLayout is enough here,
 * unlike the overlay, because it has a real layout manager of its own. */
function createPanel(onSkipRequested) {
    const panel = new St.BoxLayout({
        style_class: 'cadence-prompt',
        vertical: true,
        reactive: true,
    });

    // St.Label defaults to ActorAlign.FILL, which stretches it to the box
    // width and draws its text from the left. Every child says where it sits.
    const message = new St.Label({
        text: 'A break is due, and you are on camera.',
        style_class: 'cadence-prompt-message',
        x_align: Clutter.ActorAlign.START,
    });

    const skip = new St.Button({
        label: 'Skip break',
        style_class: 'cadence-prompt-button',
        can_focus: true,
        reactive: true,
        x_align: Clutter.ActorAlign.END,
    });
    // Request only. The daemon ends the break, and the resulting property
    // change removes the panel; never dismiss here (specs/camera-prompt,
    // "Corner Panel").
    skip.connect('clicked', () => {
        onSkipRequested();
        return Clutter.EVENT_STOP;
    });

    panel.add_child(message);
    panel.add_child(skip);
    return panel;
}

/* The pill: a label in a rounded bin. It offers no action, so it must not
 * take pointer input either (see show()). */
function createPill() {
    const label = new St.Label({
        text: 'Break owed',
        style_class: 'cadence-pill-label',
        x_align: Clutter.ActorAlign.CENTER,
        y_align: Clutter.ActorAlign.CENTER,
    });
    return new St.Bin({
        style_class: 'cadence-pill',
        reactive: false,
        child: label,
    });
}

/* Owns at most one surface actor. A surface is swapped when its kind or its
 * monitor changes, and destroyed rather than hidden: a hidden actor can leave
 * a stale input region behind, while destroy releases chrome tracking with the
 * actor (layout.js _trackActor connects destroy). */
export class PromptController {
    constructor(onSkipRequested) {
        this._onSkipRequested = onSkipRequested;
        this._actor = null;
        this._kind = null;
        this._monitorIndex = -1;
    }

    get visible() {
        return this._actor !== null;
    }

    showPanel(monitor) {
        this._show('panel', monitor);
    }

    showPill(monitor) {
        this._show('pill', monitor);
    }

    /* Move the current surface after the monitor layout changed. */
    reposition(monitor) {
        if (!this._actor)
            return;
        if (this._monitorIndex !== monitor.index) {
            this._show(this._kind, monitor);
            return;
        }
        this._place(this._actor, monitor);
    }

    _show(kind, monitor) {
        if (this._actor && this._kind === kind && this._monitorIndex === monitor.index)
            return;
        if (this._actor)
            this.hide();

        const actor = kind === 'panel'
            ? createPanel(() => this._onSkipRequested?.())
            : createPill();

        // The pill has no action, so it keeps out of the input region and
        // lets every click through. The panel keeps the default, which
        // absorbs input over its own rectangle only.
        Main.layoutManager.addTopChrome(actor,
            kind === 'pill' ? {affectsInputRegion: false} : {});
        this._actor = actor;
        this._kind = kind;
        this._monitorIndex = monitor.index;
        this._place(actor, monitor);
    }

    /* Top-right of the monitor, below the top bar. The width is read after the
     * actor is on the stage: a theme node, and so its CSS padding, only
     * resolves once it has a stage to resolve against. */
    _place(actor, monitor) {
        const [, width] = actor.get_preferred_width(-1);
        actor.set_position(
            monitor.x + monitor.width - width - MARGIN,
            monitor.y + Main.panel.height + MARGIN);
    }

    hide() {
        if (!this._actor)
            return;
        const actor = this._actor;
        this._actor = null;
        this._kind = null;
        this._monitorIndex = -1;
        actor.remove_all_transitions();
        actor.destroy();
    }

    destroy() {
        this.hide();
        this._onSkipRequested = null;
    }
}
