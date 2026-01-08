/**
 * Domain Truncate Component
 *
 * Provides interactive features for long domain names:
 * - Click-to-copy domain value with visual feedback
 * - Expand/collapse toggle for truncated domains
 * - Native title tooltip for full domain on hover
 *
 * IMPORTANT: Markup Considerations
 * This component works best when domain-text is NOT nested inside <a> tags.
 * If used within links (as in alert item rows), ensure:
 * 1. Parent links have hx-boost or HTMX event handlers (not standard click)
 * 2. Button events use stopPropagation() to prevent link navigation
 * 3. Consider moving buttons outside link structure for better semantics
 *
 * Usage:
 * Add data-domain-truncate attribute to domain elements in templates:
 * <span class="domain-text" data-domain-truncate="full.domain.name.here">
 *   truncated.domain...
 * </span>
 */

import { Lifecycle } from "../core/lifecycle.js";
import { showToast } from "./toast.js";

/**
 * Duration to show copy feedback before resetting button state (ms)
 */
const COPY_FEEDBACK_DURATION = 2000;

/**
 * Copy text to clipboard with fallback for older browsers
 * @param {string} text - Text to copy
 * @returns {Promise<boolean>} - Resolves to true if successful
 */
async function copyToClipboard(text) {
	// Try modern Clipboard API first
	if (navigator.clipboard?.writeText) {
		try {
			await navigator.clipboard.writeText(text);
			return true;
		} catch (err) {
			console.error("Clipboard API failed:", err);
			// Fall through to fallback
		}
	}

	// Fallback: use deprecated execCommand
	try {
		const textarea = document.createElement("textarea");
		textarea.value = text;
		textarea.style.position = "fixed";
		textarea.style.opacity = "0";
		document.body.appendChild(textarea);

		textarea.select();
		textarea.setSelectionRange(0, text.length);

		const success = document.execCommand("copy");
		document.body.removeChild(textarea);

		return success;
	} catch (err) {
		console.error("Fallback copy failed:", err);
		return false;
	}
}

/**
 * Initialize domain truncation features for an element
 * @param {HTMLElement} element - The domain text element
 */
function initDomainElement(element) {
	const fullDomain = element.getAttribute("data-domain-truncate");
	if (!fullDomain) return;

	// Add title attribute for native browser tooltip
	element.title = fullDomain;

	// Wrap in container if not already wrapped
	let wrapper = element.closest(".domain-wrapper");
	if (!wrapper) {
		wrapper = document.createElement("span");
		wrapper.className = "domain-wrapper";
		element.parentNode.insertBefore(wrapper, element);
		wrapper.appendChild(element);
	}

	// Mark parent with domain actions for CSS styling
	const parent = wrapper.parentElement;
	if (parent) {
		parent.classList.add("has-domain-actions");
	}

	// Add copy button if not exists
	if (!wrapper.querySelector(".btn-copy-domain")) {
		const copyBtn = createCopyButton(fullDomain);
		wrapper.appendChild(copyBtn);
	}

	// Check if domain is actually truncated on screen after layout
	requestAnimationFrame(() => {
		const needsExpand = element.scrollWidth > element.clientWidth;
		if (needsExpand && !wrapper.querySelector(".btn-expand-domain")) {
			const expandBtn = createExpandButton(element);
			wrapper.appendChild(expandBtn);
		}
	});
}

/**
 * Create copy button with Lucide icon
 * @param {string} textToCopy - The full domain text to copy
 * @returns {HTMLButtonElement}
 */
function createCopyButton(textToCopy) {
	const button = document.createElement("button");
	button.type = "button";
	button.className = "btn-copy-domain";
	button.setAttribute("aria-label", "Copy domain to clipboard");
	button.setAttribute("data-domain-value", textToCopy);
	button.title = "Copy domain";

	const icon = document.createElement("i");
	icon.setAttribute("data-lucide", "copy");
	button.appendChild(icon);

	return button;
}

/**
 * Create expand/collapse toggle button
 * @param {HTMLElement} domainElement - The domain text element to expand
 * @returns {HTMLButtonElement}
 */
