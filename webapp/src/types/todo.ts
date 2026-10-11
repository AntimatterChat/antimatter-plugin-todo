// Copyright (c) 2026-present Antimatter contributors.
// See LICENSE for license information.

// A todo of someone's own lists (server/issue.go): "my" todos, todos others sent them ("in"), and
// todos they sent others ("out"). user and user_id are the other person's username and ID, for
// "in" and "out" todos.
export type Issue = {
    id: string;
    message: string;
    description?: string;
    postPermalink?: string;
    post_id?: string;
    create_at: number;
    user?: string;
    user_id?: string;
    list?: string;
    position?: number;
};

export type Lists = {
    my: Issue[];
    in: Issue[];
    out: Issue[];
};

// A todo of a channel's shared list (server/channel.go).
export type ChannelTodo = {
    id: string;
    channel_id: string;
    message: string;
    description?: string;
    creator_id: string;
    create_at: number;
    update_at: number;
    assignee_id?: string;
    done_by?: string;
    done_at?: number;
    post_id?: string;
    post_permalink?: string;
};

export type ChannelList = {
    can_edit: boolean;
    can_manage: boolean;
    todos: ChannelTodo[];
};

// A channel todo assigned to the user, with its channel.
export type AssignedTodo = ChannelTodo & {
    channel_name: string;
    channel_url_name: string;
    team_name?: string;
};

// The changes of a channel todo the server accepts.
export type ChannelTodoPatch = {
    message?: string;
    description?: string;
    assignee_id?: string;
    done?: boolean;
    position?: number;
    post_id?: string;
};
