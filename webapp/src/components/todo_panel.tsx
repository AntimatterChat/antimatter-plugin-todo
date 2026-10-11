// Copyright (c) 2026-present Antimatter contributors.
// See LICENSE for license information.

import React from 'react';
import {useIntl} from 'react-intl';
import {useSelector} from 'react-redux';

import {getCurrentChannel} from 'mattermost-redux/selectors/entities/channels';

import {messages} from '../messages';
import {setView, useTodoState} from '../state';
import {cx} from '../ui/web_ui';

import ChannelView from './channel_view';
import MineView from './mine_view';

// TodoPanel is the todo app of the right-hand panel: the current channel's shared list, or the
// user's own lists.
export default function TodoPanel() {
    const intl = useIntl();
    const channel = useSelector(getCurrentChannel);
    const view = useTodoState((s) => s.view);
    const mineCount = useTodoState((s) => (s.personal.lists?.my.length || 0) + (s.personal.assigned?.length || 0) + (s.personal.lists?.in.length || 0));
    const conversation = channel?.type === 'D' || channel?.type === 'G';

    return (
        <div className={cx('app-panel') + ' amt-panel'}>
            <div
                className={cx('seg') + ' amt-seg'}
                role='tablist'
            >
                <button
                    type='button'
                    role='tab'
                    aria-selected={view === 'channel'}
                    className={view === 'channel' ? cx('on') : ''}
                    onClick={() => setView('channel')}
                >
                    {intl.formatMessage(conversation ? messages.viewConversation : messages.viewChannel)}
                </button>
                <button
                    type='button'
                    role='tab'
                    aria-selected={view === 'mine'}
                    className={view === 'mine' ? cx('on') : ''}
                    onClick={() => setView('mine')}
                >
                    {intl.formatMessage(messages.viewMine)}
                    {mineCount > 0 && <span className='amt-count'>{mineCount}</span>}
                </button>
            </div>
            {view === 'channel' ? (
                <ChannelView
                    key={channel?.id}
                    channel={channel}
                />
            ) : <MineView/>}
        </div>
    );
}
