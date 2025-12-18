/**
 * Modal Component
 *
 * Reusable modal dialog component with accessibility features.
 * Supports both programmatic and declarative usage.
 *
 * Features:
 * - Keyboard support (Escape to close)
 * - Focus management (trap focus in modal)
 * - Backdrop click to close
 * - Body scroll prevention
 * - ARIA attributes for accessibility
 * - Custom content via options or data attributes
 *
 * @example Programmatic usage
 * ```js
 * import { Modal } from './components/modal.js';
 *
 * Modal.show({
 *   title: 'Confirm Action',
 *   content: 'Are you sure?',
 *   buttons: [
 *     { text: 'Cancel', className: 'btn btn-secondary', onClick: (modal) => modal.hide() },
 *     { text: 'Confirm', className: 'btn btn-primary', onClick: (modal) => { doAction(); modal.hide(); } }
 *   ]
 * });
 * ```
 *
 * @example Declarative usage
 * ```html
 * <button data-modal-trigger="my-modal">Open Modal</button>
 * <div id="my-modal" data-component="modal" style="display:none;">
 *   <h3>Modal Title</h3>
 *   <p>Modal content...</p>
 *   <button data-modal-close>Close</button>
 * </div>
 * ```
 */

import { h, qs } from "../core/dom.js";
import { Events } from "../core/events.js";
import { Lifecycle } from "../core/lifecycle.js";

/**
 * Modal class for creating and managing modal dialogs
 */
export class Modal {
	/**
	 * Create a new Modal instance
	 * @param {HTMLElement} element - The modal content element
	 * @param {Object} options - Modal options
	 * @param {string} options.id - Modal ID
	 * @param {string} options.ariaLabel - ARIA label for the modal (used if no title element exists)
	 * @param {boolean} options.closeOnBackdrop - Close on backdrop click (default: true)
	 * @param {boolean} options.closeOnEscape - Close on Escape key (default: true)
	 * @param {Function} options.onShow - Callback when modal is shown
	 * @param {Function} options.onHide - Callback when modal is hidden
	 */
	constructor(element, options = {}) {
		this.content = element;
		this.options = {
			id: options.id || element.id || `modal-${Date.now()}`,
			ariaLabel: options.ariaLabel || "Modal dialog",
			closeOnBackdrop: options.closeOnBackdrop !== false,
			closeOnEscape: options.closeOnEscape !== false,
			onShow: options.onShow || null,
			onHide: options.onHide || null,
		};

		this.overlay = null;
		this.isOpen = false;
		this.previousActiveElement = null;
		this.previousBodyOverflow = null;

		// Store original location for declarative modals
		this.originalParent = element.parentNode;
		this.originalNextSibling = element.nextSibling;

		// Bind methods
		this.handleEscape = this.handleEscape.bind(this);
		this.handleBackdropClick = this.handleBackdropClick.bind(this);
		this.handleFocusTrap = this.handleFocusTrap.bind(this);
	}

	/**
	 * Show the modal
	 */
	show() {
		if (this.isOpen) return;

		// Store current active element to restore focus later
		this.previousActiveElement = document.activeElement;

		// Prevent body scroll
		this.previousBodyOverflow = document.body.style.overflow;
		document.body.style.overflow = "hidden";

		// Check if title element exists for ARIA labeling
		const titleId = `${this.options.id}-title`;
		const hasTitleElement =
			document.getElementById(titleId) || this.content.querySelector(`#${titleId}`);

		// Create overlay
		const overlayAttrs = {
			id: `${this.options.id}-overlay`,
			className: "modal-overlay",
			role: "dialog",
			"aria-modal": "true",
			style: {
				position: "fixed",
				top: "0",
				left: "0",
				width: "100%",
				height: "100%",
				background: "rgba(0,0,0,0.8)",
				zIndex: "1000",
				display: "flex",
				alignItems: "center",
				justifyContent: "center",
				cursor: "pointer",
			},
		};

		// Set ARIA label appropriately
		if (hasTitleElement) {
			overlayAttrs["aria-labelledby"] = titleId;
		} else {
			overlayAttrs["aria-label"] = this.options.ariaLabel;
		}

		this.overlay = h("div", overlayAttrs, [
			h(
				"div",
				{
					className: "modal-content",
					tabIndex: -1, // Make focusable as fallback
					style: {
						maxWidth: "90%",
						maxHeight: "90%",
						background: "white",
						borderRadius: "8px",
						padding: "20px",
						cursor: "default",
						overflow: "auto",
					},
				},
				[this.content],
			),
		]);

		// Add event listeners
		if (this.options.closeOnBackdrop) {
			this.overlay.addEventListener("click", this.handleBackdropClick);
		}
		if (this.options.closeOnEscape) {
			document.addEventListener("keydown", this.handleEscape);
		}

		// Focus trap
		document.addEventListener("keydown", this.handleFocusTrap);

		// Append to body
		document.body.appendChild(this.overlay);

		// Focus first focusable element
		this.focusFirstElement();

		this.isOpen = true;

		// Call onShow callback
		if (this.options.onShow) {
			this.options.onShow(this);
		}
	}

