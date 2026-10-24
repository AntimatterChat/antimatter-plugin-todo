// Copyright (c) 2026-present Antimatter contributors.
// See LICENSE for license information.

import {mockFetch} from '../tests/utils';

import {channelKey, refreshIndex, refreshList, refreshLoaded, resetState} from './state';

beforeEach(() => {
    resetState();
});

// Regression test: refreshLoaded (run on reconnect, see index.tsx) used to only reload the
// requests and the "mine" tab, so a channel's tab or an open list kept the state it had before the
// websocket dropped, missing the changes made meanwhile. It must now refresh every loaded index
// (any tab) and every open list's detail.
test('refreshLoaded refreshes every loaded index and open list', async () => {
    const calls = mockFetch([
        {path: '/api/v1/lists?scope=mine', body: []},
        {path: '/api/v1/lists?scope=channel&channel_id=chan1', body: []},
        {path: '/api/v1/lists/l1', body: {id: 'l1', name: 'Launch', items: []}},
    ]);

    await refreshIndex('mine');
    await refreshIndex(channelKey('chan1'));
    await refreshList('l1');
    calls.length = 0;

    await refreshLoaded();

    const paths = calls.map((c) => c.url);
    expect(paths.some((p) => p.includes('scope=mine'))).toBe(true);
    expect(paths.some((p) => p.includes('scope=channel&channel_id=chan1'))).toBe(true);
    expect(paths.some((p) => p.endsWith('/api/v1/lists/l1'))).toBe(true);
});
