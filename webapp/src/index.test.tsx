// Copyright (c) 2026-present Antimatter contributors.
// See LICENSE for license information.

import {act, fireEvent, screen, waitFor, within} from '@testing-library/react';
import React from 'react';

import type {Channel} from '@mattermost/types/channels';
import type {Post} from '@mattermost/types/posts';

import {mockFetch, renderWithHost} from '../tests/utils';

import {dayOf, urgency} from './components/due';
import TodoHeader from './components/todo_header';
import TodoPanel from './components/todo_panel';
import TodoPost from './components/todo_post';
import {getState, openDraft, openList, resetState} from './state';
import type {Item, ListInfo} from './types/todo';

jest.mock('./manifest', () => ({__esModule: true, default: {id: 'todo'}}));

const channel = {id: 'chan1', team_id: 'team1', type: 'O', display_name: 'Firmware', name: 'firmware', delete_at: 0} as Channel;

type Call = {method: string; url: string; body?: unknown};

// sent is the body of the first request matching a method and URL end.
const sent = (calls: Call[], method: string, end = '') => calls.find((c) => c.method === method && c.url.endsWith(end))?.body;
const bodies = (calls: Call[], method: string, end: string) => calls.filter((c) => c.method === method && c.url.endsWith(end)).map((c) => c.body);

const yesterday = dayOf(new Date(Date.now() - (24 * 60 * 60 * 1000)));

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

const personal = (id: string, name: string, extra: Partial<ListInfo> = {}) => info(id, name, {scope: 'personal', channel_id: undefined, channel_name: undefined, ...extra});
const item = (id: string, message: string, extra: Partial<Item> = {}): Item => ({id, list_id: 'l1', message, creator_id: 'me', create_at: 1, update_at: 1, ...extra});
const launch = info('l1', 'Launch', {open_count: 1});

// What every panel loads: the todos sent between people, the assignments, and (unless a test says
// otherwise) no lists.
const base = (requests = {my: [], in: [], out: []}) => [
    {path: '/lists', body: requests},
    {path: '/api/v1/lists?scope=', body: []},
    {path: '/api/v1/assigned', body: []},
];

beforeEach(() => {
    act(() => resetState());
});

test('due dates: overdue, today, tomorrow, soon and later', () => {
    expect(urgency('2026-10-16', '2026-10-17')).toBe('overdue');
    expect(urgency('2026-10-17', '2026-10-17')).toBe('today');
    expect(urgency('2026-10-18', '2026-10-17')).toBe('tomorrow');
    expect(urgency('2026-10-20', '2026-10-17')).toBe('soon');
    expect(urgency('2026-11-01', '2026-10-17')).toBe('later');
    expect(urgency('2027-01-01', '2026-12-31')).toBe('tomorrow');
});

test('list of lists: the channel\'s lists with what is left to do and what is overdue, the archived ones folded', async () => {
    mockFetch([...base(), {path: '/api/v1/lists?scope=channel', body: [info('l1', 'Launch', {open_count: 3, earliest_due: yesterday}), info('l2', 'Retro', {archived_at: 5})]}]);
    renderWithHost(
        <>
            <TodoHeader/>
            <TodoPanel/>
        </>,
        {channel},
    );

    expect(await screen.findByText('Launch')).toBeInTheDocument();
    expect(screen.getByText('Todo')).toBeInTheDocument();
    expect(screen.getByText('3 open')).toBeInTheDocument();
    expect(screen.getByText('Overdue')).toBeInTheDocument();
    expect(screen.queryByText('Retro')).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', {name: /Archived \(1\)/}));
    expect(screen.getByText('Retro')).toBeInTheDocument();
});

