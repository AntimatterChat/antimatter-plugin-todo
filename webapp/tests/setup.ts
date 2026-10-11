// Copyright (c) 2026-present Antimatter contributors.
// See LICENSE for license information.

import '@testing-library/jest-dom';

// The host registers plugins; the tests import the plugin's entry point.
window.registerPlugin = jest.fn();
