/* cadence — GNOME Shell 48 panel indicator.
 *
 * Invariant I1: D-Bus properties are the only authority for session state; the
 * local tick only renders. computeDisplay() is pure and keeps no countdown
 * between calls, so a drifting client-side timer is unrepresentable rather
 * than merely discouraged.
 */

import Clutter from 'gi://Clutter';
import Gio from 'gi://Gio';
import GLib from 'gi://GLib';
import GObject from 'gi://GObject';
import St from 'gi://St';

import {Extension} from 'resource:///org/gnome/shell/extensions/extension.js';
import * as Main from 'resource:///org/gnome/shell/ui/main.js';
import * as PanelMenu from 'resource:///org/gnome/shell/ui/panelMenu.js';
import * as PopupMenu from 'resource:///org/gnome/shell/ui/popupMenu.js';

import {computeDisplay, menuSensitivity, DISCONNECTED} from './render.js';

const BUS_NAME = 'dev.ian.Cadence';
const OBJECT_PATH = '/dev/ian/Cadence';

const IFACE_XML = `
<node>
  <interface name="dev.ian.Cadence1">
    <method name="StartSession"/>
    <method name="StopSession"/>
    <method name="Pause"/>
    <method name="Resume"/>
    <method name="SkipBreak"/>
    <property name="SessionActive" type="b" access="read"/>
    <property name="Phase" type="s" access="read"/>
    <property name="PhaseEndsAt" type="x" access="read"/>
    <property name="RemainingSeconds" type="x" access="read"/>
    <property name="Paused" type="b" access="read"/>
    <property name="Tier" type="s" access="read"/>
  </interface>
</node>`;

const CadenceProxy = Gio.DBusProxy.makeProxyWrapper(IFACE_XML);

/* Owns the bus name watch, the proxy, and the property cache. The only writer
 * of session state. */
class CadenceClient {
    constructor(onChanged) {
        this._onChanged = onChanged;
        this._proxy = null;
        this._propsId = 0;
        this._watchId = 0;
        this._state = DISCONNECTED;
    }

    get state() {
        return this._state;
    }

    start() {
        this._watchId = Gio.bus_watch_name(
            Gio.BusType.SESSION, BUS_NAME, Gio.BusNameWatcherFlags.NONE,
            () => this._onAppeared(),
            () => this._onVanished());
    }

    stop() {
        if (this._propsId && this._proxy)
            this._proxy.disconnect(this._propsId);
        this._propsId = 0;
        this._proxy = null;

        if (this._watchId)
            Gio.bus_unwatch_name(this._watchId);
        this._watchId = 0;

        this._state = DISCONNECTED;
    }

    call(method) {
        if (!this._proxy)
            return;
        this._proxy[`${method}Async`]().catch(
            e => console.warn(`cadence: ${method} failed: ${e.message}`));
    }

    async _onAppeared() {
        try {
            const proxy = await CadenceProxy.newAsync(
                Gio.DBus.session, BUS_NAME, OBJECT_PATH, null);

            // enable() may have been torn down while this was in flight.
            if (this._watchId === 0) {
                proxy.run_dispose();
                return;
            }

            this._proxy = proxy;
            this._propsId = proxy.connect(
                'g-properties-changed', () => this._refresh());
            this._refresh();
        } catch (e) {
            console.warn(`cadence: proxy construction failed: ${e.message}`);
        }
    }

    _onVanished() {
        if (this._propsId && this._proxy)
            this._proxy.disconnect(this._propsId);
        this._propsId = 0;
        this._proxy = null;
        this._state = DISCONNECTED;
        this._onChanged();
    }

    /* g-properties-changed carries only the changed subset, so the cache is
     * rebuilt whole to keep every render on one coherent snapshot. */
    _refresh() {
        const p = this._proxy;
        if (!p)
            return;
        this._state = {
            available: true,
            sessionActive: !!p.SessionActive,
            phase: p.Phase ?? 'none',
            phaseEndsAt: Number(p.PhaseEndsAt ?? 0),
            remainingSeconds: Number(p.RemainingSeconds ?? 0),
            paused: !!p.Paused,
            tier: p.Tier ?? 'T0',
        };
        this._onChanged();
    }
}