test('tabs: this channel, the favorites (saying where each list is) and mine (with the todos sent to me)', async () => {
    mockFetch([
        ...base({my: [], in: [{id: 'i2', message: 'Sign the PO', create_at: 1, user: 'bob', user_id: 'bob'}], out: []} as never),
        {path: '/api/v1/lists?scope=channel', body: []},
        {path: '/api/v1/lists?scope=favorites', body: [personal('l3', 'Groceries'), launch]},
        {path: '/api/v1/lists?scope=mine', body: [personal('m1', 'My todos', {default: true, open_count: 2})]},
    ]);
    renderWithHost(<TodoPanel/>, {channel});

    expect(await screen.findByText('No lists here yet. Lists made here are shared with everyone in the channel.')).toBeInTheDocument();
    expect(screen.getByRole('tab', {name: 'This channel'})).toHaveAttribute('aria-selected', 'true');

    fireEvent.click(screen.getByRole('tab', {name: 'Favourites'}));
    expect(await screen.findByText('Groceries')).toBeInTheDocument();
    expect(screen.getByText('Personal')).toBeInTheDocument();
    expect(screen.getByTitle('Shared with everyone in ~Firmware')).toBeInTheDocument();

    fireEvent.click(screen.getByRole('tab', {name: /^Mine/}));
    expect(await screen.findByText('My todos')).toBeInTheDocument();
    expect(screen.getByText('2 open')).toBeInTheDocument();
    expect(screen.getByText('Only you can see your personal lists.')).toBeInTheDocument();
    expect(await screen.findByText('Sign the PO')).toBeInTheDocument();
    expect(screen.getByText('Sent to you')).toBeInTheDocument();
});

test('open list: back, its name, where it is, the star and its menu; a todo checked off', async () => {
    const calls = mockFetch([
        ...base(),
        {path: '/api/v1/lists?scope=channel', body: [launch]},
        {path: '/api/v1/lists/l1', body: {...launch, items: [item('t1', 'Order the bulbs', {assignee_id: 'bob', due: yesterday}), item('t2', 'Ship rc3', {done_at: 5, done_by: 'bob'})]}},
        {method: 'PATCH', path: '/api/v1/lists/l1/items/t1', body: item('t1', 'Order the bulbs', {done_at: 9})},
    ]);
    renderWithHost(
        <>
            <TodoHeader/>
            <TodoPanel/>
        </>,
        {channel},
    );

    fireEvent.click(await screen.findByText('Launch'));
    expect(await screen.findByText('Order the bulbs')).toBeInTheDocument();
    expect(screen.getByDisplayValue('Launch')).toBeInTheDocument();
    expect(screen.queryByText('Todo')).not.toBeInTheDocument();
    expect(screen.getByRole('img', {name: 'Shared with everyone in ~Firmware'})).toBeInTheDocument();
    expect(screen.getByRole('button', {name: 'Add to favourites'})).toBeInTheDocument();
    expect(screen.getByRole('button', {name: 'List actions'})).toBeInTheDocument();
    expect(screen.getByText('Assigned to Bob Ross')).toBeInTheDocument();
    expect(screen.getByText('Overdue')).toBeInTheDocument();
    expect(screen.queryByText('Ship rc3')).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', {name: /Done \(1\)/}));
    expect(screen.getByText('Done by Bob Ross')).toBeInTheDocument();

    fireEvent.click(screen.getByRole('checkbox', {name: 'Mark as done'}));
    await waitFor(() => expect(sent(calls, 'PATCH', '/items/t1')).toEqual({done: true}));

    fireEvent.click(screen.getByRole('button', {name: 'All lists'}));
    expect(await screen.findByRole('tab', {name: 'This channel'})).toBeInTheDocument();
});

test('open list: adds a todo with a due date', async () => {
    const calls = mockFetch([
        ...base(),
        {path: '/api/v1/lists/l1', body: {...launch, items: []}},
        {method: 'POST', path: '/api/v1/lists/l1/items', body: item('t3', 'Book the room'), status: 201},
    ]);
    act(() => openList('l1'));
    renderWithHost(<TodoPanel/>, {channel});

    const input = await screen.findByPlaceholderText('Add a todo to Launch…');
    fireEvent.change(input, {target: {value: 'Book the room'}});
    fireEvent.click(screen.getByRole('button', {name: 'Due date'}));
    fireEvent.change(screen.getByLabelText('Due date'), {target: {value: '2026-12-01'}});
    fireEvent.click(screen.getByRole('button', {name: 'Add'}));
    await waitFor(() => expect(sent(calls, 'POST', '/items')).toEqual({message: 'Book the room', description: '', due: '2026-12-01', channel_id: 'chan1'}));
});

