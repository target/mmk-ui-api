/**
 * Test setup for Bun test runner
 * Configures happy-dom for DOM testing
 */

import { Window } from 'happy-dom';

// Create a window instance
const window = new Window();
const document = window.document;

// Set globals
global.window = window;
global.document = document;
global.navigator = window.navigator;
global.HTMLElement = window.HTMLElement;
global.Element = window.Element;
global.Node = window.Node;
global.KeyboardEvent = window.KeyboardEvent;
global.MouseEvent = window.MouseEvent;
global.CustomEvent = window.CustomEvent;

// Ensure window has SyntaxError constructor
if (!window.SyntaxError) {
	window.SyntaxError = SyntaxError;
}

