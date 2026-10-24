// Copyright (c) 2026-present Antimatter contributors.
// See LICENSE for license information.

import {webLink} from './todo_row';

// Regression test: todos made before the server checked the permalink keep whatever their client
// sent, which could be a javascript: URL. The panel and the bot's todo messages must only ever
// link to it if it's a web link, never render it unfiltered (that would be a stored-XSS vector).
describe('webLink', () => {
    test('keeps http(s) links', () => {
        expect(webLink('https://chat.example/lumen/pl/post1')).toBe('https://chat.example/lumen/pl/post1');
        expect(webLink('http://chat.example/lumen/pl/post1')).toBe('http://chat.example/lumen/pl/post1');
    });

    test('drops a javascript: URL', () => {
        // eslint-disable-next-line no-script-url -- this is the dangerous link webLink must reject
        expect(webLink('javascript:alert(1)')).toBe('');
    });

    test('drops other non-web schemes', () => {
        expect(webLink('data:text/html,<script>alert(1)</script>')).toBe('');
        expect(webLink('mailto:a@b.com')).toBe('');
    });

    test('is empty for no link', () => {
        expect(webLink()).toBe('');
        expect(webLink('')).toBe('');
    });
});
