/**
 * HTMX event bridge
 *
 * Provides a single place to subscribe to HTMX lifecycle events so features
 * don't have to register their own document-level listeners. Features call
 * `on("htmx:afterSwap", handler)` and receive an unsubscribe function.
 */

const registry = new Map();

function hasDocument() {
	return typeof document !== "undefined" && typeof document.addEventListener === "function";
}

function normalizeListenerOptions(options) {
	if (options == null) {
		return undefined;
	}

	if (typeof options === "boolean") {
		return options;
	}

	if (typeof options === "object") {
		const normalized = {};
		if ("capture" in options) normalized.capture = !!options.capture;
		if ("passive" in options) normalized.passive = !!options.passive;

		return Object.keys(normalized).length ? normalized : undefined;
	}

	return undefined;
}

function entryKey(eventName, listenerOptions) {
	const optionsKey = JSON.stringify(listenerOptions ?? null);
	return `${eventName}::${optionsKey}`;
}

function ensureEntry(eventName, options) {
	if (!eventName) {
		throw new Error("htmx-bridge: eventName is required");
	}

	const listenerOptions = normalizeListenerOptions(options);
	const key = entryKey(eventName, listenerOptions);
	let entry = registry.get(key);
	if (entry) return entry;

	const handlers = new Set();
	const delegatedListener = (event) => {
		for (const handler of handlers) {
			try {
				handler(event);
			} catch (error) {
				console.error(`htmx-bridge handler failed for ${eventName}`, error);
			}
		}
	};

	entry = { eventName, handlers, delegatedListener, options: listenerOptions, key };
	registry.set(key, entry);

	if (hasDocument()) {
		document.addEventListener(eventName, delegatedListener, listenerOptions);
	}

	return entry;
}

export function on(eventName, handler, options) {
	if (typeof handler !== "function") {
		throw new Error(`htmx-bridge: handler for ${eventName} must be a function`);
	}

	const entry = ensureEntry(eventName, options);
	entry.handlers.add(handler);

	return () => off(eventName, handler, options);
}

export function once(eventName, handler, options) {
	if (typeof handler !== "function") {
		throw new Error(`htmx-bridge: handler for ${eventName} must be a function`);
	}

	const unsubscribe = on(
		eventName,
		(event) => {
			unsubscribe();
			handler(event);
		},
		options,
	);

	return unsubscribe;
}

function cleanupEntry(targetKey, entry) {
	if (hasDocument()) {
		document.removeEventListener(entry.eventName, entry.delegatedListener, entry.options);
	}
	registry.delete(targetKey);
}

export function off(eventName, handler, options) {
	if (options !== undefined) {
		const listenerOptions = normalizeListenerOptions(options);
		const key = entryKey(eventName, listenerOptions);
		const entry = registry.get(key);
		if (!entry) return;

		entry.handlers.delete(handler);
		if (entry.handlers.size === 0) {
			cleanupEntry(key, entry);
		}
		return;
	}

	for (const [key, entry] of registry) {
		if (entry.eventName !== eventName) continue;
		if (!entry.handlers.has(handler)) continue;

		entry.handlers.delete(handler);
		if (entry.handlers.size === 0) {
			cleanupEntry(key, entry);
		}
	}
}

export function reset() {
	for (const [key, entry] of registry) {
		if (hasDocument()) {
			document.removeEventListener(entry.eventName, entry.delegatedListener, entry.options);
		}
		registry.delete(key);
	}

	registry.clear();
}

const bridge = { on, once, off, reset };

if (typeof window !== "undefined" && !window.__htmxBridge) {
	window.__htmxBridge = bridge;
}

export default bridge;
