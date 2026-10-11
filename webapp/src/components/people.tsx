// Copyright (c) 2026-present Antimatter contributors.
// See LICENSE for license information.

import React, {useEffect, useRef, useState} from 'react';
import {useIntl} from 'react-intl';
import {useDispatch, useSelector} from 'react-redux';

import type {GlobalState} from '@mattermost/types/store';
import type {UserProfile} from '@mattermost/types/users';

import {UserTypes} from 'mattermost-redux/action_types';
import {Client4} from 'mattermost-redux/client';
import {getTeammateNameDisplaySetting} from 'mattermost-redux/selectors/entities/preferences';
import {getUser} from 'mattermost-redux/selectors/entities/users';
import {displayUsername} from 'mattermost-redux/utils/user_utils';

import {messages} from '../messages';

// The people being loaded, so that each is asked for once.
const loading = new Set<string>();

// useUser returns a person from the host's store, loading them into it when missing.
export function useUser(userId?: string): UserProfile | null {
    const dispatch = useDispatch();
    const user = useSelector((state: GlobalState) => (userId ? getUser(state, userId) : null));
    useEffect(() => {
        if (!userId || user || loading.has(userId)) {
            return;
        }
        loading.add(userId);
        Client4.getProfilesByIds([userId]).then(
            (profiles) => dispatch({type: UserTypes.RECEIVED_PROFILES_LIST, data: profiles}),
            () => null,
        ).finally(() => loading.delete(userId));
    }, [userId, user, dispatch]);
    return user;
}

export function useDisplayName(userId?: string): string {
    const user = useUser(userId);
    const setting = useSelector(getTeammateNameDisplaySetting);
    return user ? displayUsername(user, setting) : '';
}

export function Avatar({userId, size = 20}: {userId?: string; size?: number}) {
    const user = useUser(userId);
    if (!userId) {
        return null;
    }
    return (
        <img
            className='amt-avatar'
            src={Client4.getProfilePictureUrl(userId, user?.last_picture_update || 0)}
            width={size}
            height={size}
            alt=''
        />
    );
}

export function Name({userId}: {userId?: string}) {
    return <>{useDisplayName(userId) || '…'}</>;
}

// people are the ones to offer: real, active accounts.
const people = (users: UserProfile[]) => users.filter((u) => !u.is_bot && !u.delete_at).slice(0, 8);

type PickerProps = {
    teamId?: string;

    // Only people of this channel, when given.
    channelId?: string;
    onPick: (user: UserProfile) => void;
    onClose: () => void;
};

// UserPicker searches people to assign or send a todo to.
export function UserPicker({teamId, channelId, onPick, onClose}: PickerProps) {
    const intl = useIntl();
    const setting = useSelector(getTeammateNameDisplaySetting);
    const [term, setTerm] = useState('');
    const [users, setUsers] = useState<UserProfile[] | null>(null);
    const ref = useRef<HTMLDivElement>(null);

    useEffect(() => {
        let live = true;
        const timer = setTimeout(async () => {
            try {
                const result = await Client4.autocompleteUsers(term, teamId || '', channelId || '', {limit: 8});
                const found = channelId ? (result.users || []) : [...(result.users || []), ...(result.out_of_channel || [])];
                if (live) {
                    setUsers(people(found));
                }
            } catch {
                if (live) {
                    setUsers([]);
                }
            }
        }, 150);
        return () => {
            live = false;
            clearTimeout(timer);
        };
    }, [term, teamId, channelId]);

    useEffect(() => {
        const onDown = (e: MouseEvent) => {
            if (ref.current && !ref.current.contains(e.target as Node)) {
                onClose();
            }
        };
        document.addEventListener('mousedown', onDown);
        return () => document.removeEventListener('mousedown', onDown);
    }, [onClose]);

    return (
        <div
            ref={ref}
            className='amt-picker'
            role='dialog'
            aria-label={intl.formatMessage(messages.searchPeople)}
        >
            <input
                autoFocus={true}
                value={term}
                placeholder={intl.formatMessage(messages.searchPeople)}
                aria-label={intl.formatMessage(messages.searchPeople)}
                onChange={(e) => setTerm(e.target.value)}
                onKeyDown={(e) => {
                    if (e.key === 'Escape') {
                        e.stopPropagation();
                        onClose();
                    } else if (e.key === 'Enter' && users?.length) {
                        e.preventDefault();
                        onPick(users[0]);
                    }
                }}
            />
            <div
                className='amt-picker-list'
                role='listbox'
            >
                {users?.length === 0 && <div className='amt-muted'>{intl.formatMessage(messages.noPeople)}</div>}
                {users?.map((u) => (
                    <button
                        key={u.id}
                        type='button'
                        role='option'
                        aria-selected={false}
                        onClick={() => onPick(u)}
                    >
                        <Avatar userId={u.id}/>
                        <span>{displayUsername(u, setting)}</span>
                        <span className='amt-muted'>{'@' + u.username}</span>
                    </button>
                ))}
            </div>
        </div>
    );
}
