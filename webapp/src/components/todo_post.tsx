// Copyright (c) 2026-present Antimatter contributors.
// See LICENSE for license information.

import React, {useState} from 'react';
import {useIntl} from 'react-intl';

import type {Post} from '@mattermost/types/posts';

import * as client from '../client';
import {messages} from '../messages';
import {refreshIndex, refreshRequests, useRequests} from '../state';

import {Text, webLink} from './todo_row';

type TodoProps = {
    message?: string;
    todo?: string;
    postPermalink?: string;
    issueId?: string;
};

// TodoPost is the bot's message about a todo someone sent the user: they accept it (into their
// default list), mark it done or decline it from here while it's waiting for their answer.
export default function TodoPost({post}: {post: Post}) {
    const intl = useIntl();
    const {message, todo, issueId} = (post.props || {}) as TodoProps;
    const postPermalink = webLink((post.props as TodoProps | undefined)?.postPermalink);
    const requests = useRequests();
    const [error, setError] = useState('');
    const waiting = Boolean(issueId && requests.lists?.in.some((i) => i.id === issueId));

    const run = async (call: () => Promise<unknown>) => {
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
        <div className='amt-post'>
            {message && <div className='amt-post-title'><Text text={message}/></div>}
            <div className='amt-post-todo'>
                {todo && <Text text={todo}/>}
                {postPermalink && (
                    <a
                        className='amt-post-link'
                        href={postPermalink}
                    >
                        {intl.formatMessage(messages.jump)}
                    </a>
                )}
                {waiting && issueId && (
                    <div className='amt-row-buttons'>
                        <button
                            type='button'
                            className='amt-btn amt-small amt-primary'
                            onClick={() => run(() => client.accept(issueId as string))}
                        >
                            {intl.formatMessage(messages.accept)}
                        </button>
                        <button
                            type='button'
                            className='amt-btn amt-small'
                            onClick={() => run(() => client.complete(issueId as string))}
                        >
                            {intl.formatMessage(messages.markDone)}
                        </button>
                        <button
                            type='button'
                            className='amt-btn amt-small amt-danger'
                            onClick={() => run(() => client.remove(issueId as string))}
                        >
                            {intl.formatMessage(messages.decline)}
                        </button>
                    </div>
                )}
                {!waiting && requests.lists && issueId && <div className='amt-muted'>{intl.formatMessage(messages.postAccepted)}</div>}
                {error && (
                    <div
                        className='amt-error'
                        role='alert'
                    >
                        {error}
                    </div>
                )}
            </div>
        </div>
    );
}
