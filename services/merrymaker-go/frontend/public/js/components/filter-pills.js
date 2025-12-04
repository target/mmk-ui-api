/**
 * Filter Pills Component
 *
 * Reusable filter pill group with keyboard navigation and ARIA support.
 * Automatically updates hidden input and submits form via HTMX.
 *
 * @module components/filter-pills
 * @example
 * <div data-component="filter-pills" data-filter-name="status">
 *   <button class="filter-pill" data-filter-value="">All</button>
 *   <button class="filter-pill" data-filter-value="active">Active</button>
 *   <button class="filter-pill" data-filter-value="inactive">Inactive</button>
 * </div>
 * <input type="hidden" name="status" class="status-filter-input" />
 */

import { Lifecycle } from "../core/lifecycle.js";

/**
 * FilterPills component class
 */
export class FilterPills {
	/**
	 * Create a FilterPills instance
	 *
	 * @param {Element} element - Container element with data-component="filter-pills"
	 */
	constructor(element) {
		this.element = element;
		this.filterName = element.getAttribute("data-filter-name") || "filter";
		this.form = element.closest("form");
		this.hiddenInput = null;

		this.init();
	}

	/**
	 * Initialize the component
	 */
	init() {
		// Find or create hidden input
		this.findOrCreateHiddenInput();

		// Bind event handlers
		this.bindEvents();

		// Set initial active state based on hidden input value
		this.updatePillsFromInput();
	}

	/**
	 * Find or create the hidden input for this filter
	 */
	findOrCreateHiddenInput() {
		if (!this.form) {
			console.warn("FilterPills: No form found for filter pills");
			return;
		}

		// Try to find existing hidden input by name or class
		const inputClass = `${this.filterName}-filter-input`;
		this.hiddenInput =
			this.form.querySelector(`.${inputClass}`) ||
			this.form.querySelector(`input[name="${this.filterName}"]`);

		// Create if not found
		if (!this.hiddenInput) {
			this.hiddenInput = document.createElement("input");
			this.hiddenInput.type = "hidden";
			this.hiddenInput.name = this.filterName;
			this.hiddenInput.className = inputClass;
			this.form.appendChild(this.hiddenInput);
		}
	}

	/**
	 * Bind event handlers
	 */
	bindEvents() {
		// Click handler for pills
		this.element.addEventListener("click", (e) => {
			const pill = e.target.closest(".filter-pill");
			if (pill) {
				e.preventDefault();
				this.handlePillClick(pill);
			}
		});

		// Keyboard navigation
		this.element.addEventListener("keydown", (e) => {
			const pill = e.target.closest(".filter-pill");
			if (!pill) return;

			if (e.key === "Enter" || e.key === " ") {
				e.preventDefault();
				pill.click();
			} else if (e.key === "ArrowLeft" || e.key === "ArrowRight") {
				e.preventDefault();
				this.handleArrowKey(pill, e.key);
			}
		});
	}

	/**
	 * Handle pill click
	 *
	 * @param {Element} pill - Clicked pill element
	 */
	handlePillClick(pill) {
		const value = pill.getAttribute("data-filter-value") || "";
		this.setValue(value);
		this.submitForm();
	}

	/**
	 * Handle arrow key navigation
	 *
	 * @param {Element} currentPill - Currently focused pill
	 * @param {string} key - Arrow key pressed
	 */
	handleArrowKey(currentPill, key) {
		const pills = Array.from(this.element.querySelectorAll(".filter-pill"));
		const index = pills.indexOf(currentPill);
		const direction = key === "ArrowLeft" ? -1 : 1;
		const nextIndex = (index + direction + pills.length) % pills.length;
		pills[nextIndex].focus();
	}

	/**
	 * Set filter value and update UI
	 *
	 * @param {string} value - Filter value
	 */
	setValue(value) {
		if (this.hiddenInput) {
			this.hiddenInput.value = value;
		}
		this.updatePills(value);
	}

	/**
	 * Update pill active states and ARIA attributes
	 *
	 * @param {string} value - Active filter value
	 */
	updatePills(value) {
		const pills = this.element.querySelectorAll(".filter-pill");

		pills.forEach((pill) => {
			const pillValue = pill.getAttribute("data-filter-value") || "";
			const isActive = pillValue === value;

			pill.classList.toggle("active", isActive);
			pill.setAttribute("aria-pressed", isActive ? "true" : "false");
		});
	}

	/**
	 * Update pills based on current hidden input value
	 */
	updatePillsFromInput() {
		if (this.hiddenInput) {
			this.updatePills(this.hiddenInput.value);
		}
	}

	/**
	 * Submit the form via HTMX or native submit
	 */
	submitForm() {
		if (!this.form) return;

		// Try HTMX first
		if (window.htmx && typeof htmx.trigger === "function") {
			htmx.trigger(this.form, "submit");
		} else if (this.form.requestSubmit) {
			// Fallback to native submit with requestSubmit
			this.form.requestSubmit();
		} else {
			// Final fallback to submit
			this.form.submit();
		}
	}

	/**
	 * Cleanup when component is destroyed
	 */
	destroy() {
		// Event listeners are on the element itself, so they'll be cleaned up
		// when the element is removed from DOM
	}
}

/**
 * Register component with lifecycle system
 */
Lifecycle.register(
	"filter-pills",
	(element) => {
		const instance = new FilterPills(element);
		element.__filterPills = instance;
	},
	(element) => {
		element.__filterPills?.destroy();
	},
);

export default FilterPills;
