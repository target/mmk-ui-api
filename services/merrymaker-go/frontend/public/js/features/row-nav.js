/**
 * Row navigation feature module
 *
 * Provides delegated handling for clickable table/list rows. Elements marked
 * with `.row-link` will trigger navigation when activated via click or keyboard,
 * unless an ancestor has `data-stop-row-nav` set.
 *
 * Supported attributes:
 * - data-row-nav-target: optional selector defining the element to activate.
 *   Defaults to the row element itself.
 */
import { Events } from "../core/events.js";

let initialized = false;

function getNavTarget(row) {
	const selector = row.getAttribute("data-row-nav-target");
	if (!selector) return row;
	return row.querySelector(selector) || row;
}

function triggerNavigation(target) {
	if (!target) return;
	if (window.htmx && typeof window.htmx.trigger === "function") {
		window.htmx.trigger(target, "row-nav");
		return;
	}

	if (typeof target.click === "function") {
		target.click();
	}
}

function isInteractiveControl(event) {
	const control = event.target?.closest("[data-stop-row-nav], button, a, input, select, textarea");
	return Boolean(control);
}

function registerHandlers() {
	Events.on(".row-link", "click", (event, row) => {
		// Ignore clicks on interactive controls inside the row
		if (isInteractiveControl(event)) return;

		triggerNavigation(getNavTarget(row));
	});

	Events.on(".row-link", "keydown", (event, row) => {
		if (event.key !== "Enter" && event.key !== " ") return;

		// Allow space/enter to activate interactive children normally
		if (isInteractiveControl(event)) return;

		event.preventDefault();
		triggerNavigation(getNavTarget(row));
	});
}

export function initRowNav() {
	if (initialized) return;
	initialized = true;
	registerHandlers();
}