test('a read-only list shows its todos without a form, checkboxes or actions', async () => {
    mockFetch([...base(), {path: '/api/v1/lists/l1', body: {...launch, can_edit: false, can_rename: false, can_delete: false, items: [item('t1', 'Order the bulbs')]}}]);
    act(() => openList('l1'));
    renderWithHost(<TodoPanel/>, {channel});

    expect(await screen.findByText('Order the bulbs')).toBeInTheDocument();
    expect(screen.getByText('You can see the todos of this channel, not change them.')).toBeInTheDocument();
    expect(screen.queryByPlaceholderText('Add a todo to Launch…')).not.toBeInTheDocument();
    expect(screen.queryByRole('checkbox')).not.toBeInTheDocument();
    expect(screen.queryByRole('button', {name: 'More actions'})).not.toBeInTheDocument();
    expect(screen.queryByRole('button', {name: 'List actions'})).not.toBeInTheDocument();
    expect(screen.getByDisplayValue('Launch')).toHaveAttribute('readonly');
});

test('makes a list from the + of the header, in the scope of the tab', async () => {
    const calls = mockFetch([
        ...base(),
        {path: '/api/v1/lists?scope=channel', body: []},
        {path: '/api/v1/lists?scope=mine', body: [personal('m1', 'My todos', {default: true})]},
        {method: 'POST', path: '/api/v1/lists', body: {...launch, items: []}, status: 201},
        {path: '/api/v1/lists/l1', body: {...launch, items: []}},
    ]);
    renderWithHost(
        <>
            <TodoHeader/>
            <TodoPanel/>
        </>,
        {channel},
    );

    fireEvent.click(await screen.findByRole('button', {name: 'New list in this channel'}));
    fireEvent.change(screen.getByPlaceholderText('Name of the new list'), {target: {value: ' Launch '}});
    fireEvent.click(screen.getByRole('button', {name: 'Create'}));
    await waitFor(() => expect(sent(calls, 'POST', '/api/v1/lists')).toEqual({name: 'Launch', channel_id: 'chan1', icon: ''}));
    expect(await screen.findByDisplayValue('Launch')).toBeInTheDocument();
    expect(getState().openListId).toBe('l1');

    fireEvent.click(screen.getByRole('button', {name: 'All lists'}));
    fireEvent.click(await screen.findByRole('tab', {name: /^Mine/}));
    fireEvent.click(await screen.findByRole('button', {name: 'New personal list'}));
    fireEvent.change(screen.getByPlaceholderText('Name of the new list'), {target: {value: 'Groceries'}});
    fireEvent.click(screen.getByRole('button', {name: 'Create'}));
    await waitFor(() => expect(bodies(calls, 'POST', '/api/v1/lists')).toContainEqual({name: 'Groceries', channel_id: '', icon: ''}));

    act(() => resetState());
});

test('the favorites have no + : a list is made in a channel or in mine', async () => {
    mockFetch([...base(), {path: '/api/v1/lists?scope=favorites', body: []}]);
    renderWithHost(
        <>
            <TodoHeader/>
            <TodoPanel/>
        </>,
        {channel},
    );
    fireEvent.click(await screen.findByRole('tab', {name: 'Favourites'}));
    expect(await screen.findByText('Star a list to find it here, wherever it is.')).toBeInTheDocument();
    expect(screen.queryByRole('button', {name: 'New list in this channel'})).not.toBeInTheDocument();
    expect(screen.queryByRole('button', {name: 'New personal list'})).not.toBeInTheDocument();
});

