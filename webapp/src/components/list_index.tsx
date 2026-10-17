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
import Icon, {type IconName} from '../ui/icon';
import {cx, isFusionUI} from '../ui/web_ui';

import {DueMark} from './due';
import EmptyState from './fusion/empty_state';
import {iconTip} from './fusion/tip';
import Requests from './requests';
import ScopeChip, {ListTile} from './scope';

// ListRow is a list in the list of lists, as Notes and Whiteboard draw their rows: its tile, its name, how much is
// left and what's due, and (in the favorites, which mix them) where it is, or a lock if one can't change it.
function ListRow({list, showScope}: {list: ListInfo; showScope: boolean}) {
    const intl = useIntl();
    return (
        <button
            type='button'
            className={cx('app-row') + ' amt-list-row' + (list.archived_at ? ' amt-list-archived' : '')}
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
            {showScope ? <ScopeChip list={list}/> : !list.can_edit && <span className={cx('sub')}><Icon name='lock'/></span>}
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

type SectionProps = {
    showScope: boolean;

    // What the tab is: "Shared with {channel}", "Favourites" or "Personal lists", on the left of its header row
    title: string;
    index?: {lists?: ListInfo[]; error?: string} | null;

    // What the tab shows without lists: an icon (under Fusion), a title and a sentence.
    empty: {icon: IconName; title: string; text: string};

    // The + on the right of the header row, opening the form making a list in the tab's scope (under the row): none
    // in the favourites, nor in an archived channel. Under Fusion, the empty tab also offers it as its one action.
    create?: {label: string; open: boolean; onToggle: () => void; form: React.ReactNode} | null;
};

// Section is the lists of the tab picked above it, as Notes and Whiteboard show theirs: a header row (what the tab
// is, and the + making a list there), the new list's form when open, then the lists, or the tab's empty state, and
// the archived ones, folded.
function Section({title, index, empty, create, showScope}: SectionProps) {
    const intl = useIntl();
    const [showArchived, setShowArchived] = useState(false);
    const plusIcon = (
        <Icon
            name='plus'
            size='sm'
        />
    );
    const lists = index?.lists;
    const active = lists?.filter((l) => !l.archived_at) || [];
    const archived = lists?.filter((l) => l.archived_at) || [];
    const row = (list: ListInfo) => (
        <ListRow
            key={list.id}
            list={list}
            showScope={showScope}
        />
    );

    let body: React.ReactNode;
    if (active.length) {
        body = active.map(row);
    } else if (lists) {
        body = (
            <EmptyState
                icon={<Icon name={empty.icon}/>}
                title={empty.title}
                text={empty.text}
                {...(create && !create.open && isFusionUI() ? {action: {label: create.label, onClick: create.onToggle, icon: plusIcon}} : {})}
                classicClassName={cx('todo-empty')}
                data-testid='todo-empty'
            />
        );
    } else if (index?.error) {
        body = <div className='amt-error'>{index.error}</div>;
    } else {
        body = (
            <EmptyState
                busy={true}
                text={intl.formatMessage(messages.listsLoading)}
                classicClassName={cx('todo-empty')}
                data-testid='todo-empty'
            />
        );
    }
    return (
        <>
            <div
                className={cx('app-h')}
                data-testid='todo-section-head'
            >
                <span className={cx('grow')}>{title}</span>
                {create && (
                    <button
                        type='button'
                        className={cx('icon-btn', 'todo-h-btn')}
                        {...iconTip(create.label)}
                        aria-pressed={create.open}
                        onClick={create.onToggle}
                    >
                        {plusIcon}
                    </button>
                )}
            </div>
            {create?.open && create.form}
            {body}
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
                    {showArchived && archived.map(row)}
                </>
            )}
        </>
    );
}

// ListIndex is the list of lists, laid out as Notes' and Whiteboard's: the tabs (this channel's, the favorites, the
// user's own), the tab's header row (what it is, and the + making a list there, its form opening under the row), and
// the tab's lists. Mine also has the todos assigned to the user and those sent to them and by them.
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

    // The tab's + opens the new list's form: a list of the channel in its tab (unless it's archived), a personal one
    // in Mine; the favorites have none.
    let createLabel = '';
    if (shown === 'channel' && channel && !channel.delete_at) {
        createLabel = intl.formatMessage(conversation ? messages.newListConversation : messages.newListChannel);
    } else if (shown === 'mine') {
        createLabel = intl.formatMessage(messages.newListPersonal);
    }
    const create = createLabel ? {
        label: createLabel,
        open: creating,
        onToggle: () => setCreating((c) => !c),
        form: (
            <NewListForm
                channelId={shown === 'channel' ? channel?.id : ''}
                onDone={() => setCreating(false)}
            />
        ),
    } : null;

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

    const channelName = conversation ? intl.formatMessage(messages.listsConversationName) : channel?.display_name || '';
    let section: Omit<SectionProps, 'index' | 'create'> | null = null;
    if (shown === 'favorites') {
        section = {
            title: intl.formatMessage(messages.listsFavorites),
            empty: {icon: 'star', title: intl.formatMessage(messages.listsFavoritesEmptyTitle), text: intl.formatMessage(messages.listsFavoritesEmpty)},
            showScope: true,
        };
    } else if (shown === 'channel' && channel) {
        section = {
            title: intl.formatMessage(messages.listsChannel, {channel: channelName}),
            empty: {icon: 'checkbox', title: intl.formatMessage(messages.listsChannelEmptyTitle), text: intl.formatMessage(conversation ? messages.listsConversationEmptyText : messages.listsChannelEmptyText)},
            showScope: false,
        };
    } else if (shown === 'mine') {
        section = {
            title: intl.formatMessage(messages.listsPersonal),
            empty: {icon: 'lock', title: intl.formatMessage(messages.listsPersonalEmptyTitle), text: intl.formatMessage(messages.listsMineNote)},
            showScope: false,
        };
    }

    return (
        <div className={cx('app-panel', 'todo-list') + ' amt-panel'}>
            <div
                className={cx('seg', 'todo-seg')}
                role='tablist'
            >
                {channel && tab('channel', intl.formatMessage(conversation ? messages.viewConversation : messages.viewChannel))}
                {tab('favorites', intl.formatMessage(messages.viewFavorites))}
                {tab('mine', intl.formatMessage(messages.viewMine), requestCount)}
            </div>
            {section && (
                <Section
                    key={key || ''}
                    {...section}
                    index={index}
                    create={create}
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
