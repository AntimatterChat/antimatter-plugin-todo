// Copyright (c) 2026-present Antimatter contributors.
// See LICENSE for license information.

import React, {useState} from 'react';
import {useIntl} from 'react-intl';
import {useSelector} from 'react-redux';

import type {GlobalState} from '@mattermost/types/store';

import {getChannel} from 'mattermost-redux/selectors/entities/channels';
import {getCurrentTeamId} from 'mattermost-redux/selectors/entities/teams';

import * as client from '../client';
import {messages} from '../messages';
import {refreshAfterChange, refreshRequests} from '../state';
import type {Item, ItemPatch, ListDetail} from '../types/todo';

import type {Action} from './menu';
import {Avatar, Name, UserPicker} from './people';
import TodoRow from './todo_row';

type Props = {
    item: Item;
    list: ListDetail;
    me: string;
    first: boolean;
    onError: (err: unknown) => void;
};

function Person({userId, message}: {userId: string; message: typeof messages.from}) {
    const intl = useIntl();
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

// ItemRow is a todo of an open list. A channel's list's todos are assigned to its members; a
// personal list's are sent to someone instead (they leave the list for the person's requests).
export default function ItemRow({item, list, me, first, onError}: Props) {
    const intl = useIntl();
    const channel = useSelector((state: GlobalState) => (list.channel_id ? getChannel(state, list.channel_id) : null));
    const currentTeamId = useSelector(getCurrentTeamId);
    const [editing, setEditing] = useState(false);
    const [picking, setPicking] = useState<'assign' | 'send' | 'due' | null>(null);
    const done = Boolean(item.done_at);
    const shared = list.scope !== 'personal';

    const changed = () => refreshAfterChange(list.id, list.channel_id);
    const run = async (call: () => Promise<unknown>) => {
        try {
            await call();
        } catch (err) {
            onError(err);
        }
        changed();
    };
    const update = (patch: ItemPatch) => run(() => client.updateItem(list.id, item.id, patch));

    const actions: Action[] = [];
    if (list.can_edit && !done) {
        actions.push({label: intl.formatMessage(messages.editTodo), icon: 'pen', onClick: () => setEditing(true)});
        if (shared) {
            actions.push({label: intl.formatMessage(messages.assignTo), icon: 'user-plus', onClick: () => setPicking('assign')});
            if (item.assignee_id) {
                actions.push({label: intl.formatMessage(messages.unassign), icon: 'x', onClick: () => update({assignee_id: ''})});
            }
        } else if (!item.from_user_id) {
            actions.push({label: intl.formatMessage(messages.sendTo), icon: 'send', onClick: () => setPicking('send')});
        }
        actions.push({label: intl.formatMessage(messages.setDue), icon: 'calendar', onClick: () => setPicking('due')});
        if (item.due) {
            actions.push({label: intl.formatMessage(messages.dueClear), icon: 'x', onClick: () => update({due: ''})});
        }
        if (!first) {
            actions.push({label: intl.formatMessage(messages.moveTop), icon: 'pin-top', onClick: () => update({position: 0})});
        }
    }
    if (list.can_edit && (item.creator_id === me || list.can_manage)) {
        actions.push({
            label: intl.formatMessage(messages.deleteTodo),
            icon: 'trash',
            danger: true,
            onClick: () => run(() => client.deleteItem(list.id, item.id)),
        });
    }

    let meta: React.ReactNode = null;
    if (done && item.done_by) {
        meta = <span>{intl.formatMessage(messages.doneBy, {name: <Name userId={item.done_by}/>})}</span>;
    } else if (item.assignee_id) {
        meta = (
            <Person
                userId={item.assignee_id}
                message={messages.assignedTo}
            />
        );
    } else if (item.from_user_id) {
        meta = (
            <Person
                userId={item.from_user_id}
                message={messages.from}
            />
        );
    }

    let picker: React.ReactNode = null;
    if (picking === 'assign' || picking === 'send') {
        picker = (
            <UserPicker
                teamId={channel?.team_id || currentTeamId}
                channelId={picking === 'assign' ? list.channel_id : undefined}
                onPick={(u) => {
                    setPicking(null);
                    if (picking === 'assign') {
                        update({assignee_id: u.id});
                    } else {
                        run(() => client.sendItem(list.id, item.id, u.username).then(() => refreshRequests()));
                    }
                }}
                onClose={() => setPicking(null)}
            />
        );
    } else if (picking === 'due') {
        picker = (
            <input
                type='date'
                className='amt-date'
                autoFocus={true}
                defaultValue={item.due || ''}
                aria-label={intl.formatMessage(messages.dueDate)}
                onKeyDown={(e) => {
                    if (e.key === 'Escape') {
                        e.stopPropagation();
                        setPicking(null);
                    }
                }}
                onChange={(e) => {
                    if (e.target.value) {
                        setPicking(null);
                        update({due: e.target.value});
                    }
                }}
            />
        );
    }

    return (
        <TodoRow
            shared={shared}
            message={item.message}
            description={item.description}
            done={done}
            due={item.due}
            onToggle={list.can_edit ? () => update({done: !done}) : null}
            meta={meta}
            permalink={item.post_permalink}
            actions={actions}
            editing={editing}
            onEditingChange={setEditing}
            onSave={async (message, description) => {
                await client.updateItem(list.id, item.id, {message, description});
                changed();
            }}
            picker={picker}
        />
    );
}
