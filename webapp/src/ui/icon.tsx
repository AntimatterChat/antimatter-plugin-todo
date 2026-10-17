// Copyright (c) 2026-present Antimatter contributors.
// See LICENSE for license information.

import React from 'react';

import {cx, isFusionUI} from './web_ui';

// The icons used by the plugin: Fusion's icon sprite names, and the classic UI's compass icons.
const classicIcons = {
    check: 'check',
    checkbox: 'checkbox-marked-outline',
    plus: 'plus',
    dots: 'dots-vertical',
    x: 'close',
    trash: 'trash-can-outline',
    pen: 'pencil-outline',
    'user-plus': 'account-plus-outline',
    send: 'send',
    link: 'link-variant',
    hash: 'pound',
    star: 'star-outline',
    'pin-top': 'arrow-collapse-up',
    bell: 'bell-outline',
    chev: 'chevron-down',
    back: 'chevron-left',
    lock: 'lock-outline',
    users: 'account-multiple-outline',
    calendar: 'calendar-outline',
    smile: 'emoticon-happy-outline',
    archive: 'archive-outline',
} as const;

// The names of Fusion's sprite for the plugin's icons, where they differ: the back chevron is the
// sprite's chevron, turned (styles.css), and archiving its folder.
const fusionIcons: Partial<Record<keyof typeof classicIcons, string>> = {
    back: 'chev',
    archive: 'folder',
};

export type IconName = keyof typeof classicIcons;

type Props = {
    name: IconName;
    size?: 'sm' | 'xs';
};

export default function Icon({name, size}: Props) {
    if (isFusionUI()) {
        return (
            <svg
                className={cx('ic', size, name === 'back' && 'ic-back') + ' amt-ic-' + name}
                aria-hidden='true'
            >
                <use href={`#am-i-${fusionIcons[name] || name}`}/>
            </svg>
        );
    }
    return (
        <i
            className={`icon icon-${classicIcons[name]}`}
            aria-hidden='true'
        />
    );
}
