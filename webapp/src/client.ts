// Copyright (c) 2026-present Antimatter contributors.
// See LICENSE for license information.

import {Client4} from 'mattermost-redux/client';

import manifest from './manifest';
import type {AssignedTodo, ChannelList, ChannelTodo, ChannelTodoPatch, Lists} from './types/todo';

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

// Personal lists ----------------------------------------------------------------------------------

// getLists returns the user's lists; with reminder, the server also sends the daily reminder when
// one is due (in the user's time zone).
export function getLists(reminder = false) {
    return request<Lists>('GET', '/lists' + (reminder ? '?reminder=true' : ''), null, {'X-Timezone-Offset': String(new Date().getTimezoneOffset())});
}

export type AddPersonal = {
    message: string;
    description?: string;

    // A username to send the todo to, else the todo is the user's own.
    send_to?: string;
    post_id?: string;
    postPermalink?: string;
};

export function addPersonal(todo: AddPersonal) {
    return request('POST', '/add', todo);
}

export const complete = (id: string) => request('POST', '/complete', {id});
export const remove = (id: string) => request('POST', '/remove', {id});
export const accept = (id: string) => request('POST', '/accept', {id});
export const bump = (id: string) => request('POST', '/bump', {id});
export const edit = (id: string, message: string, description: string) => request('PUT', '/edit', {id, message, description});
export const sendTo = (id: string, username: string) => request('POST', '/change_assignment', {id, send_to: username});

// Channel lists -----------------------------------------------------------------------------------

export function getChannelTodos(channelId: string) {
    return request<ChannelList>('GET', `/api/v1/channels/${channelId}/todos`);
}

export function addChannelTodo(channelId: string, todo: ChannelTodoPatch) {
    return request<ChannelTodo>('POST', `/api/v1/channels/${channelId}/todos`, todo);
}

export function updateChannelTodo(channelId: string, id: string, patch: ChannelTodoPatch) {
    return request<ChannelTodo>('PATCH', `/api/v1/channels/${channelId}/todos/${id}`, patch);
}

export function deleteChannelTodo(channelId: string, id: string) {
    return request('DELETE', `/api/v1/channels/${channelId}/todos/${id}`);
}

export function getAssigned() {
    return request<AssignedTodo[]>('GET', '/api/v1/assigned');
}
