/**
 * Network event helpers feature module
 *
 * Provides toggle support for expanding and collapsing long network URLs in
 * event detail views.
 */
import { Events } from "../core/events.js";

let initialized = false;

function registerEventHandlers() {
	Events.on('[data-action="toggle-url"]', "click", (_event, button) => {
		const span = button.previousElementSibling;
		if (!span?.dataset.fullUrl) return;

		const isExpanded = button.dataset.expanded === "true";
		const nextExpanded = !isExpanded;
		const nextUrl = nextExpanded
			? span.dataset.fullUrl
			: (span.dataset.truncatedUrl ?? span.dataset.fullUrl ?? "");
		const nextLabel = nextExpanded ? "Show less" : "Show full";

		const schedule = window.requestAnimationFrame?.bind(window) ?? ((fn) => setTimeout(fn, 16));
		schedule(() => {
			if (!(span.isConnected && button.isConnected)) return;

			span.textContent = nextUrl;
			button.dataset.expanded = String(nextExpanded);
			button.setAttribute("aria-pressed", String(nextExpanded));
			button.textContent = nextLabel;
		});
	});
}

export function initNetworkDetails() {
	if (initialized) return;
	initialized = true;
	registerEventHandlers();
}
