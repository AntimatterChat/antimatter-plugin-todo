// Copyright (c) 2026-present Antimatter contributors.
// See LICENSE for license information.

import {useEffect, useSyncExternalStore} from 'react';

import * as client from './client';
import type {IndexKey} from './client';
import type {AssignedTodo, ListDetail, ListInfo, Lists} from './types/todo';

// A small store of the app: the lists of the tabs, the open list, the todos sent between people
// and those assigned to the user, refreshed by the plugin's websocket events.

type Requests = {lists?: Lists; assigned?: AssignedTodo[]; error?: string};

// A todo started from a message's "Add to todo" (see index.tsx), for the panel's form.
export type Draft = {
    message: string;
    channelId: string;
    postId: string;
    postPermalink: string;
};

export type View = 'channel' | 'favorites' | 'mine';

type Index = {lists?: ListInfo[]; error?: string};
type Detail = {list?: ListDetail; error?: string; status?: number};

type State = {
    view: View;
    openListId: string | null;
    draft: Draft | null;
    requests: Requests;
    indexes: Partial<Record<IndexKey, Index>>;
    details: Record<string, Detail>;
};

const initial = (): State => ({view: 'channel', openListId: null, draft: null, requests: {}, indexes: {}, details: {}});

let state: State = initial();
const listeners = new Set<() => void>();

// The channel the panel was closed in: opened again in another channel, it shows that channel's
// lists, not the list left open.
let closedIn: string | null = null;

// The panel is shown by the host (see index.tsx).
let showPanel: () => void = () => null;

// The app draws its header in the panel's header (TodoHeader, the title it gives the host): the row
// where the list of lists and the open list put their title and actions, while it's shown.
let headSlot: HTMLElement | null = null;

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
    state = initial();
    closedIn = null;
    headSlot = null;
    listeners.forEach((l) => l());
}

const message = (err: unknown) => (err instanceof Error ? err.message : String(err));

// Requests and assignments ------------------------------------------------------------------------

export async function refreshRequests(reminder = false) {
    try {
        const [lists, assigned] = await Promise.all([client.getRequests(reminder), client.getAssigned()]);
        set({requests: {lists: lists || {my: [], in: [], out: []}, assigned: assigned || []}});
    } catch (err) {
        set({requests: {...state.requests, error: message(err)}});
    }
}

export function useRequests() {
    const requests = useTodoState((s) => s.requests);
    const loaded = Boolean(requests.lists);
    useEffect(() => {
        if (!loaded) {
            refreshRequests();
        }
    }, [loaded]);
    return requests;
}

// Lists -------------------------------------------------------------------------------------------

export const channelKey = (channelId: string): IndexKey => `channel:${channelId}`;

export async function refreshIndex(key: IndexKey) {
    try {
        const lists = await client.listLists(key);
        set({indexes: {...state.indexes, [key]: {lists: lists || []}}});
    } catch (err) {
        set({indexes: {...state.indexes, [key]: {...state.indexes[key], error: message(err)}}});
    }
}

// useIndex returns the lists of a tab, loading them when shown.
export function useIndex(key: IndexKey | null) {
    const entry = useTodoState((s) => (key ? s.indexes[key] : null));
    useEffect(() => {
        if (key) {
            refreshIndex(key);
        }
    }, [key]);
    return entry;
}

export async function refreshList(listId: string) {
    try {
        const list = await client.getList(listId);
        set({details: {...state.details, [listId]: {list}}});
    } catch (err) {
        const status = (err as client.ClientError).status;
        set({details: {...state.details, [listId]: {...state.details[listId], error: message(err), status}}});
    }
}

export function useListDetail(listId: string) {
    const entry = useTodoState((s) => s.details[listId]);
    useEffect(() => {
        refreshList(listId);
    }, [listId]);
    return entry;
}

// refreshAfterChange refreshes what shows a list that changed: the list if it's loaded, and the
// tabs it may be in.
export function refreshAfterChange(listId: string, channelId?: string) {
    if (state.details[listId]) {
        refreshList(listId);
    }
    const keys: IndexKey[] = [channelId ? channelKey(channelId) : 'mine', 'favorites'];
    for (const key of keys) {
        if (state.indexes[key]) {
            refreshIndex(key);
        }
    }
}

// The panel ---------------------------------------------------------------------------------------

export function setView(view: View) {
    set({view, openListId: null});
}

export function openList(listId: string) {
    closedIn = null;
    set({openListId: listId});
}

export function closeList() {
    set({openListId: null});
}

export function setDraft(draft: Draft | null) {
    set({draft});
}

export function panelClosed(channelId: string) {
    closedIn = channelId;
}

export function panelOpened(channelId: string) {
    if (closedIn !== null && closedIn !== channelId) {
        state = {...state, view: 'channel', openListId: null};
    }
    closedIn = null;
}

// Where the panel is, for a panel opened in a new window (the host's setPanelState): an open list
// ('list/<id>'), or the tab it shows ('view/favorites', 'view/mine'; nothing for this channel's).
// A todo being added from a message isn't kept: it's a moment, not a place.
export const panelStateOf = (s: State) => {
    if (s.openListId) {
        return `list/${s.openListId}`;
    }
    return s.view === 'channel' ? '' : `view/${s.view}`;
};

// restorePanel puts the panel where it was in the window it was popped out of, as it first renders:
// quietly, as the panel reads it right after; panelRestored then tells the others (the header). The
// state comes from the window's address: anything this app didn't write opens the lists, and a list
// is fetched again as the person signed in here (one they can't open shows its not-found view).
export function restorePanel(panelState?: string): boolean {
    const [kind, id, ...rest] = (panelState || '').split('/');
    if (rest.length || !id) {
        return false;
    }
    if (kind === 'list' && (/^[a-z0-9]{26}$/).test(id)) {
        state = {...state, openListId: id, draft: null};
        closedIn = null;
        return true;
    }
    if (kind === 'view' && (id === 'favorites' || id === 'mine')) {
        state = {...state, view: id, openListId: null};
        closedIn = null;
        return true;
    }
    return false;
}

export function panelRestored() {
    listeners.forEach((l) => l());
}

export function setShowPanel(show: () => void) {
    showPanel = show;
}

// openDraft shows the panel with a todo started from a message, to add to a list.
export function openDraft(draft: Draft) {
    closedIn = null;
    set({draft});
    showPanel();
}

export function setHeadSlot(element: HTMLElement) {
    if (headSlot !== element) {
        headSlot = element;
        listeners.forEach((l) => l());
    }
}

// clearHeadSlot forgets the header's row as it goes away, unless another took its place.
export function clearHeadSlot(element: HTMLElement) {
    if (headSlot === element) {
        headSlot = null;
        listeners.forEach((l) => l());
    }
}

export function useHeadSlot() {
    return useSyncExternalStore(subscribe, () => headSlot);
}