	/**
	 * Hide the modal
	 */
	hide() {
		if (!this.isOpen) return;

		// Remove event listeners
		if (this.options.closeOnBackdrop) {
			this.overlay.removeEventListener("click", this.handleBackdropClick);
		}
		if (this.options.closeOnEscape) {
			document.removeEventListener("keydown", this.handleEscape);
		}
		document.removeEventListener("keydown", this.handleFocusTrap);

		// Restore declarative modal content to original location
		if (this.originalParent) {
			this.originalParent.insertBefore(this.content, this.originalNextSibling || null);
		}

		// Remove overlay
		if (this.overlay?.parentNode) {
			this.overlay.remove();
		}

		// Restore body scroll
		if (this.previousBodyOverflow !== null) {
			document.body.style.overflow = this.previousBodyOverflow;
		}

		// Restore focus
		if (this.previousActiveElement?.focus) {
			this.previousActiveElement.focus();
		}

		this.isOpen = false;
		this.overlay = null;

		// Call onHide callback
		if (this.options.onHide) {
			this.options.onHide(this);
		}
	}

	/**
	 * Handle Escape key press
	 * @param {KeyboardEvent} e
	 */
	handleEscape(e) {
		if (e.key === "Escape") {
			this.hide();
		}
	}

	/**
	 * Handle backdrop click (close when clicking outside content)
	 * @param {MouseEvent} e
	 */
	handleBackdropClick(e) {
		// Only close if clicking the overlay itself, not the content
		if (e.target === e.currentTarget) {
			this.hide();
		}
	}

	/**
	 * Handle focus trap (keep focus within modal)
	 * @param {KeyboardEvent} e
	 */
	handleFocusTrap(e) {
		if (e.key !== "Tab" || !this.overlay) return;

		const focusableElements = this.getFocusableElements();
		if (focusableElements.length === 0) return;

		const firstElement = focusableElements[0];
		const lastElement = focusableElements[focusableElements.length - 1];

		if (e.shiftKey) {
			// Shift + Tab
			if (document.activeElement === firstElement) {
				e.preventDefault();
				lastElement.focus();
			}
		} else if (document.activeElement === lastElement) {
			// Tab
			e.preventDefault();
			firstElement.focus();
		}
	}

	/**
	 * Get all focusable elements within the modal
	 * @returns {HTMLElement[]}
	 */
	getFocusableElements() {
		if (!this.overlay) return [];

		const selector = 'button, [href], input, select, textarea, [tabindex]:not([tabindex="-1"])';
		return Array.from(this.overlay.querySelectorAll(selector)).filter(
			(el) => !el.disabled && el.offsetParent !== null,
		);
	}

	/**
	 * Focus the first focusable element in the modal
	 */
	focusFirstElement() {
		const focusableElements = this.getFocusableElements();
		if (focusableElements.length > 0) {
			focusableElements[0].focus();
		} else {
			// Fallback: focus the modal content container itself
			const modalContent = this.overlay?.querySelector(".modal-content");
			if (modalContent) {
				modalContent.focus();
			}
		}
	}

	/**
	 * Static method to show a modal with custom content
	 * @param {Object} options - Modal options
	 * @param {string} options.title - Modal title
	 * @param {string|HTMLElement} options.content - Modal content
	 * @param {Array} options.buttons - Array of button configs
	 * @param {string} options.id - Modal ID
	 * @returns {Modal}
	 */
	// biome-ignore lint/suspicious/useAdjacentOverloadSignatures: Static method is different from instance method
	static show(options = {}) {
		const { title, content, buttons = [], id = `modal-${Date.now()}` } = options;

		// Declare modal variable first to avoid use-before-define in button onClick
		let modal;

		// Create content element
		const contentEl = h(
			"div",
			{ id, className: "modal-dynamic" },
			[
				title
					? h("h3", {
							id: `${id}-title`,
							text: title,
							style: { margin: "0 0 10px 0" },
						})
					: null,
				typeof content === "string" ? h("div", { innerHTML: content }) : content,
				buttons.length > 0
					? h(
							"div",
							{
								className: "modal-buttons",
								style: {
									marginTop: "20px",
									display: "flex",
									gap: "10px",
									justifyContent: "flex-end",
								},
							},
							buttons.map((btn) =>
								h("button", {
									type: "button",
									text: btn.text || "Button",
									className: btn.className || "btn btn-secondary",
									onClick: () => {
										if (btn.onClick) btn.onClick(modal);
									},
								}),
							),
						)
					: null,
			].filter(Boolean),
		);

		modal = new Modal(contentEl, options);
		modal.show();
		return modal;
	}
}

// Register with Lifecycle system for declarative usage
Lifecycle.register(
	"modal",
	(element) => {
		// Skip if already initialized
		if (element._modalInstance) return;

		const modal = new Modal(element);
		element._modalInstance = modal;

		// Listen for close button clicks within the modal
		const closeButtons = element.querySelectorAll("[data-modal-close]");
		closeButtons.forEach((btn) => {
			btn.addEventListener("click", () => modal.hide());
		});
	},
	(element) => {
		// Cleanup
		if (element._modalInstance) {
			element._modalInstance.hide();
			element._modalInstance = undefined;
		}
	},
);

// Register event delegation for modal triggers (only in browser environment)
if (typeof document !== "undefined") {
	Events.on("[data-modal-trigger]", "click", (_e, trigger) => {
		const targetId = trigger.dataset.modalTrigger;
		const targetElement = qs(`#${targetId}`);

		if (targetElement?._modalInstance) {
			targetElement._modalInstance.show();
		}
	});
}

// Export for use in other modules
export default Modal;
