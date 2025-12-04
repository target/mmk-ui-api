/**
 * Navigation state feature module
 *
 * Sets active navigation links based on the current location and responds to
 * HTMX navigation events and custom nav activation signals.
 */
import { qsa } from "../core/dom.js";
import { on } from "../core/htmx-bridge.js";

let initialized = false;

const FALLBACK_NAV_SELECTOR = ".sidebar .nav-link";

/**
 * Resolve navigation link elements with configurable data attribute contracts.
 * - Explicitly mark anchors with `data-nav-link`.
 * - Wrap groups with `data-nav-root` and optionally `data-nav-link-selector`.
 * - Provide `data-nav-scope` selectors on any element (including <body>) to discover scoped roots.
 * Falls back to the legacy `.sidebar .nav-link` selector for compatibility.
 */
function getConfiguredNavLinks() {
	const seen = new Set();
	const links = [];

	const addLink = (link) => {
		if (link && !seen.has(link)) {
			seen.add(link);
			links.push(link);
		}
	};

	const collectLinks = (selector, root = document) => {
		if (!selector) return;
		qsa(selector, root).forEach(addLink);
	};

	const collectFromContainer = (container) => {
		if (!container) return;

		if (
			typeof container.matches === "function" &&
			container.matches("[data-nav-link], .nav-link, a")
		) {
			addLink(container);
		}

		const configuredSelectors = (container.dataset.navLinkSelector || "").split(",");
		configuredSelectors
			.map((value) => value.trim())
			.filter(Boolean)
			.forEach((selector) => {
				collectLinks(selector, container);
			});

		collectLinks("[data-nav-link]", container);
		collectLinks(".nav-link", container);
	};

	qsa("[data-nav-link]").forEach(addLink);

	const navRoots = qsa("[data-nav-root]");
	if (navRoots.length) {
		navRoots.forEach(collectFromContainer);
	}

	const scopedConfigs = qsa("[data-nav-scope]");
	if (scopedConfigs.length) {
		scopedConfigs.forEach((configHost) => {
			const scope = configHost.dataset.navScope?.trim();
			if (!scope) return;
			qsa(scope, configHost).forEach(collectFromContainer);
		});
	}

	if (links.length === 0) {
		const globalScope =
			document.body?.dataset?.navScope?.trim() ||
			document.documentElement?.dataset?.navScope?.trim();
		if (globalScope) {
			qsa(globalScope).forEach(collectFromContainer);
		}
	}

	if (links.length === 0) {
		collectLinks(FALLBACK_NAV_SELECTOR);
	}

	return links;
}

function setActiveNav(pathname) {
	try {
		const links = getConfiguredNavLinks();
		links.forEach((anchor) => {
			let href = anchor.getAttribute("href") || "";
			if (!href) return;

			try {
				href = new URL(href, location.origin).pathname;
			} catch (_) {
				/* noop */
			}

			let active = false;
			if (href === "/") {
				active = pathname === "/";
			} else {
				const isPrefix = pathname.startsWith(`${href}/`);
				active = pathname === href || isPrefix;
			}

			anchor.classList.toggle("is-active", !!active);
		});
	} catch (_) {
		/* noop */
	}
}

function updateNavFor(path) {
	let pathname = path;
	try {
		pathname = new URL(path, location.origin).pathname;
	} catch (_) {
		/* noop */
	}

	setActiveNav(pathname);
}

function registerEventHandlers() {
	const refreshFromLocation = () => {
		setActiveNav(location.pathname);
	};

	if (document.readyState === "loading") {
		document.addEventListener("DOMContentLoaded", refreshFromLocation, { once: true });
	} else {
		refreshFromLocation();
	}

	document.addEventListener("nav:activate", (event) => {
		const path = event?.detail?.path ?? location.pathname;
		updateNavFor(path);
	});

	on("htmx:afterSwap", refreshFromLocation);
	on("htmx:historyRestore", refreshFromLocation);
}

export function initNavigation() {
	if (initialized) return;
	initialized = true;
	registerEventHandlers();
}

export { setActiveNav };
