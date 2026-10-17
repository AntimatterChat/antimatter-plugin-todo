// Copyright (c) 2026-present Antimatter contributors.
// See LICENSE for license information.

import React, {useEffect, useLayoutEffect, useRef, useState} from 'react';
import {createPortal} from 'react-dom';

import Icon, {type IconName} from '../ui/icon';
import {cx, isFusionUI} from '../ui/web_ui';

export type Action = {
    label: string;
    icon: IconName;
    onClick: () => void;
    danger?: boolean;
};

// The gap between the button and its menu, and the margin the menu keeps from the edges of the window.
const GAP = 4;
const MARGIN = 8;

// Layer is drawn over the page, in Fusion's layer (which Fusion styles) under the Fusion UI: what's
// in the panel's header, or in a row near the panel's edge, would be clipped.
export function Layer({children}: {children: React.ReactNode}) {
    return createPortal(
        <div className={(isFusionUI() ? 'am-layer ' : '') + 'amt-layer'}>{children}</div>,
        document.body,
    );
}

// usePlace places a popup under its anchor (over it when there's no room below), aligned with its
// right edge, inside the window.
export function usePlace(open: boolean, anchor: React.RefObject<HTMLElement | null>, popup: React.RefObject<HTMLElement | null>) {
    const [place, setPlace] = useState<React.CSSProperties>({visibility: 'hidden'});
    useLayoutEffect(() => {
        if (!open || !anchor.current) {
            return;
        }
        const button = anchor.current.getBoundingClientRect();
        const width = popup.current?.offsetWidth || 0;
        const height = popup.current?.offsetHeight || 0;
        const left = Math.max(MARGIN, Math.min(button.right - width, window.innerWidth - width - MARGIN));
        const below = button.bottom + GAP;
        const above = button.top - GAP - height;
        setPlace({left, top: below + height > window.innerHeight - MARGIN && above > MARGIN ? above : below});
    }, [open, anchor, popup]);
    return place;
}

// useDismiss closes a popup on a click outside it and its anchor, on Escape (only the popup, not the
// panel it's in), and when the window is resized.
export function useDismiss(open: boolean, close: () => void, refs: Array<React.RefObject<HTMLElement | null>>) {
    const refsRef = useRef(refs);
    refsRef.current = refs;
    useEffect(() => {
        if (!open) {
            return () => null;
        }
        const onDown = (e: MouseEvent) => {
            const target = e.target as Node;
            if (!refsRef.current.some((r) => r.current?.contains(target))) {
                close();
            }
        };
        const onKey = (e: KeyboardEvent) => {
            if (e.key === 'Escape') {
                e.stopPropagation();
                close();
            }
        };
        document.addEventListener('mousedown', onDown);
        document.addEventListener('keydown', onKey, true);
        window.addEventListener('resize', close);
        return () => {
            document.removeEventListener('mousedown', onDown);
            document.removeEventListener('keydown', onKey, true);
            window.removeEventListener('resize', close);
        };
    }, [open, close]);
}

// Menu is a ⋯ button opening a small menu of actions, drawn over the page.
export default function Menu({label, actions, small = false}: {label: string; actions: Action[]; small?: boolean}) {
    const [open, setOpen] = useState(false);
    const button = useRef<HTMLButtonElement>(null);
    const menu = useRef<HTMLDivElement>(null);
    const place = usePlace(open, button, menu);
    const close = React.useCallback(() => setOpen(false), []);
    useDismiss(open, close, [button, menu]);

    if (!actions.length) {
        return null;
    }
    return (
        <>
            <button
                ref={button}
                type='button'
                className={cx('icon-btn') + ' amt-icon-btn'}
                title={label}
                aria-label={label}
                aria-haspopup='menu'
                aria-expanded={open}
                onClick={() => setOpen(!open)}
            >
                <Icon
                    name='dots'
                    {...(small ? {size: 'sm' as const} : {})}
                />
            </button>
            {open && (
                <Layer>
                    <div
                        ref={menu}
                        className='amt-menu'
                        role='menu'
                        aria-label={label}
                        style={place}
                    >
                        {actions.map((a) => (
                            <button
                                key={a.label}
                                type='button'
                                role='menuitem'
                                className={a.danger ? 'amt-danger' : ''}
                                onClick={() => {
                                    setOpen(false);
                                    a.onClick();
                                }}
                            >
                                <Icon
                                    name={a.icon}
                                    size='sm'
                                />
                                {a.label}
                            </button>
                        ))}
                    </div>
                </Layer>
            )}
        </>
    );
}
