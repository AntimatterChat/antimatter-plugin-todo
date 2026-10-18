// Copyright (c) 2026-present Antimatter contributors.
// See LICENSE for license information.

import React, {useEffect, useRef, useState} from 'react';
import {useIntl} from 'react-intl';
import {useSelector} from 'react-redux';

import type {Channel} from '@mattermost/types/channels';

import {getCurrentTeamId} from 'mattermost-redux/selectors/entities/teams';
import {getCurrentUserId} from 'mattermost-redux/selectors/entities/users';

import * as client from '../client';
import {messages} from '../messages';
import {closeList, refreshAfterChange, refreshRequests, useListDetail} from '../state';
import type {ListDetail} from '../types/todo';
import Icon from '../ui/icon';
import {cx} from '../ui/web_ui';

import AddForm from './add_form';
import IconPicker from './icon_picker';
import ItemRow from './item_row';
import Menu, {type Action} from './menu';
import ScopeChip, {FavoriteButton, ListTile} from './scope';
import {TodoToolbar} from './todo_toolbar';

function TitleInput({list, inputRef}: {list: ListDetail; inputRef: React.RefObject<HTMLInputElement | null>}) {
    const intl = useIntl();
    const [name, setName] = useState(list.name);
    const [focused, setFocused] = useState(false);

    useEffect(() => {
        if (!focused) {
            setName(list.name);
        }
    }, [list.name, focused]);

    const save = () => {
        setFocused(false);
        const trimmed = name.trim();
        if (!trimmed) {
            setName(list.name);
        } else if (trimmed !== list.name) {
            client.updateList(list.id, {name: trimmed}).then(() => refreshAfterChange(list.id, list.channel_id), () => setName(list.name));
        }
    };

    return (
        <input
            ref={inputRef}
            className='amt-title'
            value={name}
            readOnly={!list.can_rename}
            maxLength={100}
            aria-label={intl.formatMessage(messages.listName)}
            onFocus={() => setFocused(true)}
            onChange={(e) => setName(e.target.value)}
            onBlur={save}
            onKeyDown={(e) => {
                if (e.key === 'Enter') {
                    (e.target as HTMLInputElement).blur();
                } else if (e.key === 'Escape') {
                    e.stopPropagation();
                    setName(list.name);
                    (e.target as HTMLInputElement).blur();
                }
            }}
        />
    );
}

