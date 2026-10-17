// Copyright (c) 2026-present Antimatter contributors.
// See LICENSE for license information.

import React, {useEffect, useRef, useState} from 'react';
import {useSelector} from 'react-redux';

import {getCurrentChannel} from 'mattermost-redux/selectors/entities/channels';

import {panelClosed, panelOpened, useTodoState} from '../state';

import DraftView from './draft_view';
import ListIndex from './list_index';
import ListView from './list_view';

// TodoPanel is the todo app of the right-hand panel: the lists (of the current channel, the
// favorites, the user's own), an open list, or a todo started from a message.
export default function TodoPanel() {
    const channel = useSelector(getCurrentChannel);

    // Back in another channel than the one it was closed in, the panel opens on that channel's lists
    useState(() => panelOpened(channel?.id || ''));
    const channelId = useRef(channel?.id || '');
    channelId.current = channel?.id || '';
    useEffect(() => () => panelClosed(channelId.current), []);

    const draft = useTodoState((s) => s.draft);
    const openListId = useTodoState((s) => s.openListId);

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
