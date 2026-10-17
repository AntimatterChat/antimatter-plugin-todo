// Copyright (c) 2026-present Antimatter contributors.
// See LICENSE for license information.

import React, {useLayoutEffect, useState} from 'react';
import {createPortal} from 'react-dom';
import {useIntl} from 'react-intl';

import {messages} from '../messages';
import {clearHeadSlot, setHeadSlot, useHeadSlot, useTodoState} from '../state';

// TodoHeader is the title the app gives the host's panel header: "Todo" over the lists (whose + is
// in the tab's header row, under the tabs), and a row where the open list draws its own header
// (back, the list's name and its actions), so that a list has one header, the panel's, as a note
// does.
export default function TodoHeader() {
    const intl = useIntl();
    const open = useTodoState((s) => Boolean(s.openListId || s.draft));
    const [slot, setSlot] = useState<HTMLSpanElement | null>(null);

    useLayoutEffect(() => {
        if (!slot) {
            return () => null;
        }
        setHeadSlot(slot);
        return () => clearHeadSlot(slot);
    }, [slot]);

    return (
        <span className={'amt-head' + (open ? ' amt-head-open' : '')}>
            {!open && <span className='amt-head-label'>{intl.formatMessage(messages.app)}</span>}
            <span
                ref={setSlot}
                className='amt-head-slot'
            />
        </span>
    );
}

// HeadPortal shows its children in the panel's header, or, on a host that doesn't show the app's
// title, in a bar at the top of the panel.
export function HeadPortal({children}: {children: React.ReactNode}) {
    const slot = useHeadSlot();
    if (slot) {
        return createPortal(children, slot);
    }
    return <div className='amt-bar'>{children}</div>;
}