// ListView is an open list: back to the lists, its name, where it is, the star and its actions in
// its toolbar at the top of the panel; under it, the form adding a todo and its todos, the done
// ones folded.
export default function ListView({listId, channel}: {listId: string; channel?: Channel}) {
    const intl = useIntl();
    const me = useSelector(getCurrentUserId);
    const teamId = useSelector(getCurrentTeamId);
    const entry = useListDetail(listId);
    const [showDone, setShowDone] = useState(false);
    const [error, setError] = useState('');
    const [confirming, setConfirming] = useState(false);
    const [pickingIcon, setPickingIcon] = useState(false);
    const titleRef = useRef<HTMLInputElement>(null);
    const tileRef = useRef<HTMLButtonElement>(null);

    const back = (
        <button
            type='button'
            className='amt-icon-btn amt-back'
            title={intl.formatMessage(messages.back)}
            aria-label={intl.formatMessage(messages.back)}
            onClick={closeList}
        >
            <Icon name='back'/>
        </button>
    );

    const list = entry?.list;
    if (!list) {
        let failure = '';
        if (entry?.error) {
            // Gone, or not theirs (anymore): a list one can't open is "not found"
            failure = entry.status === 404 || entry.status === 403 ? intl.formatMessage(messages.listNotFound) : entry.error;
        }
        return (
            <div className={cx('app-panel') + ' amt-panel'}>
                <TodoToolbar>{back}</TodoToolbar>
                <div className={failure ? 'amt-error' : cx('empty') + ' amt-empty'}>{failure || '…'}</div>
            </div>
        );
    }

    const onError = (err: unknown) => setError(intl.formatMessage(messages.error, {error: (err as Error).message}));
    const changed = () => refreshAfterChange(list.id, list.channel_id);
    const patch = (change: Parameters<typeof client.updateList>[1]) => client.updateList(list.id, change).then(changed, onError);
    const open = list.items.filter((t) => !t.done_at);
    const done = list.items.filter((t) => t.done_at);
    const shared = list.scope !== 'personal';
    const archived = Boolean(list.archived_at);

    const actions: Action[] = [];
    if (list.can_rename) {
        actions.push({label: intl.formatMessage(messages.rename), icon: 'pen', onClick: () => titleRef.current?.focus()});
        actions.push({label: intl.formatMessage(messages.changeIcon), icon: 'smile', onClick: () => setPickingIcon(true)});
        if (!list.default) {
            actions.push(archived ? {label: intl.formatMessage(messages.unarchive), icon: 'archive', onClick: () => patch({archived: false})} : {label: intl.formatMessage(messages.archive), icon: 'archive', onClick: () => patch({archived: true})});
        }
    }
    if (list.can_delete) {
        actions.push({label: intl.formatMessage(messages.deleteList), icon: 'trash', danger: true, onClick: () => setConfirming(true)});
    }

    return (
        <div className={cx('app-panel') + ' amt-panel'}>
            <TodoToolbar>
                {back}
                <button
                    ref={tileRef}
                    type='button'
                    className='amt-tile-btn'
                    disabled={!list.can_rename}
                    aria-label={intl.formatMessage(messages.changeIcon)}
                    title={intl.formatMessage(messages.changeIcon)}
                    onClick={() => setPickingIcon(true)}
                >
                    <ListTile list={list}/>
                </button>
                <TitleInput
                    list={list}
                    inputRef={titleRef}
                />
                <ScopeChip
                    list={list}
                    compact={true}
                />
                <FavoriteButton list={list}/>
                <Menu
                    label={intl.formatMessage(messages.listMenu)}
                    actions={actions}
                />
            </TodoToolbar>
            {pickingIcon && (
                <IconPicker
                    anchor={tileRef}
                    current={list.icon}
                    onPick={(icon) => {
                        setPickingIcon(false);
                        patch({icon});
                    }}
                    onClose={() => setPickingIcon(false)}
                />
            )}
            {confirming && (
                <div
                    className='amt-confirm'
                    role='alertdialog'
                    aria-label={intl.formatMessage(messages.deleteList)}
                >
                    <span>{intl.formatMessage(shared ? messages.deleteConfirmShared : messages.deleteConfirm, {name: list.name})}</span>
                    <div className='amt-edit-actions'>
                        <button
                            type='button'
                            className='amt-btn'
                            onClick={() => setConfirming(false)}
                        >
                            {intl.formatMessage(messages.cancel)}
                        </button>
                        <button
                            type='button'
                            className='amt-btn amt-danger-fill'
                            onClick={() => client.deleteList(list.id).then(() => {
                                closeList();
                                changed();
                            }, onError)}
                        >
                            {intl.formatMessage(messages.deleteTodo)}
                        </button>
                    </div>
                </div>
            )}
            {error && (
                <div
                    className='amt-error'
                    role='alert'
                >
                    {error}
                </div>
            )}
            {archived && <div className='amt-muted amt-note'>{intl.formatMessage(messages.archivedNote)}</div>}
            {!archived && !list.can_edit && <div className='amt-muted amt-note'>{intl.formatMessage(messages.channelReadOnly)}</div>}
            {list.can_edit && (
                <AddForm
                    placeholder={intl.formatMessage(messages.addToList, {list: list.name})}
                    personLabel={intl.formatMessage(shared ? messages.assignTo : messages.sendTo)}
                    teamId={teamId}
                    pickerChannelId={shared ? list.channel_id : ''}
                    onDraftUsed={() => null}
                    onAdd={async ({message, description, person, due}) => {
                        if (person && !shared) {
                            // Sent: it's their todo, in their requests, not this list's
                            await client.sendNew({message, description, username: person.username});
                            await refreshRequests();
                            return;
                        }
                        await client.addItem(list.id, {message, description, ...(due ? {due} : {}), ...(person ? {assignee_id: person.id} : {}), ...(channel ? {channel_id: channel.id} : {})});
                        changed();
                    }}
                />
            )}
            {open.length === 0 && (
                <div className={cx('empty') + ' amt-empty'}>
                    <Icon name='checkbox'/>
                    <span>{intl.formatMessage(shared ? messages.channelEmpty : messages.mineEmpty)}</span>
                </div>
            )}
            <div className='amt-list'>
                {open.map((item, i) => (
                    <ItemRow
                        key={item.id}
                        item={item}
                        list={list}
                        me={me}
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
                            {done.map((item) => (
                                <ItemRow
                                    key={item.id}
                                    item={item}
                                    list={list}
                                    me={me}
                                    first={false}
                                    onError={onError}
                                />
                            ))}
                        </div>
                    )}
                </>
            )}
        </div>
    );
}
