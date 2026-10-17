// Copyright (c) 2026-present Antimatter contributors.
// See LICENSE for license information.

// A todo sent between people (server/issue.go): the ones sent to the user ("in", waiting for their
// answer) and the ones they sent ("out"). user and user_id are the other person's username and ID.
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

// The todos sent between people, and ("my") the open todos of the user's own lists.
export type Lists = {
    my: Issue[];
    in: Issue[];
    out: Issue[];
};

// Whose a list is: the user's own, a channel's, or a conversation's (a direct or group message).
export type Scope = 'personal' | 'channel' | 'conversation';

// A list of todos (server/lists.go), as the user sees it: where it is, what they may do with it,
// and what's left to do.
export type ListInfo = {
    id: string;
    name: string;
    icon?: string;
    channel_id?: string;
    owner_id: string;
    create_at: number;
    update_at: number;
    archived_at?: number;

    scope: Scope;
    channel_name?: string;
    channel_url_name?: string;
    team_name?: string;

    // The user's default personal list, where the todos they accept land: it stays.
    default?: boolean;
    favorite: boolean;

    // can_edit: change its todos; can_rename: its name, icon and archiving; can_manage: delete
    // others' todos.
    can_edit: boolean;
    can_rename: boolean;
    can_delete: boolean;
    can_manage: boolean;
    open_count: number;

    // The earliest due date of its open todos (YYYY-MM-DD).
    earliest_due?: string;
};

// A todo of a list (server/items.go).
export type Item = {
    id: string;
    list_id: string;
    message: string;
    description?: string;
    creator_id: string;
    create_at: number;
    update_at: number;
    assignee_id?: string;

    // The day it's due (YYYY-MM-DD).
    due?: string;
    done_by?: string;
    done_at?: number;
    post_id?: string;
    post_permalink?: string;

    // Someone sent it to the user, who accepted it.
    from_user_id?: string;
};

export type ListDetail = ListInfo & {items: Item[]};

// The changes of a todo the server accepts. channel_id is the channel it's added from.
export type ItemPatch = {
    message?: string;
    description?: string;
    assignee_id?: string;
    due?: string;
    done?: boolean;
    position?: number;
    post_id?: string;
    channel_id?: string;
};

export type ListPatch = {
    name?: string;
    icon?: string;
    archived?: boolean;
    position?: number;
};

// The lists the user can add a todo to from a channel, and the one to offer first.
export type Targets = {
    lists: ListInfo[];
    default_id: string;
};

// A todo assigned to the user, with its list and channel.
export type AssignedTodo = Item & {
    channel_id: string;
    list_name: string;
    list_icon?: string;
    channel_name: string;
    channel_url_name: string;
    team_name?: string;
};