test('renames, changes the icon of and archives the open list', async () => {
    const calls = mockFetch([
        ...base(),
        {path: '/api/v1/lists/l1', body: {...launch, items: []}},
        {method: 'PATCH', path: '/api/v1/lists/l1', body: {...launch, items: []}},
    ]);
    act(() => openList('l1'));
    renderWithHost(<TodoPanel/>, {channel});

    const title = await screen.findByDisplayValue('Launch');
    fireEvent.focus(title);
    fireEvent.change(title, {target: {value: 'Launch day'}});
    fireEvent.blur(title);
    await waitFor(() => expect(bodies(calls, 'PATCH', '/api/v1/lists/l1')).toContainEqual({name: 'Launch day'}));

    fireEvent.click(screen.getByRole('button', {name: 'List actions'}));
    fireEvent.click(screen.getByRole('menuitem', {name: 'Change icon'}));
    fireEvent.click(await screen.findByRole('button', {name: '🚀'}));
    await waitFor(() => expect(bodies(calls, 'PATCH', '/api/v1/lists/l1')).toContainEqual({icon: '🚀'}));

    fireEvent.click(screen.getByRole('button', {name: 'List actions'}));
    fireEvent.click(screen.getByRole('menuitem', {name: 'Archive'}));
    await waitFor(() => expect(bodies(calls, 'PATCH', '/api/v1/lists/l1')).toContainEqual({archived: true}));
});

test('an archived list is read-only, and unarchived from its menu', async () => {
    const calls = mockFetch([
        ...base(),
        {path: '/api/v1/lists/l1', body: {...launch, archived_at: 5, can_edit: false, items: [item('t1', 'Order the bulbs')]}},
        {method: 'PATCH', path: '/api/v1/lists/l1', body: {...launch, items: []}},
    ]);
    act(() => openList('l1'));
    renderWithHost(<TodoPanel/>, {channel});

    expect(await screen.findByText('This list is archived: unarchive it to change its todos.')).toBeInTheDocument();
    expect(screen.queryByRole('checkbox')).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', {name: 'List actions'}));
    fireEvent.click(screen.getByRole('menuitem', {name: 'Unarchive'}));
    await waitFor(() => expect(bodies(calls, 'PATCH', '/api/v1/lists/l1')).toContainEqual({archived: false}));
});

test('the default list is neither archived nor deleted', async () => {
    mockFetch([...base(), {path: '/api/v1/lists/m1', body: {...personal('m1', 'My todos', {default: true, can_delete: false}), items: []}}]);
    act(() => openList('m1'));
    renderWithHost(<TodoPanel/>, {channel});

    await screen.findByDisplayValue('My todos');
    expect(screen.getByRole('img', {name: 'Personal: only you can see it'})).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', {name: 'List actions'}));
    expect(screen.getByRole('menuitem', {name: 'Rename'})).toBeInTheDocument();
    expect(screen.queryByRole('menuitem', {name: 'Archive'})).not.toBeInTheDocument();
    expect(screen.queryByRole('menuitem', {name: 'Delete list'})).not.toBeInTheDocument();
});

test('deleting a list asks first, in the page', async () => {
    const calls = mockFetch([
        ...base(),
        {path: '/api/v1/lists?scope=channel', body: []},
        {path: '/api/v1/lists/l1', body: {...launch, items: []}},
        {method: 'DELETE', path: '/api/v1/lists/l1', body: {status: 'OK'}},
    ]);
    act(() => openList('l1'));
    renderWithHost(<TodoPanel/>, {channel});

    await screen.findByDisplayValue('Launch');
    fireEvent.click(screen.getByRole('button', {name: 'List actions'}));
    fireEvent.click(screen.getByRole('menuitem', {name: 'Delete list'}));
    const dialog = screen.getByRole('alertdialog');
    expect(dialog).toHaveTextContent('Delete “Launch” and its todos for everyone? This can\'t be undone.');
    expect(calls.some((c) => c.method === 'DELETE')).toBe(false);
    fireEvent.click(within(dialog).getByRole('button', {name: 'Cancel'}));
    expect(screen.queryByRole('alertdialog')).not.toBeInTheDocument();

    fireEvent.click(screen.getByRole('button', {name: 'List actions'}));
    fireEvent.click(screen.getByRole('menuitem', {name: 'Delete list'}));
    fireEvent.click(within(screen.getByRole('alertdialog')).getByRole('button', {name: 'Delete'}));
    await waitFor(() => expect(calls.some((c) => c.method === 'DELETE' && c.url.endsWith('/api/v1/lists/l1'))).toBe(true));
    expect(await screen.findByRole('tab', {name: 'This channel'})).toBeInTheDocument();
    expect(getState().openListId).toBeNull();
});

