/**
 * IOC Form Component
 * Handles single/bulk entry mode switching for IOC forms
 */

import { Events } from "../core/events.js";
import { on } from "../core/htmx-bridge.js";

/**
 * Get form elements for mode switching
 * @param {HTMLElement} container - Form container element
 * @returns {Object} Form elements
 */
function getFormElements(container) {
	return {
		modeButtons: container.querySelectorAll("[data-ioc-mode-btn]"),
		entryModeInput: container.querySelector("#entry_mode"),
		singleValueGroup: container.querySelector("#single-value-group"),
		bulkValuesGroup: container.querySelector("#bulk-values-group"),
		valueInput: container.querySelector("#value"),
		bulkValuesInput: container.querySelector("#bulk_values"),
		descriptionHelpSingle: container.querySelector("#description-help-single"),
		descriptionHelpBulk: container.querySelector("#description-help-bulk"),
		enabledHelpSingle: container.querySelector("#enabled-help-single"),
		enabledHelpBulk: container.querySelector("#enabled-help-bulk"),
		submitSingle: container.querySelector("#submit-single"),
		submitBulk: container.querySelector("#submit-bulk"),
	};
}

/**
 * Update button states for mode
 * @param {NodeList} buttons - Mode buttons
 * @param {string} mode - Active mode
 */
function updateButtonStates(buttons, mode) {
	buttons.forEach((btn) => {
		const isActive = btn.dataset.iocModeBtn === mode;
		btn.classList.toggle("active", isActive);
		btn.setAttribute("aria-checked", String(isActive));
	});
}

/**
 * Toggle element visibility
 * @param {HTMLElement|null} element - Element to toggle
 * @param {boolean} show - Whether to show the element
 */
function toggleElement(element, show) {
	if (element) {
		element.style.display = show ? "" : "none";
	}
}

/**
 * Configure single mode display
 * @param {Object} elements - Form elements
 */
function configureSingleMode(elements) {
	toggleElement(elements.singleValueGroup, true);
	toggleElement(elements.bulkValuesGroup, false);
	toggleElement(elements.descriptionHelpSingle, true);
	toggleElement(elements.descriptionHelpBulk, false);
	toggleElement(elements.enabledHelpSingle, true);
	toggleElement(elements.enabledHelpBulk, false);
	toggleElement(elements.submitSingle, true);
	toggleElement(elements.submitBulk, false);

	if (elements.valueInput) {
		elements.valueInput.required = true;
	}
	if (elements.bulkValuesInput) {
		elements.bulkValuesInput.required = false;
		elements.bulkValuesInput.value = "";
	}
}

/**
 * Configure bulk mode display
 * @param {Object} elements - Form elements
 */
function configureBulkMode(elements) {
	toggleElement(elements.singleValueGroup, false);
	toggleElement(elements.bulkValuesGroup, true);
	toggleElement(elements.descriptionHelpSingle, false);
	toggleElement(elements.descriptionHelpBulk, true);
	toggleElement(elements.enabledHelpSingle, false);
	toggleElement(elements.enabledHelpBulk, true);
	toggleElement(elements.submitSingle, false);
	toggleElement(elements.submitBulk, true);

	if (elements.valueInput) {
		elements.valueInput.required = false;
		elements.valueInput.value = "";
	}
	if (elements.bulkValuesInput) {
		elements.bulkValuesInput.required = true;
	}
}

/**
 * Switch between single and bulk entry modes
 * @param {string} mode - 'single' or 'bulk'
 * @param {HTMLElement} container - Form container element
 */
function switchMode(mode, container) {
	const elements = getFormElements(container);

	if (!(elements.modeButtons.length && elements.entryModeInput)) return;

	// Update button states
	updateButtonStates(elements.modeButtons, mode);

	// Update hidden input
	elements.entryModeInput.value = mode;

	// Configure display based on mode
	if (mode === "single") {
		configureSingleMode(elements);
	} else {
		configureBulkMode(elements);
	}
}

/**
 * Initialize IOC form mode switching
 */
function initIOCForm() {
	const container = document.querySelector("[data-ioc-form]");
	if (!container) return;

	// Initialize to single mode
	switchMode("single", container);
}

// Register event handler for mode buttons
Events.on("[data-ioc-mode-btn]", "click", (e, button) => {
	e.preventDefault();
	const mode = button.dataset.iocModeBtn;
	const container = button.closest("[data-ioc-form]");
	if (container && mode) {
		switchMode(mode, container);
	}
});

// Initialize on page load
if (typeof document !== "undefined") {
	document.addEventListener("DOMContentLoaded", initIOCForm);

	// Re-initialize after HTMX swaps
	on("htmx:afterSwap", (event) => {
		const container = event.target.querySelector("[data-ioc-form]");
		if (container) {
			initIOCForm();
		}
	});
}
