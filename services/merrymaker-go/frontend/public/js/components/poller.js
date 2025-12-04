/**
 * Generic Polling Component
 *
 * Provides a reusable polling abstraction with exponential backoff, timeout handling,
 * terminal state detection, and AbortController support for cancellation.
 *
 * Consolidates polling logic from source_form.js and job_status.js into a single
 * reusable component.
 *
 * @module components/poller
 * @example
 * // Programmatic usage
 * import { Poller } from './components/poller.js';
 *
 * const poller = new Poller({
 *   url: '/api/jobs/123/status',
 *   interval: 2000,
 *   maxInterval: 30000,
 *   maxAttempts: 30,
 *   timeout: 10000,
 *   onData: (data) => updateUI(data),
 *   onError: (error) => showError(error),
 *   isTerminal: (data) => data.status === 'completed' || data.status === 'failed'
 * });
 *
 * poller.start();
 * // Later: poller.stop();
 *
 * @example
 * // Declarative usage in templates
 * <div data-component="poller"
 *      data-poll-url="/api/jobs/{{.JobID}}/status"
 *      data-poll-interval="2000"
 *      data-poll-terminal="completed,failed"
 *      id="status-container">
 * </div>
 *
 * <script type="module">
 *   document.getElementById('status-container')
 *     .addEventListener('poll:data', (e) => {
 *       console.log('Received data:', e.detail);
 *     });
 * </script>
 */

import { Lifecycle } from "../core/lifecycle.js";

/**
 * Poller class for generic polling with backoff and error handling
 */
export class Poller {
	/**
	 * Create a new Poller instance
	 *
	 * @param {Object} options - Poller configuration
	 * @param {string} options.url - URL to poll
	 * @param {number} [options.interval=2000] - Initial polling interval in milliseconds
	 * @param {number} [options.maxInterval=30000] - Maximum backoff interval in milliseconds
	 * @param {number} [options.maxAttempts=30] - Maximum number of polling attempts
	 * @param {number} [options.timeout=10000] - Request timeout in milliseconds
	 * @param {number} [options.maxConsecutiveFailures=3] - Maximum consecutive failures before stopping
	 * @param {Function} [options.onData] - Callback when data is received (data) => void
	 * @param {Function} [options.onError] - Callback when error occurs (error) => void
	 * @param {Function} [options.isTerminal] - Function to check if data represents terminal state (data) => boolean
	 * @param {HTMLElement} [options.element] - Element to emit custom events on
	 */
	constructor(options) {
		this.url = options.url;
		this.interval = options.interval || 2000;
		this.maxInterval = options.maxInterval || 30000;
		this.maxAttempts = options.maxAttempts || 30;
		this.timeout = options.timeout || 10000;
		this.maxConsecutiveFailures = options.maxConsecutiveFailures || 3;
		this.onData = options.onData || null;
		this.onError = options.onError || null;
		this.isTerminal = options.isTerminal || (() => false);
		this.element = options.element || null;

		// Internal state
		this.attempts = 0;
		this.consecutiveFailures = 0;
		this.inFlight = false;
		this.stopped = true; // Start in stopped state so start() works intuitively
		this.controller = null;
		this.timerId = null;
	}

	/**
	 * Start polling
	 */
	start() {
		if (!this.stopped) {
			// Already running or was never stopped
			return;
		}
		this.stopped = false;
		this.attempts = 0;
		this.consecutiveFailures = 0;
		this.controller = new AbortController();
		this.tick();
	}

	/**
	 * Stop polling
	 */
	stop() {
		this.stopped = true;
		if (this.controller) {
			this.controller.abort();
		}
		if (this.timerId) {
			clearTimeout(this.timerId);
			this.timerId = null;
		}
	}

	/**
	 * Perform a single poll tick
	 * @private
	 */
	async tick() {
		if (this.stopped) return;

		// Skip if request is already in flight
		if (this.inFlight) {
			this.timerId = setTimeout(() => this.tick(), this.calculateBackoffInterval());
			return;
		}

		// Check max attempts
		if (this.attempts >= this.maxAttempts) {
			this.handleTimeout();
			return;
		}

		this.attempts++;
		this.inFlight = true;

		try {
			const response = await this.fetchWithTimeout(
				this.url,
				{ signal: this.controller.signal },
				this.timeout,
			);

			if (!response.ok) {
				const error = new Error(`HTTP ${response.status}`);
				error.name = "HTTPError";
				error.status = response.status;
				throw error;
			}

			const data = await response.json();

			// Reset consecutive failures on success
			this.consecutiveFailures = 0;

			// Emit data event
			this.emitEvent("poll:data", data);

			// Call onData callback
			if (this.onData) {
				this.onData(data);
			}

			// Check if terminal state reached
			if (this.isTerminal(data)) {
				this.stop();
				return;
			}
		} catch (error) {
			// Only return early if explicitly stopped (not for timeouts or other errors)
			if (error.name === "AbortError" && this.stopped) {
				return;
			}

			this.consecutiveFailures++;

			// Emit error event with detailed information
			const errorPayload = {
				name: error.name,
				message: error.message,
				attempts: this.attempts,
			};
			if (error.status) {
				errorPayload.status = error.status;
			}
			this.emitEvent("poll:error", errorPayload);

			// Call onError callback
			if (this.onError) {
				this.onError(error);
			}

			// Stop after too many consecutive failures
			if (this.consecutiveFailures >= this.maxConsecutiveFailures) {
				this.stop();
				return;
			}
		} finally {
			this.inFlight = false;
		}

		// Schedule next tick with backoff
		const nextInterval = this.calculateBackoffInterval();
		this.timerId = setTimeout(() => this.tick(), nextInterval);
	}