test('"Add to todo" asks which list, offering the last one used from the channel', async () => {
    const calls = mockFetch([
        ...base(),
        {path: '/api/v1/targets?channel_id=chan1', body: {lists: [launch, info('l2', 'Retro'), personal('m1', 'My todos')], default_id: 'l2'}},
        {method: 'POST', path: '/api/v1/lists/m1/items', body: item('t9', 'Fix the OTA bug'), status: 201},
        {path: '/api/v1/lists/m1', body: {...personal('m1', 'My todos'), items: [item('t9', 'Fix the OTA bug')]}},
    ]);
    act(() => openDraft({message: 'Fix the OTA bug', channelId: 'chan1', postId: 'post1', postPermalink: 'http://h/lumen/pl/post1'}));
    renderWithHost(<TodoPanel/>, {channel});

    const select = await screen.findByRole('combobox');
    expect(select).toHaveValue('l2');
    expect(screen.getByDisplayValue('Fix the OTA bug')).toBeInTheDocument();
    expect(screen.getByText('From a message')).toBeInTheDocument();
    expect(within(select).getAllByRole('option').map((o) => o.textContent)).toEqual(['Launch · Firmware', 'Retro · Firmware', 'My todos']);

    fireEvent.change(select, {target: {value: 'm1'}});
    expect(await screen.findByPlaceholderText('Add a todo to My todos…')).toHaveValue('Fix the OTA bug');
    fireEvent.click(screen.getByRole('button', {name: 'Add'}));
    await waitFor(() => expect(sent(calls, 'POST', '/api/v1/lists/m1/items')).toEqual({message: 'Fix the OTA bug', description: '', post_id: 'post1', channel_id: 'chan1'}));
    await waitFor(() => expect(getState().draft).toBeNull());
    expect(getState().openListId).toBe('m1');
});

test('mine: accepts a todo sent to me', async () => {
    const calls = mockFetch([
        ...base({my: [], in: [{id: 'i2', message: 'Sign the PO', create_at: 1, user: 'bob', user_id: 'bob'}], out: []} as never),
        {path: '/api/v1/lists?scope=mine', body: [personal('m1', 'My todos', {default: true})]},
        {method: 'POST', path: '/accept', body: null},
    ]);
    renderWithHost(<TodoPanel/>);

    expect(await screen.findByText('Sign the PO')).toBeInTheDocument();
    expect(screen.getByText('From Bob Ross')).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', {name: 'Accept'}));
    await waitFor(() => expect(sent(calls, 'POST', '/accept')).toEqual({id: 'i2'}));
});

test('todo post: offers to answer a request until it is answered', async () => {
    mockFetch([{path: '/lists', body: {my: [], in: [{id: 'i2', message: 'Sign the PO', create_at: 1}], out: []}}, {path: '/api/v1/assigned', body: []}, {path: '/api/v1/lists?scope=', body: []}]);
    const post = {id: 'p', props: {message: 'You have received a new Todo from @bob', todo: 'Sign the PO', issueId: 'i2'}} as unknown as Post;
    const {container} = renderWithHost(<TodoPost post={post}/>);

    expect(await within(container).findByRole('button', {name: 'Decline'})).toBeInTheDocument();
    expect(screen.getByText('Sign the PO')).toBeInTheDocument();

    mockFetch([{path: '/lists', body: {my: [], in: [], out: []}}, {path: '/api/v1/assigned', body: []}, {path: '/api/v1/lists?scope=', body: []}, {method: 'POST', path: '/accept', body: null}]);
    fireEvent.click(screen.getByRole('button', {name: 'Accept'}));
    expect(await screen.findByText('Answered')).toBeInTheDocument();
});
