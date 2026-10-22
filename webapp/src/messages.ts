// Copyright (c) 2026-present Antimatter contributors.
// See LICENSE for license information.

import {defineMessages} from 'react-intl';

export const messages = defineMessages({
    app: {id: 'todo.app', defaultMessage: 'Todo'},
    appTooltip: {id: 'todo.appTooltip', defaultMessage: 'Todo lists'},
    addFromMessage: {id: 'todo.addFromMessage', defaultMessage: 'Add to todo'},

    // The tabs of the panel
    viewChannel: {id: 'todo.view.channel', defaultMessage: 'This channel'},
    viewConversation: {id: 'todo.view.conversation', defaultMessage: 'This conversation'},
    viewMine: {id: 'todo.view.mine', defaultMessage: 'Mine'},
    viewFavorites: {id: 'todo.view.favorites', defaultMessage: 'Favourites', description: 'Tab of the Todo panel: the lists the user starred, from any channel'},

    // The lists of a tab
    newListChannel: {id: 'todo.newList.channel', defaultMessage: 'New list in this channel', description: 'Button (+) of the Todo panel\'s "Shared with {channel}" row, under its tabs: makes a todo list shared with the channel'},
    newListConversation: {id: 'todo.newList.conversation', defaultMessage: 'New list in this conversation', description: 'Button (+) of the Todo panel\'s "Shared with this conversation" row, in a direct or group message'},
    newListPersonal: {id: 'todo.newList.personal', defaultMessage: 'New personal list', description: 'Button (+) of the Todo panel\'s "Personal lists" row, in the Mine tab: a list only the user sees'},
    newListName: {id: 'todo.newList.name', defaultMessage: 'Name of the new list', description: 'Placeholder of the name field when making a todo list'},
    create: {id: 'todo.create', defaultMessage: 'Create', description: 'Button: makes the new todo list'},
    listsChannel: {id: 'todo.lists.channel', defaultMessage: 'Shared with {channel}', description: 'Header row of the This channel tab of the Todo panel, over its lists: {channel} is the channel\'s name, or "this conversation"'},
    listsConversationName: {id: 'todo.lists.conversation', defaultMessage: 'this conversation', description: 'The {channel} of "Shared with {channel}" in a direct or group message'},
    listsFavorites: {id: 'todo.lists.favorites', defaultMessage: 'Favourites', description: 'Header row of the Favourites tab of the Todo panel, over the starred lists'},
    listsPersonal: {id: 'todo.lists.personal', defaultMessage: 'Personal lists', description: 'Header row of the Mine tab of the Todo panel, over the lists only the user sees'},
    listsLoading: {id: 'todo.lists.loading', defaultMessage: 'Loading lists…', description: 'Shown in the Todo panel while a tab\'s lists load'},
    listsChannelEmptyTitle: {id: 'todo.lists.channelEmptyTitle', defaultMessage: 'No lists here yet', description: 'Title of the This channel (or This conversation) tab of the Todo panel when it has no lists'},
    listsChannelEmptyText: {id: 'todo.lists.channelEmptyText', defaultMessage: 'Every member of the channel can edit the lists added here.', description: 'Under "No lists here yet" in the Todo panel: who can edit a channel\'s lists'},
    listsConversationEmptyText: {id: 'todo.lists.conversationEmptyText', defaultMessage: 'Every member of the conversation can edit the lists added here.', description: 'Under "No lists here yet" in the Todo panel, in a direct or group message: who can edit its lists'},
    listsFavoritesEmptyTitle: {id: 'todo.lists.favoritesEmptyTitle', defaultMessage: 'Nothing starred yet', description: 'Title of the empty Favourites tab of the Todo panel: the user has starred no list'},
    listsFavoritesEmpty: {id: 'todo.lists.favoritesEmpty', defaultMessage: 'Star a list to find it here, wherever it is.', description: 'Under "Nothing starred yet" in the Favourites tab'},
    listsPersonalEmptyTitle: {id: 'todo.lists.personalEmptyTitle', defaultMessage: 'No personal lists yet', description: 'Title of the Mine tab of the Todo panel when the user has no personal lists'},
    listsMineNote: {id: 'todo.lists.mineNote', defaultMessage: 'Only you can see your personal lists.', description: 'Note under the lists of the Mine tab, and the sentence under "No personal lists yet"'},
    archivedLists: {id: 'todo.lists.archived', defaultMessage: 'Archived ({count})', description: 'Folded section of the archived todo lists, with how many there are'},
    openCount: {id: 'todo.lists.open', defaultMessage: '{count, plural, =0 {Nothing left to do} other {# open}}', description: 'Under a todo list\'s name: how many of its todos are still open'},

    // Due dates
    dueOverdue: {id: 'todo.due.overdue', defaultMessage: 'Overdue', description: 'Mark of a todo, or of a list with a todo, past its due date'},
    dueToday: {id: 'todo.due.today', defaultMessage: 'Due today', description: 'Mark of a todo (or list) due today'},
    dueTomorrow: {id: 'todo.due.tomorrow', defaultMessage: 'Due tomorrow', description: 'Mark of a todo (or list) due tomorrow'},
    dueOn: {id: 'todo.due.on', defaultMessage: 'Due {date}', description: 'When a todo is due, e.g. "Due Oct 20"'},
    dueDate: {id: 'todo.due.date', defaultMessage: 'Due date', description: 'Button and field: the day a todo is due'},
    dueClear: {id: 'todo.due.clear', defaultMessage: 'No due date', description: 'Button: removes a todo\'s due date'},
    setDue: {id: 'todo.due.set', defaultMessage: 'Set a due date', description: 'Action of a todo\'s menu'},

    // Where a list is
    scopePersonal: {id: 'todo.scope.personal', defaultMessage: 'Personal', description: 'Chip of a list only the user sees'},
    scopePersonalTip: {id: 'todo.scope.personalTip', defaultMessage: 'Personal: only you can see it', description: 'Tooltip of the chip of a personal list'},
    scopeConversation: {id: 'todo.scope.conversation', defaultMessage: 'Conversation', description: 'Chip of a list of a direct or group message'},
    scopeConversationTip: {id: 'todo.scope.conversationTip', defaultMessage: 'Shared with everyone in this conversation', description: 'Tooltip of the chip of a conversation\'s list'},
    scopeSharedTip: {id: 'todo.scope.sharedTip', defaultMessage: 'Shared with everyone in #{channel}', description: 'Tooltip of the chip of a channel\'s list'},
    favoriteAdd: {id: 'todo.favorite.add', defaultMessage: 'Add to favourites', description: 'Star button of a todo list'},
    favoriteRemove: {id: 'todo.favorite.remove', defaultMessage: 'Remove from favourites', description: 'Star button of a todo list in the favourites'},

    // An open list
    back: {id: 'todo.back', defaultMessage: 'All lists', description: 'Button back from an open todo list to the lists'},
    listName: {id: 'todo.list.name', defaultMessage: 'List name', description: 'Label of the editable name of an open todo list'},
    listMenu: {id: 'todo.list.menu', defaultMessage: 'List actions', description: 'The ⋯ menu of an open todo list'},
    rename: {id: 'todo.list.rename', defaultMessage: 'Rename', description: 'Action of a todo list\'s menu'},
    changeIcon: {id: 'todo.list.icon', defaultMessage: 'Change icon', description: 'Action of a todo list\'s menu: pick an emoji for it'},
    noIcon: {id: 'todo.list.noIcon', defaultMessage: 'No icon', description: 'Choice of the icon picker: the list has no emoji'},
    archive: {id: 'todo.list.archive', defaultMessage: 'Archive', description: 'Action of a todo list\'s menu: hides it under Archived, read-only'},
    unarchive: {id: 'todo.list.unarchive', defaultMessage: 'Unarchive', description: 'Action of an archived todo list\'s menu'},
    deleteList: {id: 'todo.list.delete', defaultMessage: 'Delete list', description: 'Action of a todo list\'s menu'},
    deleteConfirm: {id: 'todo.list.deleteConfirm', defaultMessage: 'Delete “{name}” and its todos? This can\'t be undone.', description: 'In-page confirmation before deleting a personal todo list'},
    deleteConfirmShared: {id: 'todo.list.deleteConfirmShared', defaultMessage: 'Delete “{name}” and its todos for everyone? This can\'t be undone.', description: 'In-page confirmation before deleting a channel\'s todo list'},
    archivedNote: {id: 'todo.list.archivedNote', defaultMessage: 'This list is archived: unarchive it to change its todos.', description: 'Note at the top of an archived todo list'},
    listNotFound: {id: 'todo.list.notFound', defaultMessage: 'This list was deleted, or you can\'t open it.', description: 'Shown instead of a todo list that is gone'},
    addToList: {id: 'todo.addToList', defaultMessage: 'Add a todo to {list}…', description: 'Placeholder of the field adding a todo to an open list'},
    pickList: {id: 'todo.pickList', defaultMessage: 'List', description: 'Label of the list picker when adding a todo from a message'},

    channelEmpty: {id: 'todo.channel.empty', defaultMessage: 'No todos here yet. Todos added here are shared with everyone in the channel.'},
    channelReadOnly: {id: 'todo.channel.readOnly', defaultMessage: 'You can see the todos of this channel, not change them.'},
    done: {id: 'todo.done', defaultMessage: 'Done ({count})'},

    mineEmpty: {id: 'todo.mineEmpty', defaultMessage: 'Nothing to do! Only you see your own todos.'},
    assigned: {id: 'todo.assigned', defaultMessage: 'Assigned to you in channels'},
    requests: {id: 'todo.requests', defaultMessage: 'Sent to you'},
    sent: {id: 'todo.sent', defaultMessage: 'Sent by you'},

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
    inList: {id: 'todo.inList', defaultMessage: '{list} in {channel}', description: 'Where a todo assigned to the user is: its list, in its channel'},

    postAutomated: {id: 'todo.post.automated', defaultMessage: 'Todo'},
    postAccepted: {id: 'todo.post.handled', defaultMessage: 'Answered'},
    error: {id: 'todo.error', defaultMessage: 'Something went wrong: {error}'},
});
