// Copyright (c) 2026-present Antimatter contributors.
// See LICENSE for license information.

import React, {useState} from 'react';
import {useIntl} from 'react-intl';

import type {Channel} from '@mattermost/types/channels';

import * as client from '../client';
import type {IndexKey} from '../client';
import {messages} from '../messages';
import {channelKey, openList, refreshIndex, setView, useIndex, useTodoState, type View} from '../state';
import type {ListInfo} from '../types/todo';
import Icon from '../ui/icon';
import {cx} from '../ui/web_ui';

import {DueMark} from './due';
import Requests from './requests';
import ScopeChip, {ListTile} from './scope';
import {HeadPortal} from './todo_header';

// ListRow is a list in the list of lists: its icon, name, how much is left and what's due, and
// (in the favorites, which mix them) where it is.
function ListRow({list, showScope}: {list: ListInfo; showScope: boolean}) {
    const intl = useIntl();
    return (
        <button
            type='button'
            className={cx('app-row') + ' amt-list-row'}
            onClick={() => openList(list.id)}
        >
            <ListTile list={list}/>
            <span className='amt-list-main'>
                <b>{list.name}</b>
                <span className={cx('sub') + ' amt-sub'}>
                    {intl.formatMessage(messages.openCount, {count: list.open_count})}
                    <DueMark
                        due={list.earliest_due}
                        onlyUrgent={true}
                    />
                </span>
            </span>
            {showScope ? <ScopeChip list={list}/> : <span className='amt-list-count'>{list.open_count || ''}</span>}
        </button>
    );
}

// NewListForm names a new list, in the scope of the tab.
function NewListForm({channelId, onDone}: {channelId?: string; onDone: () => void}) {
    const intl = useIntl();
    const [name, setName] = useState('');
    const [busy, setBusy] = useState(false);
    const [error, setError] = useState('');
    const submit = async (e: React.FormEvent) => {
        e.preventDefault();
        if (!name.trim() || busy) {
            return;
        }
        setBusy(true);
        try {
            const list = await client.createList(name.trim(), channelId);
            refreshIndex(channelId ? channelKey(channelId) : 'mine');
            onDone();
            openList(list.id);
        } catch (err) {
            setError(intl.formatMessage(messages.error, {error: (err as Error).message}));
        } finally {
            setBusy(false);
        }
    };
    return (
        <form
            className='amt-add amt-new-list'
            onSubmit={submit}
            onKeyDown={(e) => {
                if (e.key === 'Escape') {
                    e.stopPropagation();
                    onDone();
                }
            }}
        >
            <div className='amt-add-row'>
                <input
                    autoFocus={true}
                    value={name}
                    maxLength={100}
                    placeholder={intl.formatMessage(messages.newListName)}
                    aria-label={intl.formatMessage(messages.newListName)}
                    onChange={(e) => setName(e.target.value)}
                />
                <button
                    type='submit'
                    className='amt-btn amt-primary'
                    disabled={busy || !name.trim()}
                >
                    {intl.formatMessage(messages.create)}
                </button>
                <button
                    type='button'
                    className='amt-icon-btn'
                    aria-label={intl.formatMessage(messages.cancel)}
                    title={intl.formatMessage(messages.cancel)}
                    onClick={onDone}
                >
                    <Icon name='x'/>
                </button>
            </div>
            {error && (
                <div
                    className='amt-error'
                    role='alert'
                >
                    {error}
                </div>
            )}
        </form>
    );
}

