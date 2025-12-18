/**
 * Tests for Toast Component
 */

import { afterEach, beforeEach, describe, expect, it } from "bun:test";
import { removeToast, showToast } from "./toast.js";

describe("Toast Component", () => {
	let container;

	beforeEach(() => {
		// Clean up any existing toasts from previous tests
		const existingToasts = document.querySelectorAll(".toast");
		for (const t of existingToasts) {
			if (t.dataset.timeoutId) {
				clearTimeout(Number(t.dataset.timeoutId));
			}
			t.remove();
		}

		// Remove any existing toast container
		const existingContainer = document.getElementById("toast-container");
		if (existingContainer) {
			existingContainer.remove();
		}

		// Create toast container
		container = document.createElement("div");
		container.id = "toast-container";
		document.body.appendChild(container);

		// Mock Lucide icons with createIcons function
		window.lucide = {
			icons: {},
			createIcons: () => {
				// Mock implementation that converts data-lucide attributes to SVG
				const elements = document.querySelectorAll("[data-lucide]");
				for (const el of elements) {
					const iconName = el.getAttribute("data-lucide");
					// Create a simple SVG element as a mock
					const svg = document.createElementNS("http://www.w3.org/2000/svg", "svg");
					svg.setAttribute("class", el.className);
					svg.setAttribute("data-icon", iconName);
					// Replace the i element with the svg
					el.replaceWith(svg);
				}
			},
		};

		// Set up global API for tests with full options support
		window.showToast = (message, type, options) => showToast(message, type, options);

		// Set up event listener for tests (only once)
		if (!window.__toastEventListenerRegistered) {
			window.__toastEventListenerRegistered = true;
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

		// Clear any existing toasts in container
		container.innerHTML = "";
	});

	afterEach(() => {
		// Clear ALL toasts from the entire document (not just from this container)
		// This is important because tests from other files may have created toasts
		const allToasts = document.querySelectorAll(".toast");
		for (const t of allToasts) {
			// Clear any pending timeouts
			if (t.dataset.timeoutId) {
				clearTimeout(Number(t.dataset.timeoutId));
			}
			// Remove from wherever it is in the DOM
			if (t.parentNode) {
				t.parentNode.removeChild(t);
			}
		}

		// Clean up ALL toast containers in the document
		const allContainers = document.querySelectorAll("#toast-container");
		for (const c of allContainers) {
			if (c.parentNode) {
				c.parentNode.removeChild(c);
			}
		}

		// Clean up our container reference
		container = null;

		// Clean up globals
		window.lucide = undefined;
		window.__toastEventListenerRegistered = undefined;
	});

	describe("showToast()", () => {
		it("should create and show a toast notification", () => {
			const toast = showToast("Test message", "info");

			expect(toast).toBeDefined();
			expect(toast.classList.contains("toast")).toBe(true);
			expect(toast.classList.contains("toast--info")).toBe(true);
			expect(toast.textContent).toContain("Test message");
			expect(container.contains(toast)).toBe(true);
		});

		it("should default to info type", () => {
			const toast = showToast("Test message");

			expect(toast.classList.contains("toast--info")).toBe(true);
		});

		it("should support success type", () => {
			const toast = showToast("Success!", "success");

			expect(toast.classList.contains("toast--success")).toBe(true);
		});

		it("should support error type", () => {
			const toast = showToast("Error!", "error");

			expect(toast.classList.contains("toast--error")).toBe(true);
		});

		it("should support warning type", () => {
			const toast = showToast("Warning!", "warning");

			expect(toast.classList.contains("toast--warning")).toBe(true);
		});

		it("should coerce invalid type to info", () => {
			const toast = showToast("Test", "invalid-type");

			expect(toast.classList.contains("toast--info")).toBe(true);
		});

		it("should return null if container does not exist", () => {
			container.remove();
			const toast = showToast("Test");

			expect(toast).toBeNull();
		});

		it("should include ARIA attributes", () => {
			const toast = showToast("Test message", "info");

			expect(toast.getAttribute("role")).toBe("alert");
			expect(toast.getAttribute("aria-live")).toBe("polite");
		});

		it("should include icon element", () => {
			const toast = showToast("Test message", "success");
			// After createIcons is called, the i element is replaced with svg
			const icon = toast.querySelector(".toast-icon");

			expect(icon).toBeDefined();
			expect(icon.tagName.toLowerCase()).toBe("svg");
			expect(icon.getAttribute("data-icon")).toBe("check-circle");
		});

		it("should include message content", () => {
			const toast = showToast("Test message", "info");
			const content = toast.querySelector(".toast-content");

			expect(content).toBeDefined();
			expect(content.textContent).toBe("Test message");
		});

		it("should include close button", () => {
			const toast = showToast("Test message", "info");
			const closeBtn = toast.querySelector(".toast-close");

			expect(closeBtn).toBeDefined();
			expect(closeBtn.getAttribute("type")).toBe("button");
			expect(closeBtn.getAttribute("aria-label")).toBe("Close notification");
		});

		it("should auto-dismiss with custom duration", (done) => {
			const toast = showToast("Test message", "info", { duration: 100 });

			expect(container.contains(toast)).toBe(true);

			// Wait for auto-dismiss (100ms + 500ms fallback timeout)
			setTimeout(() => {
				expect(container.contains(toast)).toBe(false);
				done();
			}, 700);
		});

		it("should auto-dismiss after default duration", (done) => {
			const toast = showToast("Test message", "info", { duration: 200 });

			expect(container.contains(toast)).toBe(true);

			// Wait for auto-dismiss (200ms + 500ms fallback timeout)
			setTimeout(() => {
				expect(container.contains(toast)).toBe(false);
				done();
			}, 800);
		});

		it("should not auto-dismiss when duration is 0", (done) => {
			const toast = showToast("Test message", "info", { duration: 0 });

			expect(container.contains(toast)).toBe(true);

			// Wait to ensure it doesn't auto-dismiss
			setTimeout(() => {
				expect(container.contains(toast)).toBe(true);
				done();
			}, 1000);
		});

		it("should store timeout ID in dataset", () => {
			const toast = showToast("Test message", "info");

			expect(toast.dataset.timeoutId).toBeDefined();
			expect(Number(toast.dataset.timeoutId)).toBeGreaterThan(0);
		});

		it("should support multiple toasts", () => {
			const toast1 = showToast("Message 1", "info");
			const toast2 = showToast("Message 2", "success");
			const toast3 = showToast("Message 3", "error");

			expect(container.children.length).toBe(3);
			expect(container.contains(toast1)).toBe(true);
			expect(container.contains(toast2)).toBe(true);
			expect(container.contains(toast3)).toBe(true);
		});
	});

	describe("removeToast()", () => {
		it("should remove a toast notification", (done) => {
			const toast = showToast("Test message", "info", { duration: 0 });

			expect(container.contains(toast)).toBe(true);

			removeToast(toast);

			// Wait for fallback timeout (500ms)
			setTimeout(() => {
				expect(container.contains(toast)).toBe(false);
				done();
			}, 600);
		});

		it("should add removing class during animation", () => {
			const toast = showToast("Test message", "info", { duration: 0 });

			removeToast(toast);

			expect(toast.classList.contains("toast--removing")).toBe(true);
		});

		it("should clear timeout if exists", () => {
			const toast = showToast("Test message", "info");
			const timeoutId = toast.dataset.timeoutId;

			expect(timeoutId).toBeDefined();

			removeToast(toast);

			expect(toast.dataset.timeoutId).toBeUndefined();
		});

		it("should handle null toast gracefully", () => {
			expect(() => removeToast(null)).not.toThrow();
		});

		it("should handle toast without parent gracefully", () => {
			const toast = document.createElement("div");
			expect(() => removeToast(toast)).not.toThrow();
		});

		it("should guard against double-invocation", () => {
			const toast = showToast("Test message", "info", { duration: 0 });

			removeToast(toast);
			expect(toast.dataset.removing).toBe("1");

			// Second call should be a no-op
			removeToast(toast);
			expect(toast.dataset.removing).toBe("1");
		});

		it("should work when called via close button", (done) => {
			const toast = showToast("Test message", "info", { duration: 0 });
			const closeBtn = toast.querySelector(".toast-close");

			expect(container.contains(toast)).toBe(true);

			closeBtn.click();

			// Wait for fallback timeout (500ms)
			setTimeout(() => {
				expect(container.contains(toast)).toBe(false);
				done();
			}, 600);
		});
	});

	describe("window.showToast global API", () => {
		it("should expose showToast globally", () => {
			expect(window.showToast).toBeDefined();
			expect(typeof window.showToast).toBe("function");
		});

		it("should create toast via global API", () => {
			const toast = window.showToast("Global test", "success");

			expect(toast).toBeDefined();
			expect(container.contains(toast)).toBe(true);
			expect(toast.textContent).toContain("Global test");
		});

		it("should support options parameter in global API", () => {
			const toast = window.showToast("Global toast with options", "info", { duration: 0 });

			expect(toast).toBeDefined();
			expect(container.contains(toast)).toBe(true);
			expect(toast.dataset.timeoutId).toBeUndefined(); // No timeout when duration is 0
		});
	});

	describe("showToast custom event", () => {
		it("should listen for showToast custom event", (done) => {
			document.dispatchEvent(
				new CustomEvent("showToast", {
					detail: { message: "Event test unique 1", type: "warning" },
				}),
			);

			// Wait for event to be processed
			setTimeout(() => {
				const toasts = container.querySelectorAll(".toast");
				// Find the toast we just created by its unique message
				const ourToast = Array.from(toasts).find((t) =>
					t.textContent.includes("Event test unique 1"),
				);
				expect(ourToast).toBeDefined();
				expect(ourToast.classList.contains("toast--warning")).toBe(true);
				done();
			}, 50);
		});

		it("should handle nested detail.value structure", (done) => {
			document.dispatchEvent(
				new CustomEvent("showToast", {
					detail: { value: { message: "Nested test unique 2", type: "error" } },
				}),
			);

			setTimeout(() => {
				const toasts = container.querySelectorAll(".toast");
				// Find the toast we just created by its unique message
				const ourToast = Array.from(toasts).find((t) =>
					t.textContent.includes("Nested test unique 2"),
				);
				expect(ourToast).toBeDefined();
				expect(ourToast.classList.contains("toast--error")).toBe(true);
				done();
			}, 50);
		});

		it("should use default message if not provided", (done) => {
			document.dispatchEvent(new CustomEvent("showToast", { detail: {} }));

			setTimeout(() => {
				const toasts = container.querySelectorAll(".toast");
				// Find a toast with the default message
				const ourToast = Array.from(toasts).find((t) => t.textContent.includes("Action completed"));
				expect(ourToast).toBeDefined();
				done();
			}, 50);
		});

		it("should support custom duration via event", (done) => {
			document.dispatchEvent(
				new CustomEvent("showToast", {
					detail: { message: "Custom duration unique 3", type: "info", duration: 100 },
				}),
			);

			setTimeout(() => {
				const toasts = container.querySelectorAll(".toast");
				// Find the toast we just created by its unique message
				const ourToast = Array.from(toasts).find((t) =>
					t.textContent.includes("Custom duration unique 3"),
				);
				expect(ourToast).toBeDefined();
				expect(ourToast.dataset.timeoutId).toBeDefined();
				done();
			}, 50);
		});
	});
});
