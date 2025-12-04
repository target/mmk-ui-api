/**
 * Tests for Modal Component
 */

import { describe, it, expect, beforeEach, afterEach } from "bun:test";
import { Modal } from "./modal.js";

describe("Modal Component", () => {
	let container;

	beforeEach(() => {
		// Create a container for testing
		container = document.createElement("div");
		container.id = "test-container";
		document.body.appendChild(container);
	});

	afterEach(() => {
		// Clean up
		if (container?.parentNode) {
			container.remove();
		}
		// Remove any lingering modals
		const modals = document.querySelectorAll(".modal-overlay");
		for (const m of modals) {
			m.remove();
		}
		// Restore body overflow
		document.body.style.overflow = "";
	});

	describe("constructor", () => {
		it("should create a modal instance", () => {
			const content = document.createElement("div");
			content.textContent = "Modal content";
			const modal = new Modal(content);

			expect(modal).toBeDefined();
			expect(modal.content).toBe(content);
			expect(modal.isOpen).toBe(false);
		});

		it("should accept options", () => {
			const content = document.createElement("div");
			const onShow = () => {};
			const onHide = () => {};

			const modal = new Modal(content, {
				id: "test-modal",
				closeOnBackdrop: false,
				closeOnEscape: false,
				onShow,
				onHide,
			});

			expect(modal.options.id).toBe("test-modal");
			expect(modal.options.closeOnBackdrop).toBe(false);
			expect(modal.options.closeOnEscape).toBe(false);
			expect(modal.options.onShow).toBe(onShow);
			expect(modal.options.onHide).toBe(onHide);
		});
	});

	describe("show()", () => {
		it("should show the modal", () => {
			const content = document.createElement("div");
			content.textContent = "Modal content";
			const modal = new Modal(content);

			modal.show();

			expect(modal.isOpen).toBe(true);
			expect(document.querySelector(".modal-overlay")).toBeDefined();
			expect(document.body.style.overflow).toBe("hidden");
		});

		it("should call onShow callback", () => {
			const content = document.createElement("div");
			let called = false;
			const modal = new Modal(content, {
				onShow: () => {
					called = true;
				},
			});

			modal.show();

			expect(called).toBe(true);
		});

		it("should not show if already open", () => {
			const content = document.createElement("div");
			const modal = new Modal(content);

			modal.show();
			const overlay1 = document.querySelector(".modal-overlay");

			modal.show(); // Try to show again
			const overlays = document.querySelectorAll(".modal-overlay");

			expect(overlays.length).toBe(1);
			expect(overlay1).toBe(overlays[0]);
		});

		it("should set ARIA attributes with aria-label when no title exists", () => {
			const content = document.createElement("div");
			const modal = new Modal(content, { id: "test-modal", ariaLabel: "Test dialog" });

			modal.show();

			const overlay = document.querySelector(".modal-overlay");
			expect(overlay.getAttribute("role")).toBe("dialog");
			expect(overlay.getAttribute("aria-modal")).toBe("true");
			expect(overlay.getAttribute("aria-label")).toBe("Test dialog");
			expect(overlay.getAttribute("aria-labelledby")).toBeNull();
		});

		it("should set ARIA attributes with aria-labelledby when title exists", () => {
			const content = document.createElement("div");
			const titleEl = document.createElement("h3");
			titleEl.id = "test-modal-title";
			titleEl.textContent = "Test Title";
			content.appendChild(titleEl);

			const modal = new Modal(content, { id: "test-modal" });
			modal.show();

			const overlay = document.querySelector(".modal-overlay");
			expect(overlay.getAttribute("role")).toBe("dialog");
			expect(overlay.getAttribute("aria-modal")).toBe("true");
			expect(overlay.getAttribute("aria-labelledby")).toBe("test-modal-title");
			expect(overlay.getAttribute("aria-label")).toBeNull();

			modal.hide();
		});
	});

	describe("hide()", () => {
		it("should hide the modal", () => {
			const content = document.createElement("div");
			const modal = new Modal(content);

			modal.show();
			expect(modal.isOpen).toBe(true);

			modal.hide();
			expect(modal.isOpen).toBe(false);
			expect(document.querySelector(".modal-overlay")).toBeNull();
		});

		it("should restore body overflow", () => {
			document.body.style.overflow = "auto";
			const content = document.createElement("div");
			const modal = new Modal(content);

			modal.show();
			expect(document.body.style.overflow).toBe("hidden");

			modal.hide();
			expect(document.body.style.overflow).toBe("auto");
		});

		it("should call onHide callback", () => {
			const content = document.createElement("div");
			let called = false;
			const modal = new Modal(content, {
				onHide: () => {
					called = true;
				},
			});

			modal.show();
			modal.hide();

			expect(called).toBe(true);
		});

		it("should not error if already hidden", () => {
			const content = document.createElement("div");
			const modal = new Modal(content);

			expect(() => modal.hide()).not.toThrow();
		});
	});

	describe("keyboard support", () => {
		it("should close on Escape key", () => {
			const content = document.createElement("div");
			const modal = new Modal(content);

			modal.show();
			expect(modal.isOpen).toBe(true);

			// Simulate Escape key
			const event = new KeyboardEvent("keydown", { key: "Escape" });
			document.dispatchEvent(event);

			expect(modal.isOpen).toBe(false);
		});

		it("should not close on Escape if closeOnEscape is false", () => {
			const content = document.createElement("div");
			const modal = new Modal(content, { closeOnEscape: false });

			modal.show();
			expect(modal.isOpen).toBe(true);

			// Simulate Escape key
			const event = new KeyboardEvent("keydown", { key: "Escape" });
			document.dispatchEvent(event);

			expect(modal.isOpen).toBe(true);
			modal.hide(); // Clean up
		});
	});

	describe("backdrop click", () => {
		it("should close on backdrop click", () => {
			const content = document.createElement("div");
			const modal = new Modal(content);

			modal.show();
			const overlay = document.querySelector(".modal-overlay");

			// Simulate backdrop click (target === currentTarget)
			const event = new MouseEvent("click", { bubbles: true });
			Object.defineProperty(event, "target", { value: overlay, enumerable: true });
			Object.defineProperty(event, "currentTarget", { value: overlay, enumerable: true });
			overlay.dispatchEvent(event);

			expect(modal.isOpen).toBe(false);
		});

		it("should not close on content click", () => {
			const content = document.createElement("div");
			const modal = new Modal(content);

			modal.show();
			const overlay = document.querySelector(".modal-overlay");
			const modalContent = overlay.querySelector(".modal-content");

			// Simulate content click (target !== currentTarget)
			const event = new MouseEvent("click", { bubbles: true });
			Object.defineProperty(event, "target", { value: modalContent, enumerable: true });
			Object.defineProperty(event, "currentTarget", { value: overlay, enumerable: true });
			overlay.dispatchEvent(event);

			expect(modal.isOpen).toBe(true);
			modal.hide(); // Clean up
		});

		it("should not close on backdrop click if closeOnBackdrop is false", () => {
			const content = document.createElement("div");
			const modal = new Modal(content, { closeOnBackdrop: false });

			modal.show();
			const overlay = document.querySelector(".modal-overlay");

			// Simulate backdrop click
			const event = new MouseEvent("click", { bubbles: true });
			Object.defineProperty(event, "target", { value: overlay, enumerable: true });
			Object.defineProperty(event, "currentTarget", { value: overlay, enumerable: true });
			overlay.dispatchEvent(event);

			expect(modal.isOpen).toBe(true);
			modal.hide(); // Clean up
		});
	});

	describe("Modal.show() static method", () => {
		it("should create and show a modal", () => {
			const modal = Modal.show({
				title: "Test Modal",
				content: "Test content",
			});

			expect(modal).toBeDefined();
			expect(modal.isOpen).toBe(true);
			expect(document.querySelector(".modal-overlay")).toBeDefined();

			modal.hide(); // Clean up
		});

		it("should support HTML content", () => {
			const modal = Modal.show({
				title: "Test Modal",
				content: "<p>HTML content</p>",
			});

			const overlay = document.querySelector(".modal-overlay");
			expect(overlay.innerHTML).toContain("HTML content");

			modal.hide(); // Clean up
		});

		it("should support element content", () => {
			const contentEl = document.createElement("div");
			contentEl.textContent = "Element content";

			const modal = Modal.show({
				title: "Test Modal",
				content: contentEl,
			});

			const overlay = document.querySelector(".modal-overlay");
			expect(overlay.textContent).toContain("Element content");

			modal.hide(); // Clean up
		});

		it("should support buttons with type='button'", () => {
			let clicked = false;
			const modal = Modal.show({
				title: "Test Modal",
				content: "Test content",
				buttons: [
					{
						text: "Click Me",
						className: "btn btn-primary",
						onClick: () => {
							clicked = true;
						},
					},
				],
			});

			const button = document.querySelector(".modal-buttons button");
			expect(button).toBeDefined();
			expect(button.textContent).toBe("Click Me");
			expect(button.getAttribute("type")).toBe("button");

			button.click();
			expect(clicked).toBe(true);

			modal.hide(); // Clean up
		});
	});

	describe("declarative modal restoration", () => {
		it("should restore content to original location after hide", () => {
			// Create a content element in the DOM
			const originalParent = document.createElement("div");
			originalParent.id = "original-parent";
			document.body.appendChild(originalParent);

			const content = document.createElement("div");
			content.id = "modal-content";
			content.textContent = "Modal content";
			originalParent.appendChild(content);

			// Create and show modal
			const modal = new Modal(content);
			modal.show();

			// Content should be moved to modal
			expect(content.parentNode?.className).toBe("modal-content");
			expect(originalParent.contains(content)).toBe(false);

			// Hide modal
			modal.hide();

			// Content should be restored to original location
			expect(originalParent.contains(content)).toBe(true);
			expect(content.parentNode).toBe(originalParent);

			// Clean up
			originalParent.remove();
		});
	});

	describe("focus management", () => {
		it("should focus modal content when no focusable elements exist", () => {
			const content = document.createElement("div");
			content.textContent = "Read-only content";

			const modal = new Modal(content);
			modal.show();

			const modalContent = document.querySelector(".modal-content");
			expect(document.activeElement).toBe(modalContent);

			modal.hide();
		});

		it("should focus first focusable element when available", () => {
			const content = document.createElement("div");
			const button = document.createElement("button");
			button.textContent = "Click me";
			content.appendChild(button);

			const modal = new Modal(content);
			modal.show();

			expect(document.activeElement).toBe(button);

			modal.hide();
		});
	});
});
