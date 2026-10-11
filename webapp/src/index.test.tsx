// Copyright (c) 2026-present Antimatter contributors.
// See LICENSE for license information.

import {act, fireEvent, screen, waitFor, within} from '@testing-library/react';
import React from 'react';

import type {Channel} from '@mattermost/types/channels';
import type {Post} from '@mattermost/types/posts';

import {mockFetch, renderWithHost} from '../tests/utils';

import ChannelView from './components/channel_view';
import MineView from './components/mine_view';
import TodoPost from './components/todo_post';
import {getState, openDraft, resetState} from './state';

jest.mock('./manifest', () => ({__esModule: true, default: {id: 'todo'}}));

const channel = {id: 'chan1', team_id: 'team1', type: 'O', display_name: 'Firmware', name: 'firmware'} as Channel;
const listURL = '/api/v1/channels/chan1/todos';

type Call = {method: string; url: string; body?: unknown};

// sent is the body of the first request matching a method and URL end.
const sent = (calls: Call[], method: string, end = '') => calls.find((c) => c.method === method && c.url.endsWith(end))?.body;

const todo = (id: string, message: string, extra = {}) => ({id, channel_id: 'chan1', message, creator_id: 'me', create_at: 1, update_at: 1, ...extra});

beforeEach(() => {
    act(() => resetState());
});

test('channel view: lists the open todos, then the done ones folded', async () => {
    mockFetch([{path: listURL, body: {can_edit: true, can_manage: false, todos: [todo('t1', 'Order the bulbs', {assignee_id: 'bob'}), todo('t2', 'Ship rc3', {done_at: 5, done_by: 'bob'})]}}]);
    renderWithHost(<ChannelView channel={channel}/>);

    expect(await screen.findByText('Order the bulbs')).toBeInTheDocument();
    expect(screen.getByText('Assigned to Bob Ross')).toBeInTheDocument();
    expect(screen.queryByText('Ship rc3')).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', {name: /Done \(1\)/}));
    expect(screen.getByText('Ship rc3')).toBeInTheDocument();
    expect(screen.getByText('Done by Bob Ross')).toBeInTheDocument();
});

test('channel view: adds a todo and checks one off', async () => {
    const calls = mockFetch([
        {path: listURL, body: {can_edit: true, can_manage: false, todos: [todo('t1', 'Order the bulbs')]}},
        {method: 'POST', path: listURL, body: todo('t3', 'Book the room'), status: 201},
        {method: 'PATCH', path: listURL + '/t1', body: todo('t1', 'Order the bulbs', {done_at: 9})},
    ]);
    renderWithHost(<ChannelView channel={channel}/>);

    const input = await screen.findByPlaceholderText('Add a todo for Firmware…');
    fireEvent.change(input, {target: {value: 'Book the room'}});
    fireEvent.click(screen.getByRole('button', {name: 'Add'}));
    await waitFor(() => expect(sent(calls, 'POST')).toEqual({message: 'Book the room', description: ''}));

    fireEvent.click(await screen.findByRole('checkbox', {name: 'Mark as done'}));
    await waitFor(() => expect(sent(calls, 'PATCH')).toEqual({done: true}));
});

test('channel view: read-only channels show their todos without a form or checkboxes', async () => {
    mockFetch([{path: listURL, body: {can_edit: false, can_manage: false, todos: [todo('t1', 'Order the bulbs')]}}]);
    renderWithHost(<ChannelView channel={channel}/>);

    expect(await screen.findByText('Order the bulbs')).toBeInTheDocument();
    expect(screen.getByText('You can see the todos of this channel, not change them.')).toBeInTheDocument();
    expect(screen.queryByRole('textbox')).not.toBeInTheDocument();
    expect(screen.queryByRole('checkbox')).not.toBeInTheDocument();
    expect(screen.queryByRole('button', {name: 'More actions'})).not.toBeInTheDocument();
});

test('channel view: a todo started from a message is added linked to it', async () => {
    const calls = mockFetch([
        {path: listURL, body: {can_edit: true, can_manage: false, todos: []}},
        {method: 'POST', path: listURL, body: todo('t3', 'Fix the OTA bug'), status: 201},
    ]);
    act(() => openDraft({message: 'Fix the OTA bug', channelId: 'chan1', postId: 'post1', postPermalink: 'http://h/lumen/pl/post1'}));
    renderWithHost(<ChannelView channel={channel}/>);

    expect(await screen.findByDisplayValue('Fix the OTA bug')).toBeInTheDocument();
    expect(screen.getByText('From a message')).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', {name: 'Add'}));
    await waitFor(() => expect(sent(calls, 'POST')).toEqual({message: 'Fix the OTA bug', description: '', post_id: 'post1'}));
    await waitFor(() => expect(getState().draft).toBeNull());
});

test('mine view: my todos, the ones assigned in channels, and the requests to answer', async () => {
    const calls = mockFetch([
        {path: '/lists', body: {my: [{id: 'i1', message: 'Review the deck', create_at: 1}], in: [{id: 'i2', message: 'Sign the PO', create_at: 1, user: 'bob', user_id: 'bob'}], out: []}},
        {path: '/api/v1/assigned', body: [{...todo('t1', 'Order the bulbs'), channel_name: 'Firmware', channel_url_name: 'firmware', team_name: 'lumen'}]},
        {method: 'POST', path: '/accept', body: null},
    ]);
    renderWithHost(<MineView/>);

    expect(await screen.findByText('Review the deck')).toBeInTheDocument();
    expect(screen.getByText('Order the bulbs')).toBeInTheDocument();
    expect(screen.getByText('In Firmware')).toBeInTheDocument();
    expect(screen.getByText('From Bob Ross')).toBeInTheDocument();

    fireEvent.click(screen.getByRole('button', {name: 'Accept'}));
    await waitFor(() => expect(sent(calls, 'POST', '/accept')).toEqual({id: 'i2'}));
});

test('todo post: offers to answer a request until it is answered', async () => {
    mockFetch([{path: '/lists', body: {my: [], in: [{id: 'i2', message: 'Sign the PO', create_at: 1}], out: []}}, {path: '/api/v1/assigned', body: []}]);
    const post = {id: 'p', props: {message: 'You have received a new Todo from @bob', todo: 'Sign the PO', issueId: 'i2'}} as unknown as Post;
    const {container} = renderWithHost(<TodoPost post={post}/>);

    expect(await within(container).findByRole('button', {name: 'Decline'})).toBeInTheDocument();
    expect(screen.getByText('Sign the PO')).toBeInTheDocument();

    mockFetch([{path: '/lists', body: {my: [], in: [], out: []}}, {path: '/api/v1/assigned', body: []}, {method: 'POST', path: '/accept', body: null}]);
    fireEvent.click(screen.getByRole('button', {name: 'Accept'}));
    expect(await screen.findByText('Answered')).toBeInTheDocument();
});
