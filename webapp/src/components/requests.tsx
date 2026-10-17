// Copyright (c) 2026-present Antimatter contributors.
// See LICENSE for license information.

import React, {useState} from 'react';
import {useIntl} from 'react-intl';

import * as client from '../client';
import {messages} from '../messages';
import {refreshIndex, refreshRequests, useRequests} from '../state';
import type {AssignedTodo} from '../types/todo';
import Icon from '../ui/icon';
import {cx} from '../ui/web_ui';

import {Avatar, Name} from './people';
import TodoRow from './todo_row';

type Run = (call: () => Promise<unknown>) => Promise<void>;

function Section({title, count, children}: {title: string; count?: number; children: React.ReactNode}) {
    return (
        <>
            <div className={cx('app-h')}>
                <span className={cx('grow')}>{title}</span>
                <span>{count || ''}</span>
            </div>
            <div className='amt-list'>{children}</div>
        </>
    );
}

function Person({userId, message}: {userId?: string; message: typeof messages.from}) {
    const intl = useIntl();
    if (!userId) {
        return null;
    }
    return (
        <span className='amt-who'>
            <Avatar
                userId={userId}
                size={16}
            />
            {intl.formatMessage(message, {name: <Name userId={userId}/>})}
        </span>
    );
}

// AssignedRow is a todo of a channel's list assigned to the user: checked off from here, and a link
// to its channel.
function AssignedRow({todo, run}: {todo: AssignedTodo; run: Run}) {
    const intl = useIntl();
    const open = () => {
        if (todo.team_name && window.WebappUtils?.browserHistory) {
            window.WebappUtils.browserHistory.push(`/${todo.team_name}/channels/${todo.channel_url_name}`);
        }
    };
    const list = (todo.list_icon ? todo.list_icon + ' ' : '') + todo.list_name;
    return (
        <TodoRow
            shared={true}
            message={todo.message}
            description={todo.description}
            due={todo.due}
            onToggle={() => run(() => client.updateItem(todo.list_id, todo.id, {done: true}))}
            meta={(
                <button
                    type='button'
                    className='amt-link'
                    onClick={open}
                >
                    <Icon
                        name='hash'
                        size='xs'
                    />
                    {todo.channel_name ? intl.formatMessage(messages.inList, {list, channel: todo.channel_name}) : list}
                </button>
            )}
            permalink={todo.post_permalink}
        />
    );
}

// Requests are, in the Mine tab, the todos assigned to the user in channels, the todos sent to them
// (accepted into their default list) and those they sent.
export default function Requests() {
    const intl = useIntl();
    const requests = useRequests();
    const [error, setError] = useState('');
    const lists = requests.lists;

    const run: Run = async (call) => {
        setError('');
        try {
            await call();
        } catch (err) {
            setError(intl.formatMessage(messages.error, {error: (err as Error).message}));
        }
        await refreshRequests();
        refreshIndex('mine');
    };

    return (
        <>
            {(error || requests.error) && (
                <div
                    className='amt-error'
                    role='alert'
                >
                    {error || requests.error}
                </div>
            )}
            {Boolean(requests.assigned?.length) && (
                <Section
                    title={intl.formatMessage(messages.assigned)}
                    count={requests.assigned?.length}
                >
                    {requests.assigned?.map((todo) => (
                        <AssignedRow
                            key={todo.id}
                            todo={todo}
                            run={run}
                        />
                    ))}
                </Section>
            )}
            {lists && lists.in.length > 0 && (
                <Section
                    title={intl.formatMessage(messages.requests)}
                    count={lists.in.length}
                >
                    {lists.in.map((issue) => (
                        <TodoRow
                            key={issue.id}
                            message={issue.message}
                            description={issue.description}
                            meta={(
                                <Person
                                    userId={issue.user_id}
                                    message={messages.from}
                                />
                            )}
                            permalink={issue.postPermalink}
                            buttons={[
                                {label: intl.formatMessage(messages.accept), icon: 'check', onClick: () => run(() => client.accept(issue.id))},
                                {label: intl.formatMessage(messages.markDone), icon: 'checkbox', onClick: () => run(() => client.complete(issue.id))},
                                {label: intl.formatMessage(messages.decline), icon: 'x', danger: true, onClick: () => run(() => client.remove(issue.id))},
                            ]}
                        />
                    ))}
                </Section>
            )}
            {lists && lists.out.length > 0 && (
                <Section
                    title={intl.formatMessage(messages.sent)}
                    count={lists.out.length}
                >
                    {lists.out.map((issue) => (
                        <TodoRow
                            key={issue.id}
                            message={issue.message}
                            description={issue.description}
                            meta={(
                                <Person
                                    userId={issue.user_id}
                                    message={messages.to}
                                />
                            )}
                            permalink={issue.postPermalink}
                            actions={[
                                {label: intl.formatMessage(messages.bump), icon: 'bell', onClick: () => run(() => client.bump(issue.id))},
                                {label: intl.formatMessage(messages.cancelSent), icon: 'trash', danger: true, onClick: () => run(() => client.remove(issue.id))},
                            ]}
                        />
                    ))}
                </Section>
            )}
        </>
    );
}
