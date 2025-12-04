/**
 * Minimal Client-Side Monitoring
 * Plain JavaScript for browser injection - no TypeScript compilation complexity
 * Focuses on essential security events only
 */

(() => {
	// Prevent double initialization
	if (window.__puppeteerMonitoringActive) {
		return;
	}
	window.__puppeteerMonitoringActive = true;

	// Event collection array
	window.__puppeteerEvents = window.__puppeteerEvents || [];

	// Configuration (can be overridden by server)
	const config = {
		maxEventBuffer: 1000,
		maxValueSize: 500,
		enabledEvents: {
			localStorage: true,
			sessionStorage: true,
			dynamicCode: true,
		},
	};

	// Update config if provided by server
	if (window.__puppeteerConfig) {
		Object.assign(config, window.__puppeteerConfig);
	}

	// Utility functions
	function getCurrentTimestamp() {
		return Date.now();
	}

	function truncateValue(value, maxLength = config.maxValueSize) {
		if (typeof value !== "string") {
			value = String(value);
		}
		return value.length > maxLength
			? `${value.substring(0, maxLength)}...`
			: value;
	}

	function addEvent(eventType, data) {
		// Prevent buffer overflow
		if (window.__puppeteerEvents.length >= config.maxEventBuffer) {
			window.__puppeteerEvents.shift(); // Remove oldest event
		}

		window.__puppeteerEvents.push({
			type: eventType,
			timestamp: getCurrentTimestamp(),
			data: data,
		});
	}

	function getStackTrace() {
		try {
			throw new Error();
		} catch (e) {
			const stack = e.stack || "";
			const lines = stack.split("\n").slice(2, 4); // Get 2 frames, skip error creation
			return lines.map((line) => line.trim()).join(" | ");
		}
	}

	// ============================================================================
	// LOCALSTORAGE MONITORING (safe access)
	// ============================================================================

	let ls = null;
	if (config.enabledEvents.localStorage) {
		try {
			ls = window.localStorage;
		} catch (_) {
			ls = null;
		}
	}

	if (ls) {
		const originalSetItem = ls.setItem;
		const originalGetItem = ls.getItem;
		const originalRemoveItem = ls.removeItem;
		const originalClear = ls.clear;

		ls.setItem = function (key, value) {
			// Telemetry should not affect behavior
			try {
				addEvent("Storage.localStorageWrite", {
					key: truncateValue(key),
					value: truncateValue(value),
					valueSize: String(value).length,
					stackTrace: getStackTrace(),
				});
			} catch (_) {}
			try {
				return originalSetItem.call(this, key, value);
			} catch (err) {
				try {
					addEvent("Storage.localStorageError", {
						op: "setItem",
						key: truncateValue(key),
						error: String(err),
					});
				} catch (_) {}
				throw err;
			}
		};

		ls.getItem = function (key) {
			try {
				const result = originalGetItem.call(this, key);
				try {
					addEvent("Storage.localStorageRead", {
						key: truncateValue(key),
						found: result !== null,
						valueSize: result ? String(result).length : 0,
					});
				} catch (_) {}
				return result;
			} catch (err) {
				try {
					addEvent("Storage.localStorageError", {
						op: "getItem",
						key: truncateValue(key),
						error: String(err),
					});
				} catch (_) {}
				throw err;
			}
		};

		ls.removeItem = function (key) {
			try {
				addEvent("Storage.localStorageDelete", {
					key: truncateValue(key),
				});
			} catch (_) {}
			try {
				return originalRemoveItem.call(this, key);
			} catch (err) {
				try {
					addEvent("Storage.localStorageError", {
						op: "removeItem",
						key: truncateValue(key),
						error: String(err),
					});
				} catch (_) {}
				throw err;
			}
		};

		ls.clear = function () {
			try {
				addEvent("Storage.localStorageClear", {
					clearedAt: getCurrentTimestamp(),
				});
			} catch (_) {}
			try {
				return originalClear.call(this);
			} catch (err) {
				try {
					addEvent("Storage.localStorageError", {
						op: "clear",
						error: String(err),
					});
				} catch (_) {}
				throw err;
			}
		};
	}

	// ============================================================================
	// SESSIONSTORAGE MONITORING (safe access)
	// ============================================================================

	let ss = null;
	if (config.enabledEvents.sessionStorage) {
		try {
			ss = window.sessionStorage;
		} catch (_) {
			ss = null;
		}
	}

	if (ss) {
		const originalSetItem = ss.setItem;
		const originalGetItem = ss.getItem;
		const originalRemoveItem = ss.removeItem;
		const originalClear = ss.clear;

		ss.setItem = function (key, value) {
			try {
				addEvent("Storage.sessionStorageWrite", {
					key: truncateValue(key),
					value: truncateValue(value),
					valueSize: String(value).length,
					stackTrace: getStackTrace(),
				});
			} catch (_) {}
			try {
				return originalSetItem.call(this, key, value);
			} catch (err) {
				try {
					addEvent("Storage.sessionStorageError", {
						op: "setItem",
						key: truncateValue(key),
						error: String(err),
					});
				} catch (_) {}
				throw err;
			}
		};

		ss.getItem = function (key) {
			try {
				const result = originalGetItem.call(this, key);
				try {
					addEvent("Storage.sessionStorageRead", {
						key: truncateValue(key),
						found: result !== null,
						valueSize: result ? String(result).length : 0,
					});
				} catch (_) {}
				return result;
			} catch (err) {
				try {
					addEvent("Storage.sessionStorageError", {
						op: "getItem",
						key: truncateValue(key),
						error: String(err),
					});
				} catch (_) {}
				throw err;
			}
		};

		ss.removeItem = function (key) {
			try {
				addEvent("Storage.sessionStorageDelete", {
					key: truncateValue(key),
				});
			} catch (_) {}
			try {
				return originalRemoveItem.call(this, key);
			} catch (err) {
				try {
					addEvent("Storage.sessionStorageError", {
						op: "removeItem",
						key: truncateValue(key),
						error: String(err),
					});
				} catch (_) {}
				throw err;
			}
		};

		ss.clear = function () {
			try {
				addEvent("Storage.sessionStorageClear", {
					clearedAt: getCurrentTimestamp(),
				});
			} catch (_) {}
			try {
				return originalClear.call(this);
			} catch (err) {
				try {
					addEvent("Storage.sessionStorageError", {
						op: "clear",
						error: String(err),
					});
				} catch (_) {}
				throw err;
			}
		};
	}

	// ============================================================================
	// DYNAMIC CODE MONITORING
	// ============================================================================

	if (config.enabledEvents.dynamicCode) {
		// Monitor eval()
		if (window.eval) {
			const originalEval = window.eval;
			window.eval = function (code) {
				addEvent("Runtime.dynamicCodeEval", {
					code: truncateValue(code, 200),
					codeLength: String(code).length,
					stackTrace: getStackTrace(),
				});
				return originalEval.call(this, code);
			};
		}

		// Monitor Function constructor
		if (window.Function) {
			const originalFunction = window.Function;
			window.Function = function (...args) {
				const code = args.length > 0 ? args[args.length - 1] : "";
				addEvent("Runtime.dynamicCodeFunction", {
					code: truncateValue(code, 200),
					codeLength: String(code).length,
					argCount: args.length - 1,
					stackTrace: getStackTrace(),
				});
				return originalFunction.apply(this, args);
			};
			// Preserve prototype and constructor chain
			window.Function.prototype = originalFunction.prototype;
			try {
				Object.setPrototypeOf(window.Function, originalFunction);
			} catch (_) {}
		}

		// Monitor script element injection
		if (document.createElement) {
			const originalCreateElement = document.createElement;
			document.createElement = function (tagName) {
				const element = originalCreateElement.call(this, tagName);

				if (tagName.toLowerCase() === "script") {
					// Monitor when script content is set
					let originalTextContent = element.textContent;
					Object.defineProperty(element, "textContent", {
						get() {
							return originalTextContent;
						},
						set(value) {
							if (value?.trim()) {
								addEvent("Runtime.dynamicCodeScript", {
									code: truncateValue(value, 200),
									codeLength: value.length,
									stackTrace: getStackTrace(),
								});
							}
							originalTextContent = value;
						},
						configurable: true,
						enumerable: true,
					});

					// Monitor src via setAttribute
					const originalSetAttribute = element.setAttribute;
					element.setAttribute = function (name, value) {
						if (String(name).toLowerCase() === "src" && value) {
							addEvent("Runtime.dynamicCodeScriptSrc", {
								src: truncateValue(String(value), 200),
								stackTrace: getStackTrace(),
							});
						}
						return originalSetAttribute.call(this, name, value);
					};

					// Monitor src property assignment while preserving native semantics
					try {
						const proto =
							window.HTMLScriptElement && window.HTMLScriptElement.prototype;
						const native = proto
							? Object.getOwnPropertyDescriptor(proto, "src")
							: undefined;
						Object.defineProperty(element, "src", {
							get:
								native && native.get
									? function () {
											return native.get.call(this);
										}
									: undefined,
							set(value) {
								if (value) {
									addEvent("Runtime.dynamicCodeScriptSrc", {
										src: truncateValue(String(value), 200),
										stackTrace: getStackTrace(),
									});
								}
								return originalSetAttribute.call(this, "src", value);
							},
							configurable: true,
							enumerable: true,
						});
					} catch (_) {}
				}

				return element;
			};
		}

		// Monitor innerHTML that injects <script>
		try {
			const desc = Object.getOwnPropertyDescriptor(
				Element.prototype,
				"innerHTML",
			);
			if (desc && desc.set) {
				Object.defineProperty(Element.prototype, "innerHTML", {
					get: desc.get
						? function () {
								return desc.get.call(this);
							}
						: undefined,
					set: function (value) {
						if (typeof value === "string" && /<script/i.test(value)) {
							addEvent("Runtime.dynamicCodeInnerHTML", {
								snippet: truncateValue(value, 200),
								stackTrace: getStackTrace(),
							});
						}
						return desc.set.call(this, value);
					},
					configurable: true,
					enumerable: desc.enumerable ?? false,
				});
			}
		} catch (_) {}
	}

	// ============================================================================
	// INITIALIZATION COMPLETE
	// ============================================================================

	addEvent("Security.monitoringInitialized", {
		timestamp: getCurrentTimestamp(),
		enabledEvents: Object.keys(config.enabledEvents).filter(
			(key) => config.enabledEvents[key],
		),
		userAgent: navigator.userAgent,
		url: window.location.href,
	});

	// Expose utility functions for external access
	window.__puppeteerMonitoring = {
		getEvents: () => {
			return window.__puppeteerEvents.slice(); // Return copy
		},
		clearEvents: () => {
			window.__puppeteerEvents.length = 0;
		},
		getEventCount: () => {
			return window.__puppeteerEvents.length;
		},
		isActive: () => {
			return window.__puppeteerMonitoringActive;
		},
	};
})();
