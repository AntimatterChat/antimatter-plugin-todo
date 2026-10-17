// Copyright (c) 2026-present Antimatter contributors.
// See LICENSE for license information.

import React from 'react';
import {FormattedMessage} from 'react-intl';

import {Client4} from 'mattermost-redux/client';
import {getConfig} from 'mattermost-redux/selectors/entities/general';
import {getCurrentTeam} from 'mattermost-redux/selectors/entities/teams';

import TodoHeader from './components/todo_header';
import TodoPanel from './components/todo_panel';
import TodoPost from './components/todo_post';
import manifest from './manifest';
import {messages} from './messages';
import {openDraft, refreshAfterChange, refreshIndex, refreshRequests, setShowPanel} from './state';
import type {PluginClass, PluginRegistry, PluginStore, RightHandSidebarRegistration} from './types/host';
import Icon from './ui/icon';
import {registerTranslations} from './translations';

import './styles.css';

// The app bar's icon: a checklist, in the mockup's colors.
const appBarIcon = 'data:image/svg+xml;utf8,' + encodeURIComponent(
    '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="#34D399" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round">' +
    '<rect x="3" y="4" width="6" height="6" rx="1.5"/><path d="m4.5 7 1.2 1.2L7.8 6M12 7h9M3 14h6v6H3zM12 17h9"/></svg>',
);

// How long the user may be away before the daily reminder is checked again.
const REMINDER_IDLE = 60 * 60 * 1000;

const title = <FormattedMessage {...messages.app}/>;

// The panel's header shows the app's own: "Todo" and the + over the lists, back and the list's
// name and actions over an open list.
const header = <TodoHeader/>;

export default class Plugin implements PluginClass {
    private onActivity?: () => void;

    public initialize(registry: PluginRegistry, store: PluginStore) {
        registerTranslations(registry);

        if (window.basename) {
            Client4.setUrl(window.basename);
        }

        // The app bar opens the todo panel; hosts without it get a channel header button.
        let rhs: RightHandSidebarRegistration;
        const appBar = registry.registerAppBarComponent?.({iconUrl: appBarIcon, tooltipText: <FormattedMessage {...messages.appTooltip}/>, rhsComponent: TodoPanel, rhsTitle: header});
        if (appBar && typeof appBar === 'object') {
            rhs = appBar.rhsComponent;
        } else {
            rhs = registry.registerRightHandSidebarComponent(TodoPanel, header);
        }
        registry.registerChannelHeaderButtonAction(
            <Icon name='checkbox'/>,
            () => store.dispatch(rhs.toggleRHSPlugin),
            title,
            <FormattedMessage {...messages.appTooltip}/>,
        );
        setShowPanel(() => store.dispatch(rhs.showRHSPlugin));

        // A message's "Add to todo" starts a todo with its text, linked to it, and asks which list.
        registry.registerPostDropdownMenuAction(
            <FormattedMessage {...messages.addFromMessage}/>,
            (postId) => {
                const state = store.getState();
                const post = state.entities.posts.posts[postId];
                if (!post) {
                    return;
                }
                const team = getCurrentTeam(state);
                const siteURL = (getConfig(state).SiteURL || window.location.origin).replace(/\/$/, '');
                openDraft({
                    message: post.message.slice(0, 1000),
                    channelId: post.channel_id,
                    postId,
                    postPermalink: team ? `${siteURL}/${team.name}/pl/${postId}` : '',
                });
            },
        );

        registry.registerPostTypeComponent('custom_todo', TodoPost);

        // The todos sent between people and the assignments changed; a list (or its todos) changed.
        registry.registerWebSocketEventHandler(`custom_${manifest.id}_refresh`, () => refreshRequests());
        registry.registerWebSocketEventHandler(`custom_${manifest.id}_list_changed`, (msg) => {
            if (typeof msg.data.list_id === 'string') {
                refreshAfterChange(msg.data.list_id, typeof msg.data.channel_id === 'string' ? msg.data.channel_id : undefined);
            }
        });
        registry.registerReconnectHandler(() => {
            refreshRequests();
            refreshIndex('mine');
        });

        // The server sends the daily reminder when the lists are loaded for it: now, and when the
        // user comes back after an hour away.
        refreshRequests(true);
        let last = Date.now();
        this.onActivity = () => {
            const now = Date.now();
            if (now - last > REMINDER_IDLE) {
                refreshRequests(true);
            }
            last = now;
        };
        document.addEventListener('click', this.onActivity);
    }

    public uninitialize() {
        if (this.onActivity) {
            document.removeEventListener('click', this.onActivity);
        }
    }
}

window.registerPlugin(manifest.id, new Plugin());
