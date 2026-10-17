// Copyright (c) 2026-present Antimatter contributors.
// See LICENSE for license information.

import React, {useEffect, useState} from 'react';
import {useIntl} from 'react-intl';

import type {UserProfile} from '@mattermost/types/users';

import {messages} from '../messages';
import type {Draft} from '../state';
import Icon from '../ui/icon';

import {DueMark} from './due';
import {Avatar, Name, UserPicker} from './people';

export type NewTodo = {
    message: string;
    description: string;
    person: UserProfile | null;

    // The day it's due (YYYY-MM-DD), or empty.
    due: string;
    draft?: Draft | null;
};

type Props = {
    placeholder: string;

    // The label of the person button: assign (channel) or send (own lists); none without one.
    personLabel?: string;
    teamId?: string;
    pickerChannelId?: string;
    draft?: Draft | null;
    onAdd: (todo: NewTodo) => Promise<void>;
    onDraftUsed: () => void;
};

// AddForm adds a todo: its text, details, when it's due and who it's for; it starts from a message's
// text when the todo comes from the message's "Add to todo". A todo sent to someone (a personal
// list's, with a person) has no due date: it becomes their request.
export default function AddForm({placeholder, personLabel, teamId, pickerChannelId, draft, onAdd, onDraftUsed}: Props) {
    const intl = useIntl();

    // A todo from a message starts with its text, from the first render
    const [message, setMessage] = useState(draft?.message || '');
    const [description, setDescription] = useState('');
    const [details, setDetails] = useState(false);
    const [person, setPerson] = useState<UserProfile | null>(null);
    const [picking, setPicking] = useState(false);
    const [due, setDue] = useState('');
    const [dating, setDating] = useState(false);
    const sending = Boolean(person && !pickerChannelId);
    const [busy, setBusy] = useState(false);
    const [error, setError] = useState('');

    useEffect(() => {
        if (draft) {
            setMessage(draft.message);
        }
    }, [draft]);

    const reset = () => {
        setMessage('');
        setDescription('');
        setDetails(false);
        setPerson(null);
        setDue('');
        setDating(false);
        onDraftUsed();
    };

    const submit = async (e: React.FormEvent) => {
        e.preventDefault();
        if (!message.trim() || busy) {
            return;
        }
        setBusy(true);
        setError('');
        try {
            await onAdd({message: message.trim(), description: description.trim(), person, due: sending ? '' : due, draft});
            reset();
        } catch (err) {
            setError(intl.formatMessage(messages.error, {error: (err as Error).message}));
        } finally {
            setBusy(false);
        }
    };

    return (
        <form
            className='amt-add'
            onSubmit={submit}
        >
            {draft && (
                <div className='amt-from'>
                    <Icon
                        name='link'
                        size='xs'
                    />
                    {intl.formatMessage(messages.fromMessage)}
                    <button
                        type='button'
                        className='amt-icon-btn amt-tiny'
                        aria-label={intl.formatMessage(messages.cancel)}
                        onClick={reset}
                    >
                        <Icon
                            name='x'
                            size='xs'
                        />
                    </button>
                </div>
            )}
            <div className='amt-add-row'>
                <input
                    value={message}
                    maxLength={1000}
                    placeholder={placeholder}
                    aria-label={placeholder}
                    onChange={(e) => setMessage(e.target.value)}
                />
                <button
                    type='submit'
                    className='amt-btn amt-primary'
                    disabled={busy || !message.trim()}
                >
                    {intl.formatMessage(sending ? messages.send : messages.add)}
                </button>
            </div>
            {(message || draft || details) && (
                <div className='amt-add-options'>
                    <button
                        type='button'
                        className={'amt-chip' + (details ? ' amt-on' : '')}
                        onClick={() => setDetails((d) => !d)}
                    >
                        <Icon
                            name='pen'
                            size='xs'
                        />
                        {intl.formatMessage(messages.details)}
                    </button>
                    {!sending && (
                        <button
                            type='button'
                            className={'amt-chip' + (due || dating ? ' amt-on' : '')}
                            onClick={() => (due ? setDue('') : setDating((d) => !d))}
                        >
                            {due ? <DueMark due={due}/> : (
                                <>
                                    <Icon
                                        name='calendar'
                                        size='xs'
                                    />
                                    {intl.formatMessage(messages.dueDate)}
                                </>
                            )}
                            {due && (
                                <Icon
                                    name='x'
                                    size='xs'
                                />
                            )}
                        </button>
                    )}
                    {personLabel && (
                        <button
                            type='button'
                            className={'amt-chip' + (person ? ' amt-on' : '')}
                            onClick={() => (person ? setPerson(null) : setPicking(true))}
                        >
                            {person ? <Avatar userId={person.id}/> : (
                                <Icon
                                    name='user-plus'
                                    size='xs'
                                />
                            )}
                            {person ? <Name userId={person.id}/> : personLabel}
                            {person && (
                                <Icon
                                    name='x'
                                    size='xs'
                                />
                            )}
                        </button>
                    )}
                </div>
            )}
            {dating && !due && !sending && (
                <input
                    type='date'
                    className='amt-date'
                    autoFocus={true}
                    aria-label={intl.formatMessage(messages.dueDate)}
                    onChange={(e) => {
                        if (e.target.value) {
                            setDue(e.target.value);
                            setDating(false);
                        }
                    }}
                />
            )}
            {picking && (
                <UserPicker
                    teamId={teamId}
                    channelId={pickerChannelId}
                    onPick={(u) => {
                        setPerson(u);
                        setPicking(false);
                    }}
                    onClose={() => setPicking(false)}
                />
            )}
            {details && (
                <textarea
                    value={description}
                    maxLength={4000}
                    rows={3}
                    placeholder={intl.formatMessage(messages.description)}
                    aria-label={intl.formatMessage(messages.details)}
                    onChange={(e) => setDescription(e.target.value)}
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
        </form>
    );
}
