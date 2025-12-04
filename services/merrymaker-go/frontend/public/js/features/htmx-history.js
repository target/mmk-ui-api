/**
 * HTMX history helpers
 *
 * Ensures forms can opt into pushState updates on submit-driven HTMX requests
 * without embedding inline `hx-on` handlers in templates.
 */
import { on } from "../core/htmx-bridge.js";

const PUSH_ON_SUBMIT_ATTR = "data-hx-push-url-on-submit";
let initialized = false;

function isSubmitRequest(detail) {
	return detail?.requestConfig?.triggeringEvent?.type === "submit";
}

function elementWithOptIn(detail, fallbackTarget) {
	const elt = detail?.elt ?? detail?.target ?? fallbackTarget;
	if (!(elt instanceof Element)) {
		return null;
	}
	return elt.closest(`[${PUSH_ON_SUBMIT_ATTR}]`);
}

function getFinalPath(detail) {
	const pathInfo = detail?.pathInfo;
	if (pathInfo?.finalPath) {
		return pathInfo.finalPath;
	}
	if (pathInfo?.requestPath) {
		return pathInfo.requestPath;
	}
	return detail?.requestConfig?.path ?? null;
}

function pushStateIfPossible(path) {
	if (!path) return;
	if (typeof window === "undefined") return;
	if (!window.history || typeof window.history.pushState !== "function") {
		return;
	}

	try {
		window.history.pushState({}, "", path);
	} catch (error) {
		console.warn("htmx-history: failed to push state", error);
	}
}

function handleAfterRequest(event) {
	const { detail } = event;
	if (!detail) return;
	if (!isSubmitRequest(detail)) return;

	const target = elementWithOptIn(detail, event.target);
	if (!target) return;

	const finalPath = getFinalPath(detail);
	pushStateIfPossible(finalPath);
}

function registerHandlers() {
	on("htmx:afterRequest", handleAfterRequest);
}

export function initHtmxHistory() {
	if (initialized) return;
	initialized = true;
	registerHandlers();
}
