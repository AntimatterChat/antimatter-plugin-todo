// Copyright (c) 2026-present Antimatter contributors.
// See LICENSE for license information.

import {Client4} from 'mattermost-redux/client';

import manifest from './manifest';
import type {AssignedTodo, Item, ItemPatch, ListDetail, ListInfo, ListPatch, Lists, Targets} from './types/todo';

export class ClientError extends Error {
    status: number;

    constructor(message: string, status: number) {
        super(message);
        this.status = status;
    }
}

const base = () => `${window.basename || ''}/plugins/${manifest.id}`;

async function request<T>(method: string, path: string, body?: unknown, headers?: Record<string, string>): Promise<T> {
    const options: {method: string; body?: string; headers?: Record<string, string>} = {method, headers};
    if (body !== null && typeof body !== 'undefined') {
        options.body = JSON.stringify(body);
    }
    const response = await fetch(base() + path, Client4.getOptions(options));
    if (!response.ok) {
        let message = response.statusText;
        try {
            const data = await response.json();
            message = data.details || data.error || message;
        } catch {
            // Not JSON
        }
        throw new ClientError(message, response.status);
    }
    const text = await response.text();
    return (text ? JSON.parse(text) : null) as T;
}

// Todos sent between people ---------------------------------------------------------------------

// getRequests returns the todos sent to and by the user; with reminder, the server also sends the
// daily reminder when one is due (in the user's time zone).
export function getRequests(reminder = false) {
    return request<Lists>('GET', '/lists' + (reminder ? '?reminder=true' : ''), null, {'X-Timezone-Offset': String(new Date().getTimezoneOffset())});
}

export const complete = (id: string) => request('POST', '/complete', {id});
export const remove = (id: string) => request('POST', '/remove', {id});

// accept moves a todo sent to the user into one of their lists: their default one, unless listId.
export const accept = (id: string, listId?: string) => request('POST', '/accept', listId ? {id, list_id: listId} : {id});
export const bump = (id: string) => request('POST', '/bump', {id});

// sendNew sends a new todo to someone (by username), from a message when postId is given.
export function sendNew(todo: {message: string; description?: string; username: string; postId?: string}) {
    return request('POST', '/add', {message: todo.message, description: todo.description, send_to: todo.username, post_id: todo.postId});
}

// Lists ---------------------------------------------------------------------------------------------

// The lists of a tab: the user's own, their favorites, or a channel's.
export type IndexKey = 'mine' | 'favorites' | `channel:${string}`;

export function listLists(key: IndexKey) {
    if (key.startsWith('channel:')) {
        return request<ListInfo[]>('GET', `/api/v1/lists?scope=channel&channel_id=${key.slice('channel:'.length)}`);
    }
    return request<ListInfo[]>('GET', `/api/v1/lists?scope=${key}`);
}

// createList makes a list of a channel, or a personal one without channelId.
export function createList(name: string, channelId?: string, icon?: string) {
    return request<ListDetail>('POST', '/api/v1/lists', {name, channel_id: channelId || '', icon: icon || ''});
}

export const getList = (id: string) => request<ListDetail>('GET', `/api/v1/lists/${id}`);
export const updateList = (id: string, patch: ListPatch) => request<ListDetail>('PATCH', `/api/v1/lists/${id}`, patch);
export const deleteList = (id: string) => request('DELETE', `/api/v1/lists/${id}`);
export const setFavorite = (id: string, favorite: boolean) => request(favorite ? 'PUT' : 'DELETE', `/api/v1/lists/${id}/favorite`);

export const addItem = (listId: string, item: ItemPatch) => request<Item>('POST', `/api/v1/lists/${listId}/items`, item);
export const updateItem = (listId: string, id: string, patch: ItemPatch) => request<Item>('PATCH', `/api/v1/lists/${listId}/items/${id}`, patch);
export const deleteItem = (listId: string, id: string) => request('DELETE', `/api/v1/lists/${listId}/items/${id}`);

// sendItem sends a todo of a personal list to someone: it leaves the list.
export const sendItem = (listId: string, id: string, username: string) => request('POST', `/api/v1/lists/${listId}/items/${id}/send`, {username});

// getTargets returns the lists the user can add a todo to from a channel, and the one to offer.
export const getTargets = (channelId: string) => request<Targets>('GET', `/api/v1/targets?channel_id=${channelId}`);

export const getAssigned = () => request<AssignedTodo[]>('GET', '/api/v1/assigned');
