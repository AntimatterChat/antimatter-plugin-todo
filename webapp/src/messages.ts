// Copyright (c) 2026-present Antimatter contributors.
// See LICENSE for license information.

import {defineMessages} from 'react-intl';

export const messages = defineMessages({
    app: {id: 'todo.app', defaultMessage: 'Todo'},
    appTooltip: {id: 'todo.appTooltip', defaultMessage: 'Todo lists'},
    addFromMessage: {id: 'todo.addFromMessage', defaultMessage: 'Add to todo'},

    viewChannel: {id: 'todo.view.channel', defaultMessage: 'This channel'},
    viewConversation: {id: 'todo.view.conversation', defaultMessage: 'This conversation'},
    viewMine: {id: 'todo.view.mine', defaultMessage: 'Mine'},

    channelOpen: {id: 'todo.channel.open', defaultMessage: 'Shared with {channel}'},
    channelEmpty: {id: 'todo.channel.empty', defaultMessage: 'No todos here yet. Todos added here are shared with everyone in the channel.'},
    channelReadOnly: {id: 'todo.channel.readOnly', defaultMessage: 'You can see the todos of this channel, not change them.'},
    done: {id: 'todo.done', defaultMessage: 'Done ({count})'},
    noChannel: {id: 'todo.noChannel', defaultMessage: 'Open a channel to see its todos.'},

    mine: {id: 'todo.mine', defaultMessage: 'My todos'},
    mineEmpty: {id: 'todo.mineEmpty', defaultMessage: 'Nothing to do! Only you see your own todos.'},
    assigned: {id: 'todo.assigned', defaultMessage: 'Assigned to you in channels'},
    requests: {id: 'todo.requests', defaultMessage: 'Sent to you'},
    sent: {id: 'todo.sent', defaultMessage: 'Sent by you'},

    addPlaceholder: {id: 'todo.addPlaceholder', defaultMessage: 'Add a todo…'},
    addPlaceholderChannel: {id: 'todo.addPlaceholderChannel', defaultMessage: 'Add a todo for {channel}…'},
    description: {id: 'todo.description', defaultMessage: 'Details (optional)'},
    details: {id: 'todo.details', defaultMessage: 'Details'},
    add: {id: 'todo.add', defaultMessage: 'Add'},
    send: {id: 'todo.sendButton', defaultMessage: 'Send'},
    save: {id: 'todo.save', defaultMessage: 'Save'},
    cancel: {id: 'todo.cancel', defaultMessage: 'Cancel'},
    fromMessage: {id: 'todo.fromMessage', defaultMessage: 'From a message'},
    assignTo: {id: 'todo.assignTo', defaultMessage: 'Assign to…'},
    sendTo: {id: 'todo.sendTo', defaultMessage: 'Send to…'},
    unassign: {id: 'todo.unassign', defaultMessage: 'Unassign'},
    searchPeople: {id: 'todo.searchPeople', defaultMessage: 'Search people'},
    noPeople: {id: 'todo.noPeople', defaultMessage: 'Nobody matches.'},

    markDone: {id: 'todo.markDone', defaultMessage: 'Mark as done'},
    reopen: {id: 'todo.reopen', defaultMessage: 'Mark as not done'},
    more: {id: 'todo.more', defaultMessage: 'More actions'},
    editTodo: {id: 'todo.edit', defaultMessage: 'Edit'},
    moveTop: {id: 'todo.moveTop', defaultMessage: 'Move to the top'},
    deleteTodo: {id: 'todo.delete', defaultMessage: 'Delete'},
    accept: {id: 'todo.accept', defaultMessage: 'Accept'},
    decline: {id: 'todo.decline', defaultMessage: 'Decline'},
    bump: {id: 'todo.bump', defaultMessage: 'Remind them'},
    cancelSent: {id: 'todo.cancelSent', defaultMessage: 'Cancel'},
    jump: {id: 'todo.jump', defaultMessage: 'Jump to the message'},

    assignedTo: {id: 'todo.assignedTo', defaultMessage: 'Assigned to {name}'},
    doneBy: {id: 'todo.doneBy', defaultMessage: 'Done by {name}'},
    from: {id: 'todo.from', defaultMessage: 'From {name}'},
    to: {id: 'todo.to', defaultMessage: 'To {name}'},
    inChannel: {id: 'todo.inChannel', defaultMessage: 'In {channel}'},

    postAutomated: {id: 'todo.post.automated', defaultMessage: 'Todo'},
    postAccepted: {id: 'todo.post.handled', defaultMessage: 'Answered'},
    error: {id: 'todo.error', defaultMessage: 'Something went wrong: {error}'},
});
