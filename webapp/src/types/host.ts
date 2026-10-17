// Copyright (c) 2026-present Antimatter contributors.
// See LICENSE for license information.

import type React from 'react';
import type {AnyAction, Store} from 'redux';

import type {Channel} from '@mattermost/types/channels';
import type {Post} from '@mattermost/types/posts';
import type {GlobalState} from '@mattermost/types/store';

import type {WebUI} from '../ui/web_ui';

export type WebSocketMessage = {
    event: string;
    data: Record<string, unknown>;
};

export type RightHandSidebarRegistration = {
    id: string;
    showRHSPlugin: AnyAction;
    hideRHSPlugin: AnyAction;
    toggleRHSPlugin: AnyAction;
};

// The props Antimatter's Fusion UI gives an app's panel: where the panel is goes to setPanelState
// (a short string, null for its home), and a panel opened in a new window gets it back as
// initialPanelState (untrusted: it comes from the window's address). Other hosts give neither.
export type PanelProps = {
    setPanelState?: (state: string | null) => void;
    initialPanelState?: string;
};

// The subset of the host's plugin registry this plugin uses.
export interface PluginRegistry {
    registerRootComponent(component: React.ComponentType): string;
    registerPostTypeComponent(type: string, component: React.ComponentType<{post: Post}>): string;
    registerWebSocketEventHandler(event: string, handler: (msg: WebSocketMessage) => void): void;
    registerReconnectHandler(handler: () => void): void;
    registerRightHandSidebarComponent(component: React.ComponentType<PanelProps>, title: React.ReactNode): RightHandSidebarRegistration;
    registerChannelHeaderButtonAction(icon: React.ReactNode, action: (channel: Channel) => void, dropdownText: React.ReactNode, tooltipText: React.ReactNode): string;
    registerPostDropdownMenuAction(text: React.ReactNode, action: (postId: string) => void, filter?: (postId: string) => boolean): string;
    registerAppBarComponent?(options: {
        iconUrl: string;
        tooltipText: React.ReactNode;
        rhsComponent: React.ComponentType<PanelProps>;
        rhsTitle: React.ReactNode;
    }): {id: string; rhsComponent: RightHandSidebarRegistration} | string;
}

export type PluginStore = Store<GlobalState>;

export interface PluginClass {
    initialize(registry: PluginRegistry, store: PluginStore): void | Promise<void>;
    uninitialize?(): void;
}

declare global {
    interface Window {
        registerPlugin(pluginId: string, plugin: PluginClass): void;
        basename?: string;

        // Set by the Antimatter web UIs before plugins load
        antimatterWebUI?: WebUI;

        // The host's utilities for plugins
        WebappUtils?: {browserHistory?: {push(path: string): void}};
        PostUtils?: {
            formatText(text: string, options?: Record<string, unknown>): string;
            messageHtmlToComponent(html: string, options?: Record<string, unknown>): React.ReactNode;
        };
    }
}
