/**
 * Global Event Delegation System
 *
 * Provides efficient event handling for dynamic content via document-level delegation.
 * Works seamlessly with HTMX-swapped content without re-registering listeners.
 *
 * @module core/events
 * @example
 * import { Events } from './core/events.js';
 *
 * // Register delegated event handler
 * Events.on('[data-action="toggle"]', 'click', (event, element) => {
 *   element.classList.toggle('active');
 * });
 *
 * // Remove handler
 * Events.off('[data-action="toggle"]', 'click');
 */

/**
 * Store registered event delegates
 * @type {Map<string, Map<string, Function>>}
 */
const delegates = new Map();

/**
 * Register a delegated event handler
 *
 * @param {string} selector - CSS selector to match elements
 * @param {string} eventType - Event type (e.g., 'click', 'input', 'change')
 * @param {Function} handler - Handler function (event, matchedElement) => void
 * @example
 * Events.on('[data-filter-value]', 'click', (e, pill) => {
 *   console.log('Clicked pill:', pill.dataset.filterValue);
 * });
 */
export function on(selector, eventType, handler) {
	if (!selector || typeof selector !== "string") {
		throw new Error("Selector must be a non-empty string");
	}
	if (!eventType || typeof eventType !== "string") {
		throw new Error("Event type must be a non-empty string");
	}
	if (typeof handler !== "function") {
		throw new Error("Handler must be a function");
	}

	// Get or create event type map
	if (!delegates.has(eventType)) {
		delegates.set(eventType, new Map());

		// Register document-level listener for this event type
		document.addEventListener(
			eventType,
			(event) => {
				handleEvent(event, eventType);
			},
			true,
		); // Use capture phase for better performance
	}

	const eventMap = delegates.get(eventType);
	eventMap.set(selector, handler);
}

/**
 * Remove a delegated event handler
 *
 * @param {string} selector - CSS selector
 * @param {string} eventType - Event type
 * @example
 * Events.off('[data-filter-value]', 'click');
 */
export function off(selector, eventType) {
	const eventMap = delegates.get(eventType);
	if (eventMap) {
		eventMap.delete(selector);
	}
}

/**
 * Handle delegated event by checking if target matches any registered selectors
 *
 * @param {Event} event - DOM event
 * @param {string} eventType - Event type
 * @private
 */
function handleEvent(event, eventType) {
	const eventMap = delegates.get(eventType);
	if (!eventMap || eventMap.size === 0) {
		return;
	}

	// Check each registered selector
	for (const [selector, handler] of eventMap.entries()) {
		// Find closest matching element (supports event bubbling)
		const matchedElement = event.target.closest(selector);

		if (matchedElement) {
			try {
				handler(event, matchedElement);
			} catch (error) {
				console.error(`Error in event handler for "${selector}":`, error);
			}

			// Don't break - multiple handlers might match
		}
	}
}

/**
 * Events singleton with public API
 */
export const Events = {
	on,
	off,
};

export default Events;
