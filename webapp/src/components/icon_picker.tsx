// Copyright (c) 2026-present Antimatter contributors.
// See LICENSE for license information.

import React, {useCallback, useRef} from 'react';
import {useIntl} from 'react-intl';

import {messages} from '../messages';

import {Layer, useDismiss, usePlace} from './menu';

// The emoji a list can have.
export const LIST_ICONS = [
    '📋', '✅', '📌', '⭐', '🔥', '🚀', '🎯', '💡',
    '🐛', '🛠️', '📦', '🧪', '📅', '⏰', '📝', '📣',
    '🛒', '🏠', '✈️', '🎉', '💬', '📚', '💰', '❤️',
];

type Props = {
    anchor: React.RefObject<HTMLElement | null>;
    current?: string;
    onPick: (icon: string) => void;
    onClose: () => void;
};

// IconPicker offers the emoji a list can have, under its anchor.
export default function IconPicker({anchor, current, onPick, onClose}: Props) {
    const intl = useIntl();
    const popup = useRef<HTMLDivElement>(null);
    const place = usePlace(true, anchor, popup);
    const close = useCallback(() => onClose(), [onClose]);
    useDismiss(true, close, [anchor, popup]);
    return (
        <Layer>
            <div
                ref={popup}
                className='amt-icons'
                role='dialog'
                aria-label={intl.formatMessage(messages.changeIcon)}
                style={place}
            >
                <div className='amt-icons-grid'>
                    {LIST_ICONS.map((icon) => (
                        <button
                            key={icon}
                            type='button'
                            className={icon === current ? 'amt-on' : ''}
                            aria-pressed={icon === current}
                            onClick={() => onPick(icon)}
                        >
                            {icon}
                        </button>
                    ))}
                </div>
                <button
                    type='button'
                    className='amt-btn amt-small'
                    onClick={() => onPick('')}
                >
                    {intl.formatMessage(messages.noIcon)}
                </button>
            </div>
        </Layer>
    );
}
