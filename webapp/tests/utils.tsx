// Copyright (c) 2026-present Antimatter contributors.
// See LICENSE for license information.

import {render} from '@testing-library/react';
import React from 'react';
import {IntlProvider} from 'react-intl';
import {Provider} from 'react-redux';
import {applyMiddleware, createStore} from 'redux';

import type {Channel} from '@mattermost/types/channels';

const profiles = {
    me: {id: 'me', username: 'maya', first_name: 'Maya', last_name: 'Chen'},
    bob: {id: 'bob', username: 'bob', first_name: 'Bob', last_name: 'Ross'},
};

function hostState(channel?: Channel) {
    return {
        entities: {
            general: {config: {TeammateNameDisplay: 'full_name'}, license: {}},
            users: {currentUserId: 'me', profiles, statuses: {}},
            teams: {currentTeamId: 'team1', teams: {team1: {id: 'team1', name: 'lumen'}}},
            channels: {currentChannelId: channel?.id || '', channels: channel ? {[channel.id]: channel} : {}},
            preferences: {myPreferences: {}},
        },
    };
}

// A store like the host's: its state (in a channel, when given), and thunks run (and ignored).
export function renderWithHost(ui: React.ReactElement, {channel}: {channel?: Channel} = {}) {
    const thunk = () => (next: (a: unknown) => unknown) => (action: unknown) => (typeof action === 'function' ? Promise.resolve({data: true}) : next(action));
    const state = hostState(channel);
    const store = createStore((s = state) => s, applyMiddleware(thunk));
    return render(
        <Provider store={store}>
            <IntlProvider locale='en'>{ui}</IntlProvider>
        </Provider>,
    );
}

type Route = {method?: string; path: string; body: unknown; status?: number};

// mockFetch answers the plugin's API from routes (the last matching one wins), and records the
// requests.
export function mockFetch(routes: Route[]) {
    const calls: Array<{method: string; url: string; body?: unknown}> = [];
    global.fetch = jest.fn(async (url: string, options?: {method?: string; body?: string}) => {
        const method = options?.method || 'GET';
        calls.push({method, url, body: options?.body ? JSON.parse(options.body) : null});
        const route = [...routes].reverse().find((r) => (r.method || 'GET') === method && url.includes(r.path));
        const status = route ? route.status || 200 : 404;
        const text = JSON.stringify(route ? route.body : {error: 'not found'});
        return {ok: status < 300, status, statusText: 'error', text: async () => text, json: async () => JSON.parse(text)};
    }) as unknown as typeof fetch;
    return calls;
}
