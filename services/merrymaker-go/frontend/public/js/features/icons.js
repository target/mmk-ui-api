/**
 * Lucide icon feature module
 *
 * Initializes Lucide icons on initial load and after HTMX swaps to ensure newly
 * injected fragments receive icon treatment.
 */
import { on } from "../core/htmx-bridge.js";

let initialized = false;

function createIcons() {
	try {
		if (!window.lucide || typeof window.lucide.createIcons !== "function") return;

		window.lucide.createIcons(
			{ icons: window.lucide.icons },
			{ attrs: {}, nameAttr: "data-lucide" },
		);
	} catch (_) {
		/* noop */
	}
}

function registerEventHandlers() {
	const refresh = () => {
		createIcons();
	};

	if (document.readyState === "loading") {
		document.addEventListener("DOMContentLoaded", refresh, { once: true });
	} else {
		refresh();
	}

	on("htmx:afterSwap", refresh);
	on("htmx:historyRestore", refresh);
}

export function initIcons() {
	if (initialized) return;
	initialized = true;
	registerEventHandlers();
}
