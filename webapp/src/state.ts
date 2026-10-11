// Copyright (c) 2026-present Antimatter contributors.
// See LICENSE for license information.

import {useEffect, useSyncExternalStore} from 'react';

import * as client from './client';
import type {AssignedTodo, ChannelList, Lists} from './types/todo';

// A small store of the todos: the user's own lists, the channel todos assigned to them, and the
// lists of the channels they looked at, refreshed by the plugin's websocket events.

type Personal = {lists?: Lists; assigned?: AssignedTodo[]; error?: string};

// A todo started from a message's "Add to todo" (see index.tsx), for the panel's form.
export type Draft = {
    message: string;
    channelId: string;
    postId: string;
    postPermalink: string;
};

export type View = 'channel' | 'mine';

type State = {
    personal: Personal;
    channels: Record<string, {list?: ChannelList; error?: string}>;
    view: View;
    draft: Draft | null;
};

let state: State = {personal: {}, channels: {}, view: 'channel', draft: null};
const listeners = new Set<() => void>();

function set(change: Partial<State>) {
    state = {...state, ...change};
    listeners.forEach((l) => l());
}

function subscribe(l: () => void) {
    listeners.add(l);
    return () => {
        listeners.delete(l);
    };
}

export function useTodoState<T>(select: (s: State) => T): T {
    return useSyncExternalStore(subscribe, () => select(state));
}

export function getState() {
    return state;
}

// For tests.
export function resetState() {
    state = {personal: {}, channels: {}, view: 'channel', draft: null};
    listeners.forEach((l) => l());
}

const message = (err: unknown) => (err instanceof Error ? err.message : String(err));

export async function refreshPersonal(reminder = false) {
    try {
        const [lists, assigned] = await Promise.all([client.getLists(reminder), client.getAssigned()]);
        set({personal: {lists: lists || {my: [], in: [], out: []}, assigned: assigned || []}});
    } catch (err) {
        set({personal: {...state.personal, error: message(err)}});
    }
}

export async function refreshChannel(channelId: string) {
    try {
        const list = await client.getChannelTodos(channelId);
        set({channels: {...state.channels, [channelId]: {list}}});
    } catch (err) {
        set({channels: {...state.channels, [channelId]: {...state.channels[channelId], error: message(err)}}});
    }
}

// refreshLoadedChannel refreshes a channel's list if the user looked at it (websocket events).
export function refreshLoadedChannel(channelId: string) {
    if (state.channels[channelId]) {
        refreshChannel(channelId);
    }
}

export function useChannelList(channelId?: string) {
    const entry = useTodoState((s) => (channelId ? s.channels[channelId] : null));
    useEffect(() => {
        if (channelId) {
            refreshChannel(channelId);
        }
    }, [channelId]);
    return entry;
}

export function usePersonal() {
    const personal = useTodoState((s) => s.personal);
    const loaded = Boolean(personal.lists);
    useEffect(() => {
        if (!loaded) {
            refreshPersonal();
        }
    }, [loaded]);
    return personal;
}

export function setView(view: View) {
    set({view});
}

export function setDraft(draft: Draft | null) {
    set({draft});
}

// The panel is shown by the host (see index.tsx).
let showPanel: () => void = () => null;

export function setShowPanel(show: () => void) {
    showPanel = show;
}

// openDraft shows the panel with a todo started from a message, in the view of its channel.
export function openDraft(draft: Draft) {
    set({draft, view: 'channel'});
    showPanel();
}
