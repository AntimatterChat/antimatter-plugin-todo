// Copyright (c) 2026-present Antimatter contributors.
// See LICENSE for license information.

// Antimatter ships two web UIs: the classic one and Fusion. Under Fusion, the plugin draws the
// markup of Fusion's app panels (its am- classes, styled by Fusion); under the classic UI, the same
// markup with the plugin's own classic styles. Both UIs say which one is running before plugins
// load.

export type WebUI = 'classic' | 'fusion';

// isFusionUI returns whether the Fusion web UI is running.
export function isFusionUI(): boolean {
    return window.antimatterWebUI === 'fusion' || document.documentElement.dataset.amWebUi === 'fusion';
}

// cx returns the classes of the running UI for Fusion's class names: am-<name> under Fusion,
// amt-<name> (styled by the plugin) under the classic UI. Falsy names are skipped.
export function cx(...names: Array<string | false | null | undefined>): string {
    const prefix = isFusionUI() ? 'am-' : 'amt-';
    return names.filter(Boolean).map((name) => prefix + name).join(' ');
}
