// Copyright (c) 2026-present Antimatter contributors.
// See LICENSE for license information.

import React, {useEffect, useState} from 'react';
import {useIntl} from 'react-intl';

import * as client from '../client';
import {messages} from '../messages';
import {refreshAfterChange} from '../state';
import type {ListInfo} from '../types/todo';
import Icon from '../ui/icon';

// ScopeChip tells at a glance whose a list is: a channel's (its name), a conversation's, or the
// user's own (a lock, outlined). Compact, in the open list's header, it's only the icon, with the
// same tooltip.
export default function ScopeChip({list, compact = false}: {list: ListInfo; compact?: boolean}) {
    const intl = useIntl();
    let tip: string;
    let label: string;
    let icon: 'lock' | 'users' | 'hash';
    switch (list.scope) {
    case 'personal':
        tip = intl.formatMessage(messages.scopePersonalTip);
        label = intl.formatMessage(messages.scopePersonal);
        icon = 'lock';
        break;
    case 'conversation':
        tip = intl.formatMessage(messages.scopeConversationTip);
        label = intl.formatMessage(messages.scopeConversation);
        icon = 'users';
        break;
    default:
        tip = intl.formatMessage(messages.scopeSharedTip, {channel: list.channel_name || list.channel_url_name || ''});
        label = list.channel_name || '';
        icon = 'hash';
    }
    return (
        <span
            className={'amt-scope amt-scope-' + list.scope + (compact ? ' amt-scope-compact' : '')}
            title={tip}
            {...(compact ? {role: 'img', 'aria-label': tip} : {})}
        >
            <Icon
                name={icon}
                size='xs'
            />
            {!compact && <span className='amt-scope-name'>{label}</span>}
        </span>
    );
}

// ListTile is a list's emoji, or the app's checklist, on a tile: filled for a shared list, outlined
// with a lock for the user's own.
export function ListTile({list}: {list: Pick<ListInfo, 'icon' | 'scope'>}) {
    const personal = list.scope === 'personal';
    return (
        <span
            className={'amt-tile' + (personal ? ' amt-tile-personal' : '')}
            aria-hidden='true'
        >
            {list.icon ? <span className='amt-tile-emoji'>{list.icon}</span> : <Icon name='checkbox'/>}
            {personal && (
                <span className='amt-tile-lock'>
                    <Icon
                        name='lock'
                        size='xs'
                    />
                </span>
            )}
        </span>
    );
}

// FavoriteButton stars a list, to find it among the favorites from any channel.
export function FavoriteButton({list}: {list: ListInfo}) {
    const intl = useIntl();
    const [favorite, setFavorite] = useState(list.favorite);
    useEffect(() => setFavorite(list.favorite), [list.id, list.favorite]);
    const label = intl.formatMessage(favorite ? messages.favoriteRemove : messages.favoriteAdd);
    return (
        <button
            type='button'
            className={'amt-star' + (favorite ? ' amt-on' : '')}
            title={label}
            aria-label={label}
            aria-pressed={favorite}
            onClick={() => {
                setFavorite(!favorite);
                client.setFavorite(list.id, !favorite).then(
                    () => refreshAfterChange(list.id, list.channel_id),
                    () => setFavorite(favorite),
                );
            }}
        >
            <Icon name='star'/>
        </button>
    );
}
