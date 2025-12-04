/**
 * App initialization for the browser UI
 * - Boots feature modules registered with the app shell
 * - Boots component lifecycle integrations
 */
import { Lifecycle } from "./core/lifecycle.js";
import "./alert_sink_form.js";
import "./components/filter-pills.js";
import "./components/toast.js";
import "./components/row-delete.js";
import "./components/ioc-form.js";
import "./job_status.js";
import "./source_form.js";
import { bootFeatures } from "./features/index.js";
import { closeSidebar } from "./features/sidebar.js";
import { closeAllUserMenus } from "./features/user-menu.js";

function setupLifecycle() {
	const boot = () => {
		Lifecycle.init();
	};

	if (document.readyState === "loading") {
		document.addEventListener("DOMContentLoaded", boot, { once: true });
	} else {
		boot();
	}
}

function setupEscapeKeyListeners() {
	document.addEventListener("keydown", (event) => {
		if (event.key !== "Escape") return;

		closeSidebar();

		const toggles = closeAllUserMenus();
		const lastToggle = toggles[toggles.length - 1];
		if (lastToggle) lastToggle.focus();
	});
}

bootFeatures();
setupLifecycle();
setupEscapeKeyListeners();
