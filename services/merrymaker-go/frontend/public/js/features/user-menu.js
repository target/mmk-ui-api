/**
 * User menu feature module
 *
 * Manages dropdown menu toggling, closing on outside clicks, and accessibility
 * attributes for user navigation controls.
 */
import { qsa } from "../core/dom.js";
import { Events } from "../core/events.js";

let initialized = false;

function closeAllUserMenus() {
	const toggles = [];

	qsa(".user-toggle-menu.is-open").forEach((menu) => {
		menu.classList.remove("is-open");
		const btn = menu.parentElement?.querySelector("[data-user-toggle]");
		if (btn) {
			btn.setAttribute("aria-expanded", "false");
			toggles.push(btn);
		}
	});

	return toggles;
}

function toggleUserMenu(toggle, menu) {
	const isOpen = menu.classList.contains("is-open");

	qsa(".user-toggle-menu.is-open").forEach((otherMenu) => {
		if (otherMenu === menu) return;

		otherMenu.classList.remove("is-open");
		const otherBtn = otherMenu.parentElement?.querySelector("[data-user-toggle]");
		if (otherBtn) otherBtn.setAttribute("aria-expanded", "false");
	});

	menu.classList.toggle("is-open", !isOpen);
	toggle.setAttribute("aria-expanded", !isOpen ? "true" : "false");
}

function registerEventHandlers() {
	Events.on("[data-user-toggle]", "click", (event, toggle) => {
		event.preventDefault();
		event.stopPropagation();

		const menu = toggle.parentElement?.querySelector("[data-user-menu]");
		if (!menu) return;

		toggleUserMenu(toggle, menu);
	});

	document.addEventListener("click", (event) => {
		const toggle = event.target?.closest("[data-user-toggle]");
		const inMenu = event.target?.closest("[data-user-menu]");

		if (!(toggle || inMenu)) {
			closeAllUserMenus();
		}
	});
}

export function initUserMenu() {
	if (initialized) return;
	initialized = true;
	registerEventHandlers();
}

export { closeAllUserMenus };
