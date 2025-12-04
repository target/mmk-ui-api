/**
 * Toast Notification Component
 *
 * Provides accessible toast notifications with ARIA live regions.
 * Supports multiple types (success, error, warning, info) with auto-dismiss,
 * close button, and icon support via Lucide.
 *
 * @example
 * // Programmatic usage
 * showToast('Operation successful', 'success');
 * showToast('An error occurred', 'error', { duration: 10000 });
 *
 * @example
 * // Via custom event (HTMX integration)
 * document.dispatchEvent(new CustomEvent('showToast', {
 *   detail: { message: 'Saved!', type: 'success' }
 * }));
 */

import { h, qs } from "../core/dom.js";

// Constants
const TOAST_DURATION = 5000; // 5 seconds
const TOAST_ICONS = {
	success: "check-circle",
	error: "x-circle",
	warning: "alert-triangle",
	info: "info",
};
const VALID_TOAST_TYPES = ["success", "error", "warning", "info"];

/**
 * Create icon element using Lucide data-lucide attribute pattern
 * @param {string} iconName - Lucide icon name
 * @param {string} className - CSS class name
 * @returns {HTMLElement} Icon element
 */
function createIconElement(iconName, className) {
	return h("i", {
		"data-lucide": iconName,
		className: className,
		"aria-hidden": "true",
	});
}

/**
 * Create close button with icon
 * @param {Function} onClose - Close handler
 * @returns {HTMLElement} Close button element
 */
function createCloseButton(onClose) {
	const closeBtn = h("button", {
		type: "button",
		className: "toast-close",
		"aria-label": "Close notification",
		onClick: onClose,
	});
	// Add close icon using Lucide data-lucide pattern
	const icon = h("i", {
		"data-lucide": "x",
		className: "w-4 h-4",
		"aria-hidden": "true",
	});
	closeBtn.appendChild(icon);
	return closeBtn;
}

/**
 * Show a toast notification
 * @param {string} message - Toast message
 * @param {string} [type='info'] - Toast type (success, error, warning, info)
 * @param {Object} [options] - Additional options
 * @param {number} [options.duration=5000] - Auto-dismiss duration in ms (0 = no auto-dismiss)
 * @returns {HTMLElement|null} The created toast element
 */
export function showToast(message, type = "info", options = {}) {
	const container = qs("#toast-container");
	if (!container) return null;

	// Validate and coerce type to allowed set
	const safeType = VALID_TOAST_TYPES.includes(type) ? type : "info";
	const iconName = TOAST_ICONS[safeType] || "info";
	const duration = options.duration !== undefined ? options.duration : TOAST_DURATION;

	// Create toast element using DOM utilities
	const toast = h(
		"div",
		{
			className: `toast toast--${safeType}`,
			role: "alert",
			"aria-live": "polite",
		},
		[
			createIconElement(iconName, "toast-icon"),
			h("div", { className: "toast-content", text: message }),
			createCloseButton(() => removeToast(toast)),
		],
	);

	container.appendChild(toast);

	// Initialize Lucide icons for the toast
	try {
		if (window.lucide?.createIcons) {
			window.lucide.createIcons({ icons: window.lucide.icons });
		}
	} catch (_) {
		/* noop */
	}

	// Auto-remove after duration (if duration > 0)
	if (duration > 0) {
		const timeoutId = setTimeout(() => removeToast(toast), duration);
		toast.dataset.timeoutId = String(timeoutId);
	}

	return toast;
}

/**
 * Remove a toast notification
 * @param {HTMLElement} toast - Toast element to remove
 */
export function removeToast(toast) {
	if (!toast?.parentNode) return;

	// Guard against double-invocation
	if (toast.dataset.removing) return;
	toast.dataset.removing = "1";

	// Clear timeout if exists
	if (toast.dataset.timeoutId) {
		clearTimeout(Number(toast.dataset.timeoutId));
		delete toast.dataset.timeoutId; // Clean up stale data attribute
	}

	// Add removing class for animation
	toast.classList.add("toast--removing");

	// Listen for transition end with fallback timeout
	const onEnd = (e) => {
		if (e.target !== toast) return;
		toast.removeEventListener("transitionend", onEnd);
		if (toast.parentNode) {
			toast.parentNode.removeChild(toast);
		}
	};

	toast.addEventListener("transitionend", onEnd);

	// Fallback timeout in case transition doesn't fire
	setTimeout(() => onEnd({ target: toast }), 500);
}

// ============================================================================
// Global API & Event Listeners
// ============================================================================

// Only initialize in browser environment (not during tests/SSR)
if (typeof window !== "undefined") {
	// Expose globally for manual use with full options support
	window.showToast = (message, type, options) => showToast(message, type, options);

	// Guard against duplicate event listener registration
	if (!window.__toastEventListenerRegistered) {
		window.__toastEventListenerRegistered = true;

		// Listen for custom showToast events (HTMX integration)
		document.addEventListener("showToast", (evt) => {
			try {
				const detail = evt.detail;
				const message = detail?.message || detail?.value?.message || "Action completed";
				const type = detail?.type || detail?.value?.type || "info";
				const duration = detail?.duration || detail?.value?.duration;
				showToast(message, type, { duration });
			} catch (_) {
				/* noop */
			}
		});
	}
}
