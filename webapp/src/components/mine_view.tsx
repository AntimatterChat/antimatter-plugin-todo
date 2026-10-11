// Copyright (c) 2026-present Antimatter contributors.
// See LICENSE for license information.

import React, {useState} from 'react';
import {useIntl} from 'react-intl';
import {useSelector} from 'react-redux';

import {getCurrentTeamId} from 'mattermost-redux/selectors/entities/teams';

import * as client from '../client';
import {messages} from '../messages';
import {refreshPersonal, setDraft, usePersonal, useTodoState} from '../state';
import type {AssignedTodo, Issue} from '../types/todo';
import Icon from '../ui/icon';
import {cx} from '../ui/web_ui';

import AddForm from './add_form';
import {Avatar, Name, UserPicker} from './people';
import TodoRow, {type Action} from './todo_row';

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

// MyRow is one of the user's own todos: done when checked off, which also tells whoever sent it.
function MyRow({issue, teamId, run}: {issue: Issue; teamId: string; run: Run}) {
    const intl = useIntl();
    const [editing, setEditing] = useState(false);
    const [picking, setPicking] = useState(false);
    const actions: Action[] = [
        {label: intl.formatMessage(messages.editTodo), icon: 'pen', onClick: () => setEditing(true)},
        {label: intl.formatMessage(messages.sendTo), icon: 'send', onClick: () => setPicking(true)},
        {label: intl.formatMessage(messages.deleteTodo), icon: 'trash', danger: true, onClick: () => run(() => client.remove(issue.id))},
    ];
    return (
        <TodoRow
            message={issue.message}
            description={issue.description}
            onToggle={() => run(() => client.complete(issue.id))}
            meta={(
                <Person
                    userId={issue.user_id}
                    message={messages.from}
                />
            )}
            permalink={issue.postPermalink}
            actions={actions}
            editing={editing}
            onEditingChange={setEditing}
            onSave={(message, description) => run(() => client.edit(issue.id, message, description))}
            picker={picking && (
                <UserPicker
                    teamId={teamId}
                    onPick={(u) => {
                        setPicking(false);
                        run(() => client.sendTo(issue.id, u.username));
                    }}
                    onClose={() => setPicking(false)}
                />
            )}
        />
    );
}

function AssignedRow({todo, run}: {todo: AssignedTodo; run: Run}) {
    const intl = useIntl();
    const open = () => {
        if (todo.team_name && window.WebappUtils?.browserHistory) {
            window.WebappUtils.browserHistory.push(`/${todo.team_name}/channels/${todo.channel_url_name}`);
        }
    };
    return (
        <TodoRow
            shared={true}
            message={todo.message}
            description={todo.description}
            onToggle={() => run(() => client.updateChannelTodo(todo.channel_id, todo.id, {done: true}))}
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
                    {intl.formatMessage(messages.inChannel, {channel: todo.channel_name})}
                </button>
            )}
            permalink={todo.post_permalink}
        />
    );
}

// MineView is the user's own todos, the channel todos assigned to them, and the todos they sent or
// were sent.
export default function MineView() {
    const intl = useIntl();
    const teamId = useSelector(getCurrentTeamId);
    const personal = usePersonal();
    const draft = useTodoState((s) => s.draft);
    const [error, setError] = useState('');
    const lists = personal.lists;

    const run: Run = async (call) => {
        setError('');
        try {
            await call();
        } catch (err) {
            setError(intl.formatMessage(messages.error, {error: (err as Error).message}));
        }
        await refreshPersonal();
    };

    return (
        <>
            <AddForm
                placeholder={intl.formatMessage(messages.addPlaceholder)}
                personLabel={intl.formatMessage(messages.sendTo)}
                teamId={teamId}
                draft={draft}
                onDraftUsed={() => setDraft(null)}
                onAdd={async ({message, description, person, draft: from}) => {
                    await client.addPersonal({message, description, send_to: person?.username, post_id: from?.postId, postPermalink: from?.postPermalink});
                    await refreshPersonal();
                }}
            />
            {(error || personal.error) && (
                <div
                    className='amt-error'
                    role='alert'
                >
                    {error || personal.error}
                </div>
            )}
            {!lists && !personal.error && <div className='amt-empty'>{'…'}</div>}
            {lists && (
                <>
                    <Section
                        title={intl.formatMessage(messages.mine)}
                        count={lists.my.length}
                    >
                        {lists.my.length === 0 && <div className='amt-empty'>{intl.formatMessage(messages.mineEmpty)}</div>}
                        {lists.my.map((issue) => (
                            <MyRow
                                key={issue.id}
                                issue={issue}
                                teamId={teamId}
                                run={run}
                            />
                        ))}
                    </Section>
                    {Boolean(personal.assigned?.length) && (
                        <Section
                            title={intl.formatMessage(messages.assigned)}
                            count={personal.assigned?.length}
                        >
                            {personal.assigned?.map((todo) => (
                                <AssignedRow
                                    key={todo.id}
                                    todo={todo}
                                    run={run}
                                />
                            ))}
                        </Section>
                    )}
                    {lists.in.length > 0 && (
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
                    {lists.out.length > 0 && (
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
            )}
        </>
    );
}