function Lists({index, showScope, empty}: {index?: {lists?: ListInfo[]; error?: string}; showScope: boolean; empty: string}) {
    const intl = useIntl();
    const [showArchived, setShowArchived] = useState(false);
    if (index?.error && !index.lists) {
        return <div className='amt-error'>{index.error}</div>;
    }
    if (!index?.lists) {
        return <div className={cx('empty') + ' amt-empty'}>{'…'}</div>;
    }
    const active = index.lists.filter((l) => !l.archived_at);
    const archived = index.lists.filter((l) => l.archived_at);
    return (
        <>
            {active.length === 0 && (
                <div className={cx('empty') + ' amt-empty'}>
                    <Icon name='checkbox'/>
                    <span>{empty}</span>
                </div>
            )}
            {active.length > 0 && (
                <div className='amt-list'>
                    {active.map((list) => (
                        <ListRow
                            key={list.id}
                            list={list}
                            showScope={showScope}
                        />
                    ))}
                </div>
            )}
            {archived.length > 0 && (
                <>
                    <button
                        type='button'
                        className={cx('app-h') + ' amt-toggle'}
                        aria-expanded={showArchived}
                        onClick={() => setShowArchived((s) => !s)}
                    >
                        <span className={cx('grow')}>{intl.formatMessage(messages.archivedLists, {count: archived.length})}</span>
                        <span aria-hidden='true'>{showArchived ? '▾' : '▸'}</span>
                    </button>
                    {showArchived && (
                        <div className='amt-list amt-archived'>
                            {archived.map((list) => (
                                <ListRow
                                    key={list.id}
                                    list={list}
                                    showScope={showScope}
                                />
                            ))}
                        </div>
                    )}
                </>
            )}
        </>
    );
}

// ListIndex is the list of lists: the tabs (this channel's, the favorites, the user's own), the
// + of the panel's header making a list in the tab's scope, and the tab's lists. Mine also has the
// todos assigned to the user and those sent to them and by them.
export default function ListIndex({channel}: {channel?: Channel}) {
    const intl = useIntl();
    const view = useTodoState((s) => s.view);
    const [creating, setCreating] = useState(false);
    const conversation = channel?.type === 'D' || channel?.type === 'G';
    const shown: View = channel || view !== 'channel' ? view : 'mine';
    let key: IndexKey | null = null;
    if (shown === 'channel' && channel) {
        key = channelKey(channel.id);
    } else if (shown !== 'channel') {
        key = shown;
    }
    const index = useIndex(key);
    const requestCount = useTodoState((s) => (s.requests.lists?.in.length || 0) + (s.requests.assigned?.length || 0));

    // The new list button: a list of the channel in its tab (unless it's archived), a personal one
    // in Mine; the favorites have none.
    let createLabel = '';
    if (shown === 'channel' && channel && !channel.delete_at) {
        createLabel = intl.formatMessage(conversation ? messages.newListConversation : messages.newListChannel);
    } else if (shown === 'mine') {
        createLabel = intl.formatMessage(messages.newListPersonal);
    }

    const tab = (v: View, label: string, count = 0) => (
        <button
            type='button'
            role='tab'
            aria-selected={shown === v}
            className={shown === v ? cx('on') : ''}
            onClick={() => {
                setCreating(false);
                setView(v);
            }}
        >
            {label}
            {count > 0 && <span className='amt-count'>{count}</span>}
        </button>
    );

    let empty = intl.formatMessage(messages.listsFavoritesEmpty);
    if (shown === 'channel') {
        empty = intl.formatMessage(conversation ? messages.listsConversationEmpty : messages.listsChannelEmpty);
    }

    return (
        <div className={cx('app-panel') + ' amt-panel'}>
            {createLabel && (
                <HeadPortal>
                    <span className='amt-grow'/>
                    <button
                        type='button'
                        className='amt-icon-btn amt-h-btn'
                        title={createLabel}
                        aria-label={createLabel}
                        aria-pressed={creating}
                        onClick={() => setCreating((c) => !c)}
                    >
                        <Icon name='plus'/>
                    </button>
                </HeadPortal>
            )}
            <div
                className={cx('seg') + ' amt-seg'}
                role='tablist'
            >
                {channel && tab('channel', intl.formatMessage(conversation ? messages.viewConversation : messages.viewChannel))}
                {tab('favorites', intl.formatMessage(messages.viewFavorites))}
                {tab('mine', intl.formatMessage(messages.viewMine), requestCount)}
            </div>
            {creating && createLabel && (
                <NewListForm
                    channelId={shown === 'channel' ? channel?.id : undefined}
                    onDone={() => setCreating(false)}
                />
            )}
            {key && (
                <Lists
                    index={index}
                    showScope={shown === 'favorites'}
                    empty={empty}
                />
            )}
            {shown === 'mine' && (
                <>
                    <div className='amt-muted amt-note'>{intl.formatMessage(messages.listsMineNote)}</div>
                    <Requests/>
                </>
            )}
        </div>
    );
}
