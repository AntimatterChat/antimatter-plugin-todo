// Copyright (c) 2026-present Antimatter contributors.
// See LICENSE for license information.

import {act, fireEvent, screen, waitFor, within} from '@testing-library/react';
import React from 'react';

import type {Channel} from '@mattermost/types/channels';

import {mockFetch, renderWithHost} from '../../tests/utils';
import {resetState, setView} from '../state';
import type {ListInfo} from '../types/todo';

import TodoPanel from './todo_panel';

jest.mock('../manifest', () => ({__esModule: true, default: {id: 'todo'}}));

// The list of lists, laid out as Notes' and Whiteboard's: the tabs, then the tab's header row (what the tab is on the
// left, the + making a list there on the right, its form opening under the row), then the tab's lists or its empty
// state.

const firmware = {id: 'chan1', team_id: 'team1', type: 'O', display_name: 'Firmware', name: 'firmware', delete_at: 0} as Channel;
const dm = {id: 'dm1', team_id: '', type: 'D', display_name: 'bob', name: 'me__bob', delete_at: 0} as Channel;

function info(id: string, name: string, extra: Partial<ListInfo> = {}): ListInfo {
    return {
        id,
        name,
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
        open_count: 0,
        ...extra,
    };
}

const base = [
    {path: '/lists', body: {my: [], in: [], out: []}},
    {path: '/api/v1/lists?scope=', body: []},
    {path: '/api/v1/assigned', body: []},
];

const setWebUI = (ui?: 'classic' | 'fusion') => {
    window.antimatterWebUI = ui;
    if (ui) {
        document.documentElement.dataset.amWebUi = ui;
    } else {
        delete document.documentElement.dataset.amWebUi;
    }
};

// renderApp renders the panel as the host does, under the host's header (which only names the app).
function renderApp(channel: Channel = firmware) {
    return renderWithHost(
        <div data-testid='panel'>
            <TodoPanel/>
        </div>,
        {channel},
    );
}

// An open list's toolbar, at the top of the panel; the lists have none.
const header = () => within(screen.getByTestId('todo-toolbar'));
const panel = () => within(screen.getByTestId('panel'));
const head = () => screen.getByTestId('todo-section-head');
const headButtons = () => within(head()).queryAllByRole('button');
const postsList = (c: {method: string; url: string}) => c.method === 'POST' && c.url.endsWith('/api/v1/lists');

beforeEach(() => {
    act(() => resetState());
});

afterEach(() => setWebUI());

// inBoth runs a test under each UI: the layout is the same in Fusion and the classic UI.
function inBoth(name: string, run: (ui: 'fusion' | 'classic') => Promise<void>) {
    for (const ui of ['fusion', 'classic'] as const) {
        test(`${name} (${ui})`, async () => {
            setWebUI(ui);
            await run(ui);
        });
    }
}

inBoth('the header row sits right under the tabs: "Shared with" the channel, and its + on the right, with no toolbar above', async () => {
    mockFetch([...base, {path: '/api/v1/lists?scope=channel', body: [info('l1', 'Launch', {open_count: 3})]}]);
    renderApp();

    expect(await panel().findByText('Launch')).toBeInTheDocument();
    expect(head().previousElementSibling).toBe(panel().getByRole('tablist'));
    expect(head()).toHaveTextContent('Shared with Firmware');
    expect(headButtons()).toHaveLength(1);
    const add = within(head()).getByRole('button', {name: 'New list in this channel'});
    expect(add.previousElementSibling).toHaveTextContent('Shared with Firmware');
    expect(screen.queryByTestId('todo-toolbar')).toBeNull();
});

