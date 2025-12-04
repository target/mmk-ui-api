/**
 * Utility helpers for HTMX-driven fragments.
 * Provides a declarative pattern for showing/hiding error states
 * without relying on verbose inline hx-on attributes.
 */
import { on } from "../core/htmx-bridge.js";

const PANEL_SELECTOR = "[data-fragment-panel]";
const ERROR_SELECTOR = "[data-fragment-error]";

/**
 * Hide all error elements within a fragment panel.
 * @param {Element} panel
 */
function hideErrors(panel) {
	panel.querySelectorAll(ERROR_SELECTOR).forEach((el) => {
		el.setAttribute("hidden", "");
	});
}

/**
 * Show all error elements within a fragment panel.
 * @param {Element} panel
 */
function showErrors(panel) {
	panel.querySelectorAll(ERROR_SELECTOR).forEach((el) => {
		el.removeAttribute("hidden");
	});
}

/**
 * Extract the fragment panel associated with an HTMX event.
 * @param {CustomEvent} event
 * @returns {Element|null}
 */
function panelFromEvent(event) {
	const elt = event.detail?.elt ?? event.detail?.target ?? event.target;
	if (!(elt instanceof Element)) {
		return null;
	}
	return elt.closest(PANEL_SELECTOR);
}

/**
 * Initialize fragment helpers.
 * Wires HTMX lifecycle events to toggle error states on panels.
 */
let helpersInitialized = false;

export function initFragmentHelpers() {
	if (helpersInitialized || typeof document === "undefined") {
		return;
	}
	helpersInitialized = true;

	on("htmx:beforeRequest", (event) => {
		const panel = panelFromEvent(event);
		if (!panel) return;
		hideErrors(panel);
	});

	const showErrorHandler = (event) => {
		const panel = panelFromEvent(event);
		if (!panel) return;
		showErrors(panel);
	};

	on("htmx:responseError", showErrorHandler);
	on("htmx:sendError", showErrorHandler);
	on("htmx:timeout", showErrorHandler);
}