const CadenceIndicator = GObject.registerClass(
class CadenceIndicator extends PanelMenu.Button {
    _init(onAction) {
        super._init(0.0, 'Cadence', false);

        this._box = new St.BoxLayout({style_class: 'panel-status-menu-box'});
        this._icon = new St.Icon({
            icon_name: 'alarm-symbolic',
            style_class: 'system-status-icon',
        });
        this._label = new St.Label({
            style_class: 'cadence-label',
            y_align: Clutter.ActorAlign.CENTER,
            visible: false,
        });
        this._box.add_child(this._icon);
        this._box.add_child(this._label);
        this.add_child(this._box);

        this._items = {};
        const actions = [
            ['start', 'Start Session'],
            ['stop', 'Stop Session'],
            ['pause', 'Pause'],
            ['resume', 'Resume'],
            ['skip', 'Skip Break'],
        ];
        for (const [key, text] of actions) {
            const item = new PopupMenu.PopupMenuItem(text);
            item.connect('activate', () => onAction(key));
            this.menu.addMenuItem(item);
            this._items[key] = item;
        }
    }

    render(display, sensitivity, warningClass) {
        this._label.visible = display.label !== '';
        this._label.text = display.label;

        for (const cls of ['cadence-warning-dark', 'cadence-warning-light']) {
            this._label.remove_style_class_name(cls);
            this._icon.remove_style_class_name(cls);
        }
        if (display.warning) {
            this._label.add_style_class_name(warningClass);
            this._icon.add_style_class_name(warningClass);
        }

        if (display.dimmed)
            this._box.add_style_class_name('cadence-dimmed');
        else
            this._box.remove_style_class_name('cadence-dimmed');

        for (const [key, item] of Object.entries(this._items))
            item.setSensitive(sensitivity[key]);
    }
});

export default class CadenceExtension extends Extension {
    enable() {
        this._tickId = 0;
        this._schemeId = 0;

        this._indicator = new CadenceIndicator(key => this._onAction(key));
        Main.panel.addToStatusArea(this.uuid, this._indicator);

        this._client = new CadenceClient(() => this._onStateChanged());
        this._client.start();

        this._settings = St.Settings.get();
        this._schemeId = this._settings.connect(
            'notify::color-scheme', () => this._render());

        this._render();
    }

    disable() {
        this._stopTick();

        if (this._schemeId && this._settings)
            this._settings.disconnect(this._schemeId);
        this._schemeId = 0;
        this._settings = null;

        this._client?.stop();
        this._client = null;

        this._indicator?.destroy();
        this._indicator = null;
    }

    _onAction(key) {
        const method = {
            start: 'StartSession',
            stop: 'StopSession',
            pause: 'Pause',
            resume: 'Resume',
            skip: 'SkipBreak',
        }[key];
        // No optimistic update: the display moves when PropertiesChanged says so.
        this._client?.call(method);
    }

    _onStateChanged() {
        const s = this._client.state;
        if (s.available && s.sessionActive && !s.paused)
            this._startTick();
        else
            this._stopTick();
        this._render();
    }

    _startTick() {
        if (this._tickId)
            return;
        this._tickId = GLib.timeout_add_seconds(GLib.PRIORITY_DEFAULT, 1, () => {
            this._render();
            return GLib.SOURCE_CONTINUE;
        });
    }

    _stopTick() {
        if (this._tickId)
            GLib.Source.remove(this._tickId);
        this._tickId = 0;
    }

    _render() {
        if (!this._indicator || !this._client)
            return;
        const state = this._client.state;
        const now = Math.floor(Date.now() / 1000);
        const dark = this._settings?.colorScheme === St.SystemColorScheme.PREFER_DARK;
        this._indicator.render(
            computeDisplay(state, now),
            menuSensitivity(state),
            dark ? 'cadence-warning-dark' : 'cadence-warning-light');
    }
}