inBoth('the + opens the new list\'s form under the header row, and closes it', async () => {
    const calls = mockFetch([
        ...base,
        {path: '/api/v1/lists?scope=channel', body: [info('l1', 'Launch')]},
        {method: 'POST', path: '/api/v1/lists', body: {...info('l2', 'Retro'), items: []}, status: 201},
        {path: '/api/v1/lists/l2', body: {...info('l2', 'Retro'), items: []}},
    ]);
    renderApp();
    await panel().findByText('Launch');

    const add = within(head()).getByRole('button', {name: 'New list in this channel'});
    expect(add).toHaveAttribute('aria-pressed', 'false');
    fireEvent.click(add);
    expect(add).toHaveAttribute('aria-pressed', 'true');
    const name = panel().getByPlaceholderText('Name of the new list');
    expect(head().nextElementSibling).toBe(name.closest('form'));
    expect(name.closest('form')?.nextElementSibling).toHaveTextContent('Launch');

    fireEvent.click(add);
    expect(panel().queryByPlaceholderText('Name of the new list')).toBeNull();

    fireEvent.click(add);
    fireEvent.change(panel().getByPlaceholderText('Name of the new list'), {target: {value: 'Retro'}});
    fireEvent.click(panel().getByRole('button', {name: 'Create'}));
    await waitFor(() => expect(calls.some(postsList)).toBe(true));
    expect(await header().findByDisplayValue('Retro')).toBeInTheDocument();
});

inBoth('in a direct message, the lists are shared with "this conversation"', async () => {
    mockFetch([...base, {path: '/api/v1/lists?scope=channel', body: []}]);
    renderApp(dm);
    expect(await panel().findByText('Every member of the conversation can edit the lists added here.')).toBeInTheDocument();
    expect(head()).toHaveTextContent('Shared with this conversation');
    expect(within(head()).getByRole('button', {name: 'New list in this conversation'})).toBeInTheDocument();
});

inBoth('the favourites have a header row without a +', async () => {
    mockFetch([...base, {path: '/api/v1/lists?scope=favorites', body: []}]);
    act(() => setView('favorites'));
    renderApp();
    expect(await panel().findByText('Nothing starred yet')).toBeInTheDocument();
    expect(head()).toHaveTextContent('Favourites');
    expect(headButtons()).toHaveLength(0);
    expect(screen.queryByRole('button', {name: /New/})).toBeNull();
});

inBoth('an archived channel has its header row, without a +', async () => {
    mockFetch([...base, {path: '/api/v1/lists?scope=channel', body: []}]);
    renderApp({...firmware, delete_at: 5});
    expect(await panel().findByText('No lists here yet')).toBeInTheDocument();
    expect(head()).toHaveTextContent('Shared with Firmware');
    expect(headButtons()).toHaveLength(0);
    expect(screen.queryByRole('button', {name: /New/})).toBeNull();
});

inBoth('mine: "Personal lists" and its +, then the lists, the note and the todos sent', async () => {
    mockFetch([...base, {path: '/api/v1/lists?scope=mine', body: [info('m1', 'My todos', {scope: 'personal', channel_id: '', channel_name: '', default: true, open_count: 2})]}]);
    act(() => setView('mine'));
    renderApp();
    const row = (await panel().findByText('My todos')).closest('button') as HTMLElement;
    expect(head()).toHaveTextContent('Personal lists');
    expect(head().nextElementSibling).toBe(row);
    fireEvent.click(within(head()).getByRole('button', {name: 'New personal list'}));
    expect(head().nextElementSibling).toBe(panel().getByPlaceholderText('Name of the new list').closest('form'));
    expect(panel().getByText('Only you can see your personal lists.')).toBeInTheDocument();
});

inBoth('a list\'s row: Fusion\'s app row, its tile, its name in bold over what\'s left, and a lock if one can\'t change it', async (ui) => {
    mockFetch([...base, {path: '/api/v1/lists?scope=channel', body: [info('l1', 'Launch', {open_count: 3, can_edit: false})]}]);
    renderApp();
    const name = await panel().findByText('Launch');
    expect(name.tagName).toBe('B');
    const row = name.closest('button') as HTMLElement;
    expect(row).toHaveClass(ui === 'fusion' ? 'am-app-row' : 'amt-app-row');
    expect(row.firstElementChild).toHaveClass('amt-tile');
    expect(within(row).getByText('3 open')).toHaveClass(ui === 'fusion' ? 'am-sub' : 'amt-sub');
    expect(row.lastElementChild?.className).toBe(ui === 'fusion' ? 'am-sub' : 'amt-sub');
});

