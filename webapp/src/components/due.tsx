// Copyright (c) 2026-present Antimatter contributors.
// See LICENSE for license information.

import React from 'react';
import {useIntl} from 'react-intl';

import {messages} from '../messages';
import Icon from '../ui/icon';

// Due dates are days (YYYY-MM-DD), in the user's time zone.

const pad = (n: number) => String(n).padStart(2, '0');

export function dayOf(date: Date): string {
    return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}`;
}

export function today(): string {
    return dayOf(new Date());
}

function addDays(day: string, days: number): string {
    const [y, m, d] = day.split('-').map(Number);
    return dayOf(new Date(y, m - 1, d + days));
}

// How soon a day is: overdue, today, tomorrow, soon (within the next days) or later.
export type Urgency = 'overdue' | 'today' | 'tomorrow' | 'soon' | 'later';

// SOON is how many days ahead a list is marked as due soon.
const SOON = 3;

export function urgency(due: string, now = today()): Urgency {
    if (due < now) {
        return 'overdue';
    }
    if (due === now) {
        return 'today';
    }
    if (due === addDays(now, 1)) {
        return 'tomorrow';
    }
    return due <= addDays(now, SOON) ? 'soon' : 'later';
}

// DueMark says when a todo (or a list's first open todo) is due, marked when it's overdue or soon.
// onlyUrgent shows nothing for a day that isn't soon: the lists only mark what needs attention.
export function DueMark({due, onlyUrgent = false}: {due?: string; onlyUrgent?: boolean}) {
    const intl = useIntl();
    if (!due) {
        return null;
    }
    const how = urgency(due);
    if (onlyUrgent && how === 'later') {
        return null;
    }
    let label: string;
    switch (how) {
    case 'overdue':
        label = intl.formatMessage(messages.dueOverdue);
        break;
    case 'today':
        label = intl.formatMessage(messages.dueToday);
        break;
    case 'tomorrow':
        label = intl.formatMessage(messages.dueTomorrow);
        break;
    default: {
        const [y, m, d] = due.split('-').map(Number);
        label = intl.formatMessage(messages.dueOn, {date: intl.formatDate(new Date(y, m - 1, d), {month: 'short', day: 'numeric'})});
    }
    }
    return (
        <span
            className={'amt-due amt-due-' + how}
            title={due}
        >
            <Icon
                name='calendar'
                size='xs'
            />
            {label}
        </span>
    );
}
