// Copyright (c) 2026-present Antimatter contributors.
// See LICENSE for license information.

import React from 'react';

// TodoToolbar is the open list's toolbar: the row at the top of the panel where the list (or a todo
// started from a message) puts its back button, its name and its actions, under the host's panel
// header. The header is the host's and the same for every app (the app's icon and name, its size,
// a new window, close): nothing of the app goes in it. The lists have no toolbar: their tabs and
// the tab's header row, with the +, top them.
export function TodoToolbar({children}: {children: React.ReactNode}) {
    return (
        <div
            className='amt-bar'
            data-testid='todo-toolbar'
        >
            {children}
        </div>
    );
}
