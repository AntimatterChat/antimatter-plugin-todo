// Copyright (c) 2026-present Antimatter contributors.
// See LICENSE for license information.

import {act, fireEvent, screen, waitFor} from '@testing-library/react';
import React from 'react';

import type {Channel} from '@mattermost/types/channels';

import {mockFetch, renderWithHost} from '../../tests/utils';
import {getState, resetState, restorePanel} from '../state';
import type {ListInfo} from '../types/todo';

import TodoHeader from './todo_header';
import TodoPanel from './todo_panel';

jest.mock('../manifest', () => ({__esModule: true, default: {id: 'todo'}}));

// A panel opened in a new window (Fusion's "Open in a new window") starts where it was.

const channel = {id: 'chan1', team_id: 'team1', type: 'O', display_name: 'Firmware', name: 'firmware', delete_at: 0} as Channel;
const listId = 'abcdefghijklmnopqrstuvwxyz';
const launch: ListInfo = {
    id: listId,
    name: 'Launch',
    owner_id: 'me',
    create_at: 1,
    update_at: 1,
    scope: 'channel',
    channel_id: 'chan1',
    channel_name: 'Firmware',
    favorite: false,
    can_edit: true,
    can_rename: true,
    can_delete: true,
    can_manage: false,
    open_count: 1,
};

const base = [
    {path: '/lists', body: {my: [], in: [], out: []}},
    {path: '/api/v1/lists?scope=', body: []},
    {path: '/api/v1/assigned', body: []},
];

function renderPanel(props: {setPanelState?: (state: string | null) => void; initialPanelState?: string} = {}) {
    return renderWithHost(
        <>
            <TodoHeader/>
            <TodoPanel {...props}/>
        </>,
        {channel},
    );
}

beforeEach(() => {
    act(() => resetState());
});

test('the panel opened in a new window starts at the open list, without showing the lists first', async () => {
    const calls = mockFetch([...base, {path: '/api/v1/lists?scope=channel', body: [launch]}, {path: `/api/v1/lists/${listId}`, body: {...launch, items: []}}]);
    const setPanelState = jest.fn();
    renderPanel({setPanelState, initialPanelState: `list/${listId}`});

    expect(await screen.findByDisplayValue('Launch')).toBeInTheDocument();
    expect(screen.getByRole('button', {name: 'All lists'})).toBeInTheDocument();
    expect(calls.some((c) => c.url.includes('/api/v1/lists?scope=channel'))).toBe(false);
    expect(setPanelState).toHaveBeenLastCalledWith(`list/${listId}`);
});

test('a list one can no longer open shows it, with the way back to the lists', async () => {
    mockFetch([...base, {path: '/api/v1/lists?scope=channel', body: [launch]}, {path: `/api/v1/lists/${listId}`, body: {error: 'list not found'}, status: 404}]);
    const setPanelState = jest.fn();
    renderPanel({setPanelState, initialPanelState: `list/${listId}`});

    expect(await screen.findByText('This list was deleted, or you can\'t open it.')).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', {name: 'All lists'}));
    expect(await screen.findByText('Launch')).toBeInTheDocument();
    expect(setPanelState).toHaveBeenLastCalledWith(null);
});

test('a tab is restored, and the main window reports where the panel goes', async () => {
    mockFetch([
        ...base,
        {path: '/api/v1/lists?scope=channel', body: [launch]},
        {path: '/api/v1/lists?scope=mine', body: [{...launch, id: 'mmmmmmmmmmmmmmmmmmmmmmmmmm', name: 'My todos', scope: 'personal', channel_id: ''}]},
        {path: `/api/v1/lists/${listId}`, body: {...launch, items: []}},
    ]);
    const setPanelState = jest.fn();
    renderPanel({setPanelState, initialPanelState: 'view/mine'});

    expect(await screen.findByText('My todos')).toBeInTheDocument();
    expect(screen.getByRole('tab', {name: /^Mine/})).toHaveAttribute('aria-selected', 'true');
    expect(setPanelState).toHaveBeenLastCalledWith('view/mine');

    fireEvent.click(screen.getByRole('tab', {name: 'This channel'}));
    await waitFor(() => expect(setPanelState).toHaveBeenLastCalledWith(null));
    fireEvent.click(await screen.findByText('Launch'));
    await waitFor(() => expect(setPanelState).toHaveBeenLastCalledWith(`list/${listId}`));
    fireEvent.click(await screen.findByRole('button', {name: 'All lists'}));
    await waitFor(() => expect(setPanelState).toHaveBeenLastCalledWith(null));
});

test('a state this app didn\'t write opens the lists', () => {
    for (const state of ['', 'note/abc', 'list/', `list/${listId}/x`, 'list/../../admin', 'list/ABCDEFGHIJKLMNOPQRSTUVWXYZ', 'view/channel', 'view/everything', 'list/<script>']) {
        act(() => resetState());
        expect(restorePanel(state)).toBe(false);
        expect(getState().openListId).toBeNull();
        expect(getState().view).toBe('channel');
    }
});

test('without the props, the panel works as before', async () => {
    mockFetch([...base, {path: '/api/v1/lists?scope=channel', body: [launch]}, {path: `/api/v1/lists/${listId}`, body: {...launch, items: []}}]);
    renderPanel();

    fireEvent.click(await screen.findByText('Launch'));
    expect(await screen.findByDisplayValue('Launch')).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', {name: 'All lists'}));
    expect(await screen.findByRole('tab', {name: 'This channel'})).toBeInTheDocument();
});