inBoth('the favourites\' rows say where each list is', async () => {
    mockFetch([...base, {path: '/api/v1/lists?scope=favorites', body: [info('l1', 'Launch')]}]);
    act(() => setView('favorites'));
    renderApp();
    const row = (await panel().findByText('Launch')).closest('button') as HTMLElement;
    expect(row.lastElementChild).toHaveClass('amt-scope', 'amt-scope-channel');
    expect(row.lastElementChild).toHaveTextContent('Firmware');
});

test('an empty tab under Fusion: an icon, a title, a sentence and the + as their action, which opens the form under the header row', async () => {
    setWebUI('fusion');
    mockFetch([...base, {path: '/api/v1/lists?scope=channel', body: []}, {path: '/api/v1/lists?scope=favorites', body: []}]);
    renderApp();
    const empty = (await panel().findByText('No lists here yet')).closest('[data-empty-state]') as HTMLElement;
    expect(empty).toHaveClass('am-empty', 'amfx-empty');
    expect(empty.querySelector('.amfx-empty-ic use')).toHaveAttribute('href', '#am-i-checkbox');
    expect(within(empty).getByText('Every member of the channel can edit the lists added here.')).toBeInTheDocument();
    const action = within(empty).getByRole('button', {name: 'New list in this channel'});
    expect(action).toHaveClass('am-btn', 'am-primary');
    fireEvent.click(action);
    expect(head().nextElementSibling).toBe(panel().getByPlaceholderText('Name of the new list').closest('form'));
    expect(within(panel().getByTestId('todo-empty')).queryByRole('button')).toBeNull();

    fireEvent.click(panel().getByRole('tab', {name: 'Favourites'}));
    const starred = (await panel().findByText('Nothing starred yet')).closest('[data-empty-state]') as HTMLElement;
    expect(within(starred).getByText('Star a list to find it here, wherever it is.')).toBeInTheDocument();
    expect(within(starred).queryByRole('button')).toBeNull();
    expect(panel().queryByPlaceholderText('Name of the new list')).toBeNull();
});

test('an empty tab in an archived channel offers no action', async () => {
    setWebUI('fusion');
    mockFetch([...base, {path: '/api/v1/lists?scope=channel', body: []}]);
    renderApp({...firmware, delete_at: 5});
    const empty = (await panel().findByText('No lists here yet')).closest('[data-empty-state]') as HTMLElement;
    expect(within(empty).queryByRole('button')).toBeNull();
});

test('while a tab loads, a quiet line says what loads, not an ellipsis', async () => {
    setWebUI('fusion');
    global.fetch = jest.fn(() => new Promise(() => null)) as unknown as typeof fetch;
    renderApp();
    const busy = await panel().findByRole('status');
    expect(busy).toHaveTextContent('Loading lists…');
    expect(busy).toHaveClass('am-empty', 'amfx-empty', 'amfx-busy');
    expect(panel().queryByText('…')).toBeNull();
});

test('an empty tab under the classic UI: its own empty state, a title and a sentence, the + only in the header row', async () => {
    setWebUI('classic');
    mockFetch([...base, {path: '/api/v1/lists?scope=channel', body: []}]);
    renderApp();
    const empty = (await panel().findByText('No lists here yet')).closest('[data-empty-state]') as HTMLElement;
    expect(empty.className).toBe('amt-todo-empty');
    expect(within(empty).getByText('Every member of the channel can edit the lists added here.')).toBeInTheDocument();
    expect(within(empty).queryByRole('button')).toBeNull();
    expect(screen.getAllByRole('button', {name: 'New list in this channel'})).toHaveLength(1);
});
