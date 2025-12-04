/**
 * Sidebar controls feature module
 *
 * Handles sidebar open/close state, accessibility attributes, and responsive
 * behaviors such as auto-closing on viewport changes or HTMX swaps.
 */
import { qsa } from "../core/dom.js";
import { Events } from "../core/events.js";
import { on } from "../core/htmx-bridge.js";

const BREAKPOINT = 1024;
let initialized = false;

function syncToggleButtons(isOpen) {
	qsa("[data-sidebar-toggle]").forEach((btn) => {
		btn.setAttribute("aria-expanded", isOpen ? "true" : "false");
	});
}

function toggleSidebar() {
	document.body.classList.toggle("sidebar-open");
	const isOpen = document.body.classList.contains("sidebar-open");
	syncToggleButtons(isOpen);
}

function closeSidebar() {
	if (!document.body.classList.contains("sidebar-open")) return;
	document.body.classList.remove("sidebar-open");
	syncToggleButtons(false);
}

function registerEventHandlers() {
	Events.on("[data-sidebar-toggle]", "click", () => {
		toggleSidebar();
	});

	Events.on("[data-sidebar-close], .sidebar-overlay", "click", () => {
		closeSidebar();
	});

	window.addEventListener("resize", () => {
		if (window.innerWidth > BREAKPOINT) {
			closeSidebar();
		}
	});

	on("htmx:afterSwap", closeSidebar);
	on("htmx:historyRestore", closeSidebar);
}

export function initSidebar() {
	if (initialized) return;
	initialized = true;
	registerEventHandlers();
}

export { closeSidebar };
