/**
 * Simple Reactive State Management
 *
 * Provides observable state with subscription system for sharing data across components.
 * Uses pub/sub pattern for simplicity - no complex reactivity.
 *
 * @module core/state
 * @example
 * import { State } from './core/state.js';
 *
 * const appState = new State({
 *   theme: 'light',
 *   user: null
 * });
 *
 * // Subscribe to changes
 * appState.subscribe('theme', (newValue, oldValue) => {
 *   document.body.className = `theme-${newValue}`;
 * });
 *
 * // Update state
 * appState.set('theme', 'dark');
 *
 * // Get state
 * const theme = appState.get('theme');
 */

/**
 * State management class
 */
export class State {
	/**
	 * Create a new state store
	 *
	 * @param {Object} initialState - Initial state object
	 * @example
	 * const state = new State({ count: 0 });
	 */
	constructor(initialState = {}) {
		this._state = { ...initialState };
		this._subscribers = new Map();
	}

	/**
	 * Get a state value
	 *
	 * @param {string} key - State key
	 * @returns {*} State value
	 * @example
	 * const count = state.get('count');
	 */
	get(key) {
		return this._state[key];
	}

	/**
	 * Set a state value and notify subscribers
	 *
	 * @param {string} key - State key
	 * @param {*} value - New value
	 * @example
	 * state.set('count', 42);
	 */
	set(key, value) {
		const oldValue = this._state[key];

		// Only update and notify if value changed
		if (oldValue === value) {
			return;
		}

		this._state[key] = value;
		this._notify(key, value, oldValue);
	}

	/**
	 * Subscribe to state changes
	 *
	 * @param {string} key - State key to watch
	 * @param {Function} callback - Callback function (newValue, oldValue) => void
	 * @returns {Function} Unsubscribe function
	 * @example
	 * const unsubscribe = state.subscribe('count', (newVal, oldVal) => {
	 *   console.log(`Count changed from ${oldVal} to ${newVal}`);
	 * });
	 *
	 * // Later: unsubscribe()
	 */
	subscribe(key, callback) {
		if (typeof callback !== "function") {
			throw new Error("Callback must be a function");
		}

		if (!this._subscribers.has(key)) {
			this._subscribers.set(key, new Set());
		}

		const subscribers = this._subscribers.get(key);
		subscribers.add(callback);

		// Return unsubscribe function
		return () => {
			subscribers.delete(callback);
		};
	}

	/**
	 * Notify all subscribers of a state change
	 *
	 * @param {string} key - State key
	 * @param {*} newValue - New value
	 * @param {*} oldValue - Old value
	 * @private
	 */
	_notify(key, newValue, oldValue) {
		const subscribers = this._subscribers.get(key);

		if (!subscribers || subscribers.size === 0) {
			return;
		}

		subscribers.forEach((callback) => {
			try {
				callback(newValue, oldValue);
			} catch (error) {
				console.error(`Error in state subscriber for "${key}":`, error);
			}
		});
	}

	/**
	 * Get all state as plain object
	 *
	 * @returns {Object} State object
	 * @example
	 * const allState = state.getAll();
	 */
	getAll() {
		return { ...this._state };
	}

	/**
	 * Reset state to initial or provided values
	 *
	 * @param {Object} [newState] - New state object
	 * @example
	 * state.reset({ count: 0 });
	 */
	reset(newState = {}) {
		const oldState = { ...this._state };
		this._state = { ...newState };

		// Notify all subscribers of changes
		Object.keys(this._state).forEach((key) => {
			if (oldState[key] !== this._state[key]) {
				this._notify(key, this._state[key], oldState[key]);
			}
		});
	}
}

export default State;
