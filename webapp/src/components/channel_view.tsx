// Copyright (c) 2026-present Antimatter contributors.
// See LICENSE for license information.

import React, {useState} from 'react';
import {useIntl} from 'react-intl';
import {useSelector} from 'react-redux';

import type {Channel} from '@mattermost/types/channels';

import {getCurrentUserId} from 'mattermost-redux/selectors/entities/users';

import * as client from '../client';
import {messages} from '../messages';
import {refreshChannel, setDraft, useChannelList, useTodoState} from '../state';
import type {ChannelTodo, ChannelTodoPatch} from '../types/todo';
import {cx} from '../ui/web_ui';

import AddForm from './add_form';
import {Avatar, Name, UserPicker} from './people';
import TodoRow, {type Action} from './todo_row';

type RowProps = {
    todo: ChannelTodo;
    channel: Channel;
    canEdit: boolean;
    canDelete: boolean;
    first: boolean;
    onError: (err: unknown) => void;
};

function ChannelTodoRow({todo, channel, canEdit, canDelete, first, onError}: RowProps) {
    const intl = useIntl();
    const [editing, setEditing] = useState(false);
    const [picking, setPicking] = useState(false);
    const done = Boolean(todo.done_at);

    const update = async (patch: ChannelTodoPatch) => {
        try {
            await client.updateChannelTodo(channel.id, todo.id, patch);
            await refreshChannel(channel.id);
        } catch (err) {
            onError(err);
        }
    };

    const actions: Action[] = [];
    if (canEdit && !done) {
        actions.push({label: intl.formatMessage(messages.editTodo), icon: 'pen', onClick: () => setEditing(true)});
        actions.push({label: intl.formatMessage(messages.assignTo), icon: 'user-plus', onClick: () => setPicking(true)});
        if (todo.assignee_id) {
            actions.push({label: intl.formatMessage(messages.unassign), icon: 'x', onClick: () => update({assignee_id: ''})});
        }
        if (!first) {
            actions.push({label: intl.formatMessage(messages.moveTop), icon: 'pin-top', onClick: () => update({position: 0})});
        }
    }
    if (canDelete) {
        actions.push({
            label: intl.formatMessage(messages.deleteTodo),
            icon: 'trash',
            danger: true,
            onClick: async () => {
                try {
                    await client.deleteChannelTodo(channel.id, todo.id);
                    await refreshChannel(channel.id);
                } catch (err) {
                    onError(err);
                }
            },
        });
    }

    let meta: React.ReactNode = null;
    if (done && todo.done_by) {
        meta = <span>{intl.formatMessage(messages.doneBy, {name: <Name userId={todo.done_by}/>})}</span>;
    } else if (todo.assignee_id) {
        meta = (
            <span className='amt-who'>
                <Avatar
                    userId={todo.assignee_id}
                    size={16}
                />
                {intl.formatMessage(messages.assignedTo, {name: <Name userId={todo.assignee_id}/>})}
            </span>
        );
    }

    return (
        <TodoRow
            shared={true}
            message={todo.message}
            description={todo.description}
            done={done}
            onToggle={canEdit ? () => update({done: !done}) : null}
            meta={meta}
            permalink={todo.post_permalink}
            actions={actions}
            editing={editing}
            onEditingChange={setEditing}
            onSave={(message, description) => update({message, description})}
            picker={picking && (
                <UserPicker
                    teamId={channel.team_id}
                    channelId={channel.id}
                    onPick={(u) => {
                        setPicking(false);
                        update({assignee_id: u.id});
                    }}
                    onClose={() => setPicking(false)}
                />
            )}
        />
    );
}

// ChannelView is the todo list of the current channel, shared by its members.
export default function ChannelView({channel}: {channel?: Channel}) {
    const intl = useIntl();
    const me = useSelector(getCurrentUserId);
    const entry = useChannelList(channel?.id);
    const draft = useTodoState((s) => s.draft);
    const [showDone, setShowDone] = useState(false);
    const [error, setError] = useState('');

    if (!channel) {
        return <div className='amt-empty'>{intl.formatMessage(messages.noChannel)}</div>;
    }
    const list = entry?.list;
    const open = list?.todos.filter((t) => !t.done_at) || [];
    const done = list?.todos.filter((t) => t.done_at) || [];
    const conversation = channel.type === 'D' || channel.type === 'G';
    const name = conversation ? intl.formatMessage(messages.viewConversation).toLowerCase() : '~' + channel.display_name;
    const onError = (err: unknown) => setError(intl.formatMessage(messages.error, {error: (err as Error).message}));
    const channelDraft = draft && draft.channelId === channel.id ? draft : null;

    return (
        <>
            {list?.can_edit && (
                <AddForm
                    placeholder={intl.formatMessage(messages.addPlaceholderChannel, {channel: conversation ? intl.formatMessage(messages.viewConversation).toLowerCase() : channel.display_name})}
                    personLabel={intl.formatMessage(messages.assignTo)}
                    teamId={channel.team_id}
                    pickerChannelId={channel.id}
                    draft={channelDraft}
                    onDraftUsed={() => setDraft(null)}
                    onAdd={async ({message, description, person, draft: from}) => {
                        await client.addChannelTodo(channel.id, {message, description, assignee_id: person?.id, post_id: from?.postId});
                        await refreshChannel(channel.id);
                    }}
                />
            )}
            {error && (
                <div
                    className='amt-error'
                    role='alert'
                >
                    {error}
                </div>
            )}
            <div className={cx('app-h')}>
                <span className={cx('grow')}>{intl.formatMessage(messages.channelOpen, {channel: name})}</span>
                <span>{open.length || ''}</span>
            </div>
            {entry?.error && !list && <div className='amt-error'>{entry.error}</div>}
            {list && !list.can_edit && <div className='amt-muted amt-note'>{intl.formatMessage(messages.channelReadOnly)}</div>}
            {list && open.length === 0 && <div className='amt-empty'>{intl.formatMessage(messages.channelEmpty)}</div>}
            {!list && !entry?.error && <div className='amt-empty'>{'…'}</div>}
            <div className='amt-list'>
                {open.map((t, i) => (
                    <ChannelTodoRow
                        key={t.id}
                        todo={t}
                        channel={channel}
                        canEdit={Boolean(list?.can_edit)}
                        canDelete={Boolean(list?.can_edit && (t.creator_id === me || list.can_manage))}
                        first={i === 0}
                        onError={onError}
                    />
                ))}
            </div>
            {done.length > 0 && (
                <>
                    <button
                        type='button'
                        className={cx('app-h') + ' amt-toggle'}
                        aria-expanded={showDone}
                        onClick={() => setShowDone((s) => !s)}
                    >
                        <span className={cx('grow')}>{intl.formatMessage(messages.done, {count: done.length})}</span>
                        <span aria-hidden='true'>{showDone ? '▾' : '▸'}</span>
                    </button>
                    {showDone && (
                        <div className='amt-list'>
                            {done.map((t) => (
                                <ChannelTodoRow
                                    key={t.id}
                                    todo={t}
                                    channel={channel}
                                    canEdit={Boolean(list?.can_edit)}
                                    canDelete={Boolean(list?.can_edit && (t.creator_id === me || list.can_manage))}
                                    first={false}
                                    onError={onError}
                                />
                            ))}
                        </div>
                    )}
                </>
            )}
        </>
    );
}
