/**
 * DOM Utility Functions
 *
 * Provides cleaner API than verbose native DOM methods.
 * Simplifies element creation, querying, and manipulation.
 *
 * @module core/dom
 * @example
 * import { h, qs, qsa } from './core/dom.js';
 *
 * // Create elements
 * const button = h('button', {
 *   className: 'btn btn-primary',
 *   text: 'Click me',
 *   onClick: () => alert('Clicked!')
 * });
 *
 * // Query elements
 * const form = qs('#my-form');
 * const inputs = qsa('input', form);
 */

/**
 * Create a DOM element with attributes and children
 *
 * @param {string} tag - HTML tag name
 * @param {Object} [attrs={}] - Element attributes and properties
 * @param {Array|Element|string} [children=[]] - Child elements or text
 * @returns {Element} Created element
 * @example
 * const div = h('div', { className: 'container' }, [
 *   h('h1', { text: 'Title' }),
 *   h('p', { text: 'Content' })
 * ]);
 */
export function h(tag, attrs = {}, children = []) {
	const element = document.createElement(tag);

	// Set attributes and properties
	Object.entries(attrs).forEach(([key, value]) => {
		if (key === "className") {
			element.className = value;
		} else if (key === "text") {
			element.textContent = value;
		} else if (key === "html") {
			element.innerHTML = value;
		} else if (key.startsWith("on") && typeof value === "function") {
			// Event handlers: onClick, onInput, etc.
			const eventName = key.slice(2).toLowerCase();
			element.addEventListener(eventName, value);
		} else if (key.startsWith("data-")) {
			// Data attributes
			element.setAttribute(key, value);
		} else if (key === "style" && typeof value === "object") {
			// Style object
			Object.assign(element.style, value);
		} else if (value === true) {
			// Boolean attributes
			element.setAttribute(key, "");
		} else if (value !== false && value != null) {
			// Regular attributes
			element.setAttribute(key, value);
		}
	});

	// Append children
	if (children) {
		if (Array.isArray(children)) {
			children.forEach((child) => {
				appendChild(element, child);
			});
		} else {
			appendChild(element, children);
		}
	}

	return element;
}

/**
 * Append a child to an element
 *
 * @param {Element} parent - Parent element
 * @param {Element|string|Array} child - Child element, text, or array of children
 * @private
 */
function appendChild(parent, child) {
	if (!child) {
		return;
	}

	if (Array.isArray(child)) {
		for (const c of child) {
			appendChild(parent, c);
		}
	} else if (typeof child === "string") {
		parent.appendChild(document.createTextNode(child));
	} else if (child instanceof Element) {
		parent.appendChild(child);
	}
}

/**
 * Query selector (shorthand for querySelector)
 *
 * @param {string} selector - CSS selector
 * @param {Element|Document} [root=document] - Root element to search within
 * @returns {Element|null} Matched element or null
 * @example
 * const form = qs('#my-form');
 * const input = qs('input[name="email"]', form);
 */
export function qs(selector, root = document) {
	return root.querySelector(selector);
}

/**
 * Query selector all (shorthand for querySelectorAll)
 *
 * @param {string} selector - CSS selector
 * @param {Element|Document} [root=document] - Root element to search within
 * @returns {Element[]} Array of matched elements
 * @example
 * const inputs = qsa('input', form);
 * inputs.forEach(input => input.disabled = true);
 */
export function qsa(selector, root = document) {
	return Array.from(root.querySelectorAll(selector));
}

/**
 * Create a document fragment from HTML string
 *
 * @param {string} html - HTML string
 * @returns {DocumentFragment} Document fragment
 * @example
 * const fragment = htmlToFragment('<div>Hello</div><div>World</div>');
 * container.appendChild(fragment);
 */
export function htmlToFragment(html) {
	const template = document.createElement("template");
	template.innerHTML = html.trim();
	return template.content;
}

/**
 * Create an element from HTML string
 *
 * @param {string} html - HTML string
 * @returns {Element} First element in HTML
 * @example
 * const div = htmlToElement('<div class="box">Content</div>');
 */
export function htmlToElement(html) {
	const fragment = htmlToFragment(html);
	return fragment.firstElementChild;
}

/**
 * Remove all children from an element
 *
 * @param {Element} element - Element to clear
 * @example
 * clearElement(container);
 */
export function clearElement(element) {
	while (element.firstChild) {
		element.removeChild(element.firstChild);
	}
}

/**
 * Check if element matches selector
 *
 * @param {Element} element - Element to check
 * @param {string} selector - CSS selector
 * @returns {boolean} True if element matches selector
 * @example
 * if (matches(element, '.active')) {
 *   // Element has active class
 * }
 */
export function matches(element, selector) {
	return element.matches(selector);
}

/**
 * Get closest ancestor matching selector
 *
 * @param {Element} element - Starting element
 * @param {string} selector - CSS selector
 * @returns {Element|null} Closest matching ancestor or null
 * @example
 * const form = closest(input, 'form');
 */
export function closest(element, selector) {
	return element.closest(selector);
}

export default {
	h,
	qs,
	qsa,
	htmlToFragment,
	htmlToElement,
	clearElement,
	matches,
	closest,
};
