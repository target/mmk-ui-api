// Core UI Libraries vendor bundle
// Exports htmx and lucide for browser use (needed on every page)

// Import and re-export htmx
import htmx from "htmx.org";

// Import and re-export lucide
import { createIcons, icons } from "lucide";

// Attach to window for global access (matching CDN behavior)
declare global {
	interface Window {
		htmx: typeof htmx;
		lucide: {
			createIcons: typeof createIcons;
			icons: typeof icons;
		};
	}
}

// Make htmx available globally (matches unpkg behavior)
window.htmx = htmx;

// Configure htmx to include CSRF token in all requests
document.addEventListener("DOMContentLoaded", () => {
	// Get CSRF token from cookie
	const getCsrfToken = (): string => {
		const match = document.cookie.match(/csrf_token=([^;]+)/);
		return match ? match[1] : "";
	};

	// Add CSRF token to all htmx requests
	document.body.addEventListener("htmx:configRequest", (event: Event) => {
		const customEvent = event as CustomEvent<{
			headers: Record<string, string>;
		}>;
		const token = getCsrfToken();
		if (token) {
			customEvent.detail.headers["X-Csrf-Token"] = token;
		}
	});
});

// Make lucide available globally (matches unpkg behavior)
window.lucide = {
	createIcons,
	icons,
};
