// Copyright (c) 2026-present Antimatter contributors.
// See LICENSE for license information.

import React, {useState} from 'react';
import {useIntl} from 'react-intl';

import {messages} from '../messages';
import Icon from '../ui/icon';

import {DueMark} from './due';
import Menu, {type Action} from './menu';

export type {Action} from './menu';

// Text renders a todo's text as a message's Markdown, when the host offers its formatter.
export function Text({text}: {text: string}) {
    const utils = window.PostUtils;
    if (utils?.formatText && utils.messageHtmlToComponent) {
        return <>{utils.messageHtmlToComponent(utils.formatText(text, {singleline: false, mentionHighlight: false}), {mentionHighlight: false})}</>;
    }
    return <>{text}</>;
}

type Props = {
    message: string;
    description?: string;
    done?: boolean;

    // The day it's due, if any.
    due?: string;

    // The checkbox: absent for todos that can't be checked off from here.
    onToggle?: (() => void) | null;
    meta?: React.ReactNode;

    // A todo of a shared list (a channel's), not of someone's own lists: marked in the app's colour.
    shared?: boolean;
    permalink?: string;

    // Buttons shown on the row (e.g. Accept), and the actions of its menu.
    buttons?: Action[];
    actions?: Action[];

    // Editing in place, when allowed.
    onSave?: (message: string, description: string) => Promise<void>;
    editing?: boolean;
    onEditingChange?: (editing: boolean) => void;

    // A person picker or a date field, open under the row.
    picker?: React.ReactNode;
};

function EditForm({message, description, onSave, onCancel}: {message: string; description: string; onSave: (m: string, d: string) => Promise<void>; onCancel: () => void}) {
    const intl = useIntl();
    const [text, setText] = useState(message);
    const [details, setDetails] = useState(description);
    const [busy, setBusy] = useState(false);
    const submit = async (e: React.FormEvent) => {
        e.preventDefault();
        if (!text.trim() || busy) {
            return;
        }
        setBusy(true);
        try {
            await onSave(text.trim(), details.trim());
            onCancel();
        } finally {
            setBusy(false);
        }
    };
    return (
        <form
            className='amt-edit'
            onSubmit={submit}
            onKeyDown={(e) => {
                if (e.key === 'Escape') {
                    e.stopPropagation();
                    onCancel();
                }
            }}
        >
            <input
                autoFocus={true}
                value={text}
                maxLength={1000}
                aria-label={intl.formatMessage(messages.editTodo)}
                onChange={(e) => setText(e.target.value)}
            />
            <textarea
                value={details}
                maxLength={4000}
                rows={2}
                placeholder={intl.formatMessage(messages.description)}
                aria-label={intl.formatMessage(messages.details)}
                onChange={(e) => setDetails(e.target.value)}
            />
            <div className='amt-edit-actions'>
                <button
                    type='button'
                    className='amt-btn'
                    onClick={onCancel}
                >
                    {intl.formatMessage(messages.cancel)}
                </button>
                <button
                    type='submit'
                    className='amt-btn amt-primary'
                    disabled={busy || !text.trim()}
                >
                    {intl.formatMessage(messages.save)}
                </button>
            </div>
        </form>
    );
}

// TodoRow is a todo in a list: its checkbox, text, details, when it's due and the people and place
// it's about, with its actions in a menu.
export default function TodoRow({message, description, done, due, onToggle, meta, shared, permalink, buttons, actions, onSave, editing, onEditingChange, picker}: Props) {
    const intl = useIntl();

    if (editing && onSave) {
        return (
            <div className='amt-row amt-row-editing'>
                <EditForm
                    message={message}
                    description={description || ''}
                    onSave={onSave}
                    onCancel={() => onEditingChange?.(false)}
                />
            </div>
        );
    }

    const showDue = Boolean(due && !done);
    return (
        <div className={'amt-row' + (done ? ' amt-done' : '') + (shared ? ' amt-shared' : '')}>
            {onToggle ? (
                <button
                    type='button'
                    className='amt-check'
                    role='checkbox'
                    aria-checked={Boolean(done)}
                    aria-label={intl.formatMessage(done ? messages.reopen : messages.markDone)}
                    title={intl.formatMessage(done ? messages.reopen : messages.markDone)}
                    onClick={onToggle}
                >
                    {done && (
                        <Icon
                            name='check'
                            size='xs'
                        />
                    )}
                </button>
            ) : <span className='amt-bullet'/>}
            <div className='amt-body'>
                <div className='amt-text'><Text text={message}/></div>
                {description && <div className='amt-desc'><Text text={description}/></div>}
                {(meta || permalink || showDue) && (
                    <div className='amt-meta'>
                        {showDue && <DueMark due={due}/>}
                        {meta}
                        {permalink && (
                            <a
                                href={permalink}
                                onClick={(e) => {
                                    const url = new URL(permalink, window.location.origin);
                                    if (url.origin === window.location.origin && window.WebappUtils?.browserHistory) {
                                        e.preventDefault();
                                        window.WebappUtils.browserHistory.push(url.pathname);
                                    }
                                }}
                            >
                                <Icon
                                    name='link'
                                    size='xs'
                                />
                                {intl.formatMessage(messages.jump)}
                            </a>
                        )}
                    </div>
                )}
                {buttons && buttons.length > 0 && (
                    <div className='amt-row-buttons'>
                        {buttons.map((b) => (
                            <button
                                key={b.label}
                                type='button'
                                className={'amt-btn amt-small' + (b.danger ? ' amt-danger' : '')}
                                onClick={b.onClick}
                            >
                                {b.label}
                            </button>
                        ))}
                    </div>
                )}
                {picker}
            </div>
            {actions && actions.length > 0 && (
                <div className='amt-row-menu'>
                    <Menu
                        label={intl.formatMessage(messages.more)}
                        actions={actions}
                        small={true}
                    />
                </div>
            )}
        </div>
    );
}
