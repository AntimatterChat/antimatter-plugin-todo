// Copyright (c) 2026-present Antimatter contributors.
// See LICENSE for license information.

import React, {useCallback, useEffect, useState} from 'react';
import {useIntl} from 'react-intl';

import * as client from '../client';
import {messages} from '../messages';
import type {FollowedList} from '../types/todo';
import Icon from '../ui/icon';
import {cx} from '../ui/web_ui';

import TodoRow from './todo_row';

// FollowingView is the open todos of the channels' lists the user follows, by channel.
export default function FollowingView() {
    const intl = useIntl();
    const [lists, setLists] = useState<FollowedList[] | null>(null);
    const [error, setError] = useState('');

    const load = useCallback(() => {
        client.getFollowing().then(setLists, (err) => setError(intl.formatMessage(messages.error, {error: (err as Error).message})));
    }, [intl]);
    useEffect(load, [load]);

    const open = (list: FollowedList) => {
        if (list.team_name && window.WebappUtils?.browserHistory) {
            window.WebappUtils.browserHistory.push(`/${list.team_name}/channels/${list.channel_url_name}`);
        }
    };

    if (error) {
        return <div className='amt-error'>{error}</div>;
    }
    if (!lists) {
        return <div className='amt-empty'>{'…'}</div>;
    }
    if (!lists.length) {
        return <div className='amt-empty'>{intl.formatMessage(messages.followingEmpty)}</div>;
    }
    return (
        <>
            {lists.map((list) => (
                <React.Fragment key={list.channel_id}>
                    <div className={cx('app-h')}>
                        <button
                            type='button'
                            className='amt-link amt-grow-link'
                            onClick={() => open(list)}
                        >
                            <Icon
                                name='hash'
                                size='xs'
                            />
                            {list.channel_name}
                        </button>
                        <span>{list.todos.length || ''}</span>
                    </div>
                    {list.todos.length === 0 && <div className='amt-empty'>{intl.formatMessage(messages.followingNone)}</div>}
                    <div className='amt-list'>
                        {list.todos.map((todo) => (
                            <TodoRow
                                key={todo.id}
                                shared={true}
                                message={todo.message}
                                description={todo.description}
                                onToggle={list.can_edit ? () => client.updateChannelTodo(list.channel_id, todo.id, {done: true}).then(load, (err) => setError((err as Error).message)) : null}
                            />
                        ))}
                    </div>
                </React.Fragment>
            ))}
        </>
    );
}
