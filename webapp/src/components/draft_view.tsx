// Copyright (c) 2026-present Antimatter contributors.
// See LICENSE for license information.

import React, {useEffect, useState} from 'react';
import {useIntl} from 'react-intl';
import {useSelector} from 'react-redux';

import {getCurrentTeamId} from 'mattermost-redux/selectors/entities/teams';

import * as client from '../client';
import {messages} from '../messages';
import {openList, refreshAfterChange, refreshRequests, setDraft, type Draft} from '../state';
import type {Targets} from '../types/todo';
import Icon from '../ui/icon';
import {cx} from '../ui/web_ui';

import AddForm from './add_form';
import {HeadPortal} from './todo_header';

// DraftView adds a todo started from a message ("Add to todo"): which list it goes to (the one last
// used from the message's channel, else the channel's first list, else the user's default one),
// and the todo, with the message's text.
export default function DraftView({draft}: {draft: Draft}) {
    const intl = useIntl();
    const teamId = useSelector(getCurrentTeamId);
    const [targets, setTargets] = useState<Targets | null>(null);
    const [listId, setListId] = useState('');
    const [error, setError] = useState('');

    useEffect(() => {
        let live = true;
        client.getTargets(draft.channelId).then((t) => {
            if (live) {
                setTargets(t);
                setListId(t.default_id);
            }
        }, (err) => live && setError(intl.formatMessage(messages.error, {error: (err as Error).message})));
        return () => {
            live = false;
        };
    }, [draft.channelId, intl]);

    const cancel = () => setDraft(null);
    const target = targets?.lists.find((l) => l.id === listId);
    const shared = target ? target.scope !== 'personal' : false;

    return (
        <div className={cx('app-panel') + ' amt-panel'}>
            <HeadPortal>
                <button
                    type='button'
                    className='amt-icon-btn amt-back'
                    title={intl.formatMessage(messages.cancel)}
                    aria-label={intl.formatMessage(messages.cancel)}
                    onClick={cancel}
                >
                    <Icon name='back'/>
                    <Icon name='checkbox'/>
                </button>
                <span className='amt-title amt-title-static'>{intl.formatMessage(messages.addFromMessage)}</span>
            </HeadPortal>
            {error && (
                <div
                    className='amt-error'
                    role='alert'
                >
                    {error}
                </div>
            )}
            {!targets && !error && <div className={cx('empty') + ' amt-empty'}>{'…'}</div>}
            {targets && (
                <>
                    <label className='amt-field'>
                        <span>{intl.formatMessage(messages.pickList)}</span>
                        <select
                            value={listId}
                            onChange={(e) => setListId(e.target.value)}
                        >
                            {targets.lists.map((l) => (
                                <option
                                    key={l.id}
                                    value={l.id}
                                >
                                    {(l.icon ? l.icon + ' ' : '') + l.name + (l.scope === 'personal' ? '' : ' · ' + (l.channel_name || intl.formatMessage(messages.scopeConversation)))}
                                </option>
                            ))}
                        </select>
                    </label>
                    {target && (
                        <AddForm
                            key={target.id}
                            placeholder={intl.formatMessage(messages.addToList, {list: target.name})}
                            personLabel={intl.formatMessage(shared ? messages.assignTo : messages.sendTo)}
                            teamId={teamId}
                            pickerChannelId={shared ? target.channel_id : ''}
                            draft={draft}
                            onDraftUsed={cancel}
                            onAdd={async ({message, description, person, due}) => {
                                if (person && !shared) {
                                    await client.sendNew({message, description, username: person.username, postId: draft.postId});
                                    await refreshRequests();
                                    setDraft(null);
                                    return;
                                }
                                await client.addItem(target.id, {message, description, ...(due ? {due} : {}), ...(person ? {assignee_id: person.id} : {}), post_id: draft.postId, channel_id: draft.channelId});
                                refreshAfterChange(target.id, target.channel_id);
                                setDraft(null);
                                openList(target.id);
                            }}
                        />
                    )}
                </>
            )}
        </div>
    );
}
