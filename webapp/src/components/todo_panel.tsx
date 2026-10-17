// Copyright (c) 2026-present Antimatter contributors.
// See LICENSE for license information.

import React, {useEffect, useLayoutEffect, useRef, useState} from 'react';
import {useSelector} from 'react-redux';

import {getCurrentChannel} from 'mattermost-redux/selectors/entities/channels';

import {panelClosed, panelOpened, panelRestored, panelStateOf, restorePanel, useTodoState} from '../state';
import type {PanelProps} from '../types/host';

import DraftView from './draft_view';
import ListIndex from './list_index';
import ListView from './list_view';

// TodoPanel is the todo app of the right-hand panel: the lists (of the current channel, the
// favorites, the user's own), an open list, or a todo started from a message. The host (Fusion)
// is told where it is, and a panel opened in a new window starts there; other hosts give neither.
export default function TodoPanel({setPanelState, initialPanelState}: PanelProps = {}) {
    const channel = useSelector(getCurrentChannel);

    // Back in another channel than the one it was closed in, the panel opens on that channel's
    // lists. Opened in a new window, it starts where it was, before its first render.
    const [restored] = useState(() => {
        panelOpened(channel?.id || '');
        return restorePanel(initialPanelState);
    });
    useLayoutEffect(() => {
        if (restored) {
            panelRestored();
        }
    }, [restored]);
    const channelId = useRef(channel?.id || '');
    channelId.current = channel?.id || '';
    useEffect(() => () => panelClosed(channelId.current), []);

    const draft = useTodoState((s) => s.draft);
    const openListId = useTodoState((s) => s.openListId);
    const panelState = useTodoState(panelStateOf);
    useEffect(() => {
        setPanelState?.(panelState || null);
    }, [setPanelState, panelState]);

    return (
        <div className='amt-root'>
            {draft && (
                <DraftView
                    key={draft.postId}
                    draft={draft}
                />
            )}
            {!draft && openListId && (
                <ListView
                    key={openListId}
                    listId={openListId}
                    channel={channel}
                />
            )}
            {!draft && !openListId && <ListIndex channel={channel}/>}
        </div>
    );
}