function createExpandButton(domainElement) {
	const button = document.createElement("button");
	button.type = "button";
	button.className = "btn-expand-domain";
	button.setAttribute("aria-label", "Expand domain");
	button.setAttribute("aria-expanded", "false");
	button.setAttribute("data-domain-element", "");
	button.title = "Expand domain";

	const icon = document.createElement("i");
	icon.setAttribute("data-lucide", "chevron-down");
	button.appendChild(icon);

	// Store reference to domain element for event delegation
	button.__domainElement = domainElement;

	return button;
}

/**
 * Show visual feedback for successful copy
 * @param {HTMLButtonElement} button
 */
function showCopySuccess(button) {
	button.classList.add("is-copied");
	button.setAttribute("aria-label", "Copied!");
	button.title = "Copied!";

	// Show toast notification
	const domainValue = button.getAttribute("data-domain-value");
	showToast(`${domainValue} copied to clipboard`, "success", { duration: COPY_FEEDBACK_DURATION });

	setTimeout(() => {
		button.classList.remove("is-copied");
		button.setAttribute("aria-label", "Copy domain to clipboard");
		button.title = "Copy domain";
	}, COPY_FEEDBACK_DURATION);
}

/**
 * Show visual feedback for copy error
 * @param {HTMLButtonElement} button
 */
function showCopyError(button) {
	button.classList.add("is-error");
	showToast("Failed to copy domain", "error", { duration: COPY_FEEDBACK_DURATION });

	setTimeout(() => {
		button.classList.remove("is-error");
	}, COPY_FEEDBACK_DURATION);
}

/**
 * Handle delegated click events for copy and expand buttons
 * Uses stopPropagation to prevent parent link navigation when nested
 * @param {Event} e
 */
function handleDomainButtonClick(e) {
	const button = e.target.closest(".btn-copy-domain, .btn-expand-domain");
	if (!button) return;

	// CRITICAL: Stop propagation to prevent parent link/row navigation
	// This is especially important when buttons are nested inside <a> or hx-boost elements
	e.preventDefault();
	e.stopPropagation();

	if (button.classList.contains("btn-copy-domain")) {
		const textToCopy = button.getAttribute("data-domain-value");
		if (textToCopy) {
			copyToClipboard(textToCopy).then((success) => {
				if (success) {
					showCopySuccess(button);
				} else {
					showCopyError(button);
				}
			});
		}
	} else if (button.classList.contains("btn-expand-domain")) {
		const domainElement = button.__domainElement;
		if (domainElement) {
			const isExpanded = domainElement.classList.toggle("is-expanded");
			button.classList.toggle("is-expanded", isExpanded);
			button.setAttribute("aria-expanded", isExpanded.toString());
			button.setAttribute("aria-label", isExpanded ? "Collapse domain" : "Expand domain");
			button.title = isExpanded ? "Collapse domain" : "Expand domain";
		}
	}
}

/**
 * Initialize all domain elements in the document or container
 * @param {HTMLElement} container - Container to search for domain elements
 */
function initDomainTruncate(container = document) {
	const elements = container.querySelectorAll("[data-domain-truncate]");
	elements.forEach(initDomainElement);

	// Re-render Lucide icons after adding new buttons - do this AFTER a short delay
	// to allow DOM to settle, preventing Lucide from stripping dynamically added elements
	requestAnimationFrame(() => {
		try {
			if (window.lucide?.createIcons) {
				window.lucide.createIcons({ icons: window.lucide.icons });
			}
		} catch (_) {
			/* noop */
		}
	});
}

// Add single delegated listener for all domain buttons
document.addEventListener("click", handleDomainButtonClick, true);

// Direct HTMX afterSwap handler - ensure domain truncate init runs before Lucide icons render
if (typeof window !== "undefined" && window.htmx) {
	document.addEventListener("htmx:afterSettle", (event) => {
		// Re-initialize domain truncate on the swapped element
		initDomainTruncate(event.detail.target);
	});
}

// Register with lifecycle system as fallback
Lifecycle.register(
	"domain-truncate",
	(element) => {
		initDomainTruncate(element);
	},
	() => {
		// No cleanup needed - buttons are removed with DOM
	},
);

// Initial load
document.addEventListener("DOMContentLoaded", () => {
	initDomainTruncate();
});
