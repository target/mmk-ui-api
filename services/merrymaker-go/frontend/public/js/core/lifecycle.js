/**
 * Component Lifecycle Management System
 *
 * Provides automatic component initialization and cleanup for vanilla JS components.
 * Integrates with HTMX lifecycle events to handle dynamic content.
 *
 * @module core/lifecycle
 * @example
 * import { Lifecycle } from './core/lifecycle.js';
 *
 * // Register a component
 * Lifecycle.register('my-component', (element) => {
 *   const instance = new MyComponent(element);
 *   element.__myComponent = instance;
 * }, (element) => {
 *   element.__myComponent?.destroy();
 * });
 *
 * // Components with data-component="my-component" will auto-initialize
 */
import { on } from "./htmx-bridge.js";

/**
 * Component registry storing initialization and cleanup functions
 * @type {Map<string, {init: Function, cleanup: Function}>}
 */
const registry = new Map();

/**
 * Track initialized elements to prevent double initialization
 * @type {WeakSet<Element>}
 */
const initialized = new WeakSet();

const COMPONENT_SELECTOR = "[data-component]";

/**
 * Collect component nodes from a root, including the root itself when applicable.
 * @param {Element|Document|DocumentFragment} root
 * @returns {Element[]}
 */
function collectComponentNodes(root = document) {
	if (!root) {
		return [];
	}

	const nodes = [];

	const isElement = root?.nodeType === Node.ELEMENT_NODE;

	if (isElement && typeof root.matches === "function" && root.matches(COMPONENT_SELECTOR)) {
		nodes.push(root);
	}

	if (typeof root.querySelectorAll === "function") {
		nodes.push(...root.querySelectorAll(COMPONENT_SELECTOR));
	}

	return nodes;
}

/**
 * Register a component type with initialization and cleanup functions
 *
 * @param {string} name - Component name (matches data-component attribute)
 * @param {Function} initFn - Initialization function (element) => void
 * @param {Function} [cleanupFn] - Optional cleanup function (element) => void
 * @example
 * Lifecycle.register('filter-pills', (el) => {
 *   new FilterPills(el);
 * });
 */
export function register(name, initFn, cleanupFn) {
	if (!name || typeof name !== "string") {
		throw new Error("Component name must be a non-empty string");
	}
	if (typeof initFn !== "function") {
		throw new Error("Init function must be a function");
	}
	if (cleanupFn && typeof cleanupFn !== "function") {
		throw new Error("Cleanup function must be a function");
	}

	registry.set(name, { init: initFn, cleanup: cleanupFn });
}

/**
 * Initialize all registered components within a root element
 *
 * @param {Element|Document} [root=document] - Root element to search within
 * @example
 * // Initialize all components on page
 * Lifecycle.init();
 *
 * // Initialize components in a specific container
 * Lifecycle.init(document.getElementById('content'));
 */
export function init(root = document) {
	const elements = collectComponentNodes(root);

	elements.forEach((element) => {
		// Skip if already initialized
		if (initialized.has(element)) {
			return;
		}

		const componentName = element.getAttribute("data-component");
		const component = registry.get(componentName);

		if (!component) {
			console.warn(`Component "${componentName}" not registered`);
			return;
		}

		try {
			component.init(element);
			initialized.add(element);
		} catch (error) {
			console.error(`Error initializing component "${componentName}":`, error);
		}
	});
}

/**
 * Cleanup all registered components within a root element
 *
 * @param {Element|Document} [root=document] - Root element to search within
 * @example
 * // Cleanup before removing element
 * Lifecycle.cleanup(oldContainer);
 * oldContainer.remove();
 */
export function cleanup(root = document) {
	const elements = collectComponentNodes(root);

	elements.forEach((element) => {
		// Skip if not initialized
		if (!initialized.has(element)) {
			return;
		}

		const componentName = element.getAttribute("data-component");
		const component = registry.get(componentName);

		if (component?.cleanup) {
			try {
				component.cleanup(element);
			} catch (error) {
				console.error(`Error cleaning up component "${componentName}":`, error);
			}
		}

		// Note: We can't delete from WeakSet, but that's okay
		// The element will be garbage collected when removed from DOM
	});
}

/**
 * Lifecycle singleton with public API
 */
export const Lifecycle = {
	register,
	init,
	cleanup,
};

/**
 * Setup HTMX event handlers
 * Note: Auto-initialization is handled by the app entry point
 */
if (typeof document !== "undefined") {
	/**
	 * Re-initialize components after HTMX swaps
	 */
	on("htmx:afterSwap", (event) => {
		Lifecycle.init(event.target);
	});

	/**
	 * Cleanup components before HTMX removes them
	 */
	on("htmx:beforeSwap", (event) => {
		const { target, shouldSwap, isError } = event.detail ?? {};

		if (!target || shouldSwap === false || isError) {
			return;
		}

		Lifecycle.cleanup(target);
	});
}

export default Lifecycle;