	/**
	 * Calculate backoff interval based on consecutive failures
	 * @private
	 * @returns {number} Next polling interval in milliseconds
	 */
	calculateBackoffInterval() {
		if (this.consecutiveFailures === 0) {
			return this.interval;
		}
		const backoffMs = this.interval * 2 ** (this.consecutiveFailures - 1);
		return Math.min(backoffMs, this.maxInterval);
	}

	/**
	 * Handle timeout when max attempts reached
	 * @private
	 */
	handleTimeout() {
		this.stop();
		const timeoutError = new Error("Polling timeout: max attempts reached");
		this.emitEvent("poll:timeout", { attempts: this.attempts });
		if (this.onError) {
			this.onError(timeoutError);
		}
	}

	/**
	 * Fetch with timeout support
	 *
	 * Uses its own AbortController to enforce timeout while respecting the caller's signal.
	 * Throws TimeoutError on timeout (distinct from AbortError for explicit stops).
	 *
	 * @private
	 * @param {string} url - URL to fetch
	 * @param {Object} options - Fetch options
	 * @param {number} timeoutMs - Timeout in milliseconds
	 * @returns {Promise<Response>} Fetch response
	 */
	async fetchWithTimeout(url, options = {}, timeoutMs = 10000) {
		const outerSignal = options.signal;
		const ac = new AbortController();
		let timedOut = false;

		// Listen to outer signal and propagate abort
		const onOuterAbort = () => ac.abort();
		if (outerSignal) {
			if (outerSignal.aborted) {
				ac.abort();
			} else {
				outerSignal.addEventListener("abort", onOuterAbort, { once: true });
			}
		}

		// Set timeout to abort the request
		const timeoutId = setTimeout(() => {
			timedOut = true;
			ac.abort();
		}, timeoutMs);

		try {
			const response = await fetch(url, {
				...options,
				signal: ac.signal,
				credentials: "same-origin",
			});
			return response;
		} catch (err) {
			// If aborted by timeout (not by outer signal), throw TimeoutError
			if (timedOut && err.name === "AbortError") {
				const timeoutError = new Error("Request timeout");
				timeoutError.name = "TimeoutError";
				throw timeoutError;
			}
			// Otherwise, propagate the original error
			throw err;
		} finally {
			clearTimeout(timeoutId);
			if (outerSignal) {
				outerSignal.removeEventListener("abort", onOuterAbort);
			}
		}
	}

	/**
	 * Emit custom event on element
	 * @private
	 * @param {string} eventName - Event name
	 * @param {*} detail - Event detail data
	 */
	emitEvent(eventName, detail) {
		if (this.element) {
			const event = new CustomEvent(eventName, {
				detail,
				bubbles: true,
				cancelable: false,
			});
			this.element.dispatchEvent(event);
		}
	}
}

/**
 * Initialize poller from declarative markup
 *
 * Reads configuration from data attributes and creates a Poller instance.
 * Automatically starts polling when initialized.
 *
 * @param {HTMLElement} element - Element with data-poll-* attributes
 * @example
 * // In template:
 * <div data-component="poller"
 *      data-poll-url="/api/jobs/123/status"
 *      data-poll-interval="2000"
 *      data-poll-terminal="completed,failed">
 * </div>
 *
 * // Listen for events:
 * element.addEventListener('poll:data', (e) => {
 *   console.log('Data received:', e.detail);
 * });
 */
function initPoller(element) {
	const url = element.dataset.pollUrl;
	if (!url) {
		console.warn("Poller component missing data-poll-url attribute", element);
		return;
	}

	const interval = Number.parseInt(element.dataset.pollInterval, 10) || 2000;
	const maxInterval = Number.parseInt(element.dataset.pollMaxInterval, 10) || 30000;
	const maxAttempts = Number.parseInt(element.dataset.pollMaxAttempts, 10) || 30;
	const timeout = Number.parseInt(element.dataset.pollTimeout, 10) || 10000;

	// Parse terminal states from comma-separated list
	const terminalStates = element.dataset.pollTerminal
		? element.dataset.pollTerminal.split(",").map((s) => s.trim())
		: [];

	// Use optional chaining to avoid crashes on malformed/unexpected JSON
	const isTerminal = terminalStates.length
		? (data) => terminalStates.includes(data?.status)
		: () => false;

	const poller = new Poller({
		url,
		interval,
		maxInterval,
		maxAttempts,
		timeout,
		isTerminal,
		element,
	});

	// Store instance on element for external access
	element.__poller = poller;

	// Auto-start polling (no workaround needed - constructor defaults to stopped=true)
	poller.start();
}

/**
 * Cleanup poller instance
 *
 * Stops polling and removes the instance from the element.
 * Called automatically by the Lifecycle system when the element is removed.
 *
 * @param {HTMLElement} element - Element with poller instance
 */
function cleanupPoller(element) {
	if (element.__poller) {
		element.__poller.stop();
		element.__poller = undefined;
	}
}

// Register with lifecycle system
Lifecycle.register("poller", initPoller, cleanupPoller);
