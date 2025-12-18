import { afterEach, beforeEach, describe, expect, it, mock } from "bun:test";
import { Poller } from "./poller.js";

describe("Poller", () => {
	let originalFetch;
	let fetchMock;

	beforeEach(() => {
		// Mock fetch
		originalFetch = global.fetch;
		fetchMock = mock(() => Promise.resolve());
		global.fetch = fetchMock;
	});

	afterEach(() => {
		// Restore fetch
		global.fetch = originalFetch;
	});

	describe("constructor", () => {
		it("should initialize with required options", () => {
			const poller = new Poller({ url: "/api/test" });

			expect(poller.url).toBe("/api/test");
			expect(poller.interval).toBe(2000);
			expect(poller.maxInterval).toBe(30000);
			expect(poller.maxAttempts).toBe(30);
			expect(poller.timeout).toBe(10000);
			expect(poller.maxConsecutiveFailures).toBe(3);
		});

		it("should accept custom options", () => {
			const onData = () => {};
			const onError = () => {};
			const isTerminal = () => true;

			const poller = new Poller({
				url: "/api/test",
				interval: 1000,
				maxInterval: 15000,
				maxAttempts: 10,
				timeout: 5000,
				onData,
				onError,
				isTerminal,
			});

			expect(poller.interval).toBe(1000);
			expect(poller.maxInterval).toBe(15000);
			expect(poller.maxAttempts).toBe(10);
			expect(poller.timeout).toBe(5000);
			expect(poller.onData).toBe(onData);
			expect(poller.onError).toBe(onError);
			expect(poller.isTerminal).toBe(isTerminal);
		});

		it("should initialize internal state", () => {
			const poller = new Poller({ url: "/api/test" });

			expect(poller.attempts).toBe(0);
			expect(poller.consecutiveFailures).toBe(0);
			expect(poller.inFlight).toBe(false);
			expect(poller.stopped).toBe(true); // Starts in stopped state
			expect(poller.controller).toBe(null);
			expect(poller.timerId).toBe(null);
		});
	});

	describe("start", () => {
		it("should initialize controller and start polling", async () => {
			fetchMock.mockImplementation(() =>
				Promise.resolve({
					ok: true,
					json: () => Promise.resolve({ status: "pending" }),
				}),
			);

			const poller = new Poller({ url: "/api/test" });
			// No need to set stopped=true - constructor defaults to stopped=true
			poller.start();

			expect(poller.stopped).toBe(false);
			expect(poller.controller).not.toBe(null);
			// Note: attempts is incremented immediately when tick() is called
			expect(poller.consecutiveFailures).toBe(0);

			// Wait for first tick to complete
			await new Promise((resolve) => setTimeout(resolve, 10));

			expect(fetchMock).toHaveBeenCalled();
			expect(poller.attempts).toBeGreaterThan(0);

			poller.stop();
		});

		it("should not restart if already running", () => {
			const poller = new Poller({ url: "/api/test" });
			poller.start(); // Start it first
			poller.stopped = false; // Ensure it's running

			const originalController = poller.controller;
			poller.start(); // Try to start again

			expect(poller.controller).toBe(originalController);
		});
	});

	describe("stop", () => {
		it("should abort controller and clear timer", () => {
			const poller = new Poller({ url: "/api/test" });
			poller.controller = new AbortController();
			poller.timerId = setTimeout(() => {}, 1000);

			const abortSpy = mock(() => {});
			poller.controller.abort = abortSpy;

			poller.stop();

			expect(poller.stopped).toBe(true);
			expect(abortSpy).toHaveBeenCalled();
			expect(poller.timerId).toBe(null);
		});

		it("should handle missing controller gracefully", () => {
			const poller = new Poller({ url: "/api/test" });
			poller.controller = null;

			expect(() => poller.stop()).not.toThrow();
			expect(poller.stopped).toBe(true);
		});
	});

	describe("tick", () => {
		it("should fetch data and call onData callback", async () => {
			const testData = { status: "running", progress: 50 };
			fetchMock.mockImplementation(() =>
				Promise.resolve({
					ok: true,
					json: () => Promise.resolve(testData),
				}),
			);

			const onDataMock = mock(() => {});
			const poller = new Poller({
				url: "/api/test",
				onData: onDataMock,
			});

			poller.controller = new AbortController();
			poller.stopped = false; // Not stopped
			await poller.tick();

			expect(fetchMock).toHaveBeenCalled();
			expect(onDataMock).toHaveBeenCalledWith(testData);
			expect(poller.consecutiveFailures).toBe(0);
		});

		it("should stop polling on terminal state", async () => {
			fetchMock.mockImplementation(() =>
				Promise.resolve({
					ok: true,
					json: () => Promise.resolve({ status: "completed" }),
				}),
			);

			const poller = new Poller({
				url: "/api/test",
				isTerminal: (data) => data.status === "completed",
			});

			poller.controller = new AbortController();
			poller.stopped = false; // Not stopped
			await poller.tick();

			expect(poller.stopped).toBe(true);
		});

		it("should handle HTTP errors with status", async () => {
			fetchMock.mockImplementation(() =>
				Promise.resolve({
					ok: false,
					status: 500,
				}),
			);

			const onErrorMock = mock(() => {});
			const poller = new Poller({
				url: "/api/test",
				onError: onErrorMock,
			});

			poller.controller = new AbortController();
			poller.stopped = false; // Not stopped
			await poller.tick();

			expect(onErrorMock).toHaveBeenCalled();
			const error = onErrorMock.mock.calls[0][0];
			expect(error.name).toBe("HTTPError");
			expect(error.status).toBe(500);
			expect(poller.consecutiveFailures).toBe(1);
		});

		it("should stop after 3 consecutive failures by default", async () => {
			fetchMock.mockImplementation(() =>
				Promise.resolve({
					ok: false,
					status: 500,
				}),
			);

			const poller = new Poller({ url: "/api/test" });
			poller.controller = new AbortController();
			poller.stopped = false; // Not stopped

			// First failure
			await poller.tick();
			expect(poller.consecutiveFailures).toBe(1);
			expect(poller.stopped).toBe(false);

			// Second failure
			await poller.tick();
			expect(poller.consecutiveFailures).toBe(2);
			expect(poller.stopped).toBe(false);

			// Third failure - should stop
			await poller.tick();
			expect(poller.consecutiveFailures).toBe(3);
			expect(poller.stopped).toBe(true);
		});

		it("should respect custom maxConsecutiveFailures", async () => {
			fetchMock.mockImplementation(() =>
				Promise.resolve({
					ok: false,
					status: 500,
				}),
			);

			const poller = new Poller({
				url: "/api/test",
				maxConsecutiveFailures: 5,
			});
			poller.controller = new AbortController();
			poller.stopped = false; // Not stopped

			// First 4 failures should not stop
			for (let i = 1; i <= 4; i++) {
				await poller.tick();
				expect(poller.consecutiveFailures).toBe(i);
				expect(poller.stopped).toBe(false);
			}

			// Fifth failure should stop
			await poller.tick();
			expect(poller.consecutiveFailures).toBe(5);
			expect(poller.stopped).toBe(true);
		});

		it("should reset consecutive failures on success", async () => {
			const poller = new Poller({ url: "/api/test" });
			poller.controller = new AbortController();
			poller.stopped = false; // Not stopped
			poller.consecutiveFailures = 2;

			fetchMock.mockImplementation(() =>
				Promise.resolve({
					ok: true,
					json: () => Promise.resolve({ status: "running" }),
				}),
			);

			await poller.tick();

			expect(poller.consecutiveFailures).toBe(0);
		});

		it("should skip tick if request is in flight", async () => {
			const poller = new Poller({ url: "/api/test" });
			poller.controller = new AbortController();
			poller.stopped = false; // Not stopped
			poller.inFlight = true;

			await poller.tick();

			expect(fetchMock).not.toHaveBeenCalled();
			// Timer should be scheduled with backoff interval
			expect(poller.timerId).not.toBe(null);

			clearTimeout(poller.timerId);
		});

		it("should stop on max attempts", async () => {
			const poller = new Poller({
				url: "/api/test",
				maxAttempts: 3,
			});
			poller.controller = new AbortController();
			poller.attempts = 3;

			await poller.tick();

			expect(poller.stopped).toBe(true);
			expect(fetchMock).not.toHaveBeenCalled();
		});

		it("should handle AbortError when stopped", async () => {
			fetchMock.mockImplementation(() => {
				const error = new Error("Aborted");
				error.name = "AbortError";
				return Promise.reject(error);
			});

			const onErrorMock = mock(() => {});
			const poller = new Poller({
				url: "/api/test",
				onError: onErrorMock,
			});
			poller.controller = new AbortController();
			poller.stopped = true; // Explicitly stopped

			await poller.tick();

			// AbortError when stopped should not increment consecutive failures
			expect(poller.consecutiveFailures).toBe(0);
			// onError should not be called for AbortError when stopped
			expect(onErrorMock).not.toHaveBeenCalled();
		});

		it("should treat TimeoutError as transient failure", async () => {
			fetchMock.mockImplementation(() => {
				const error = new Error("Request timeout");
				error.name = "TimeoutError";
				return Promise.reject(error);
			});

			const onErrorMock = mock(() => {});
			const poller = new Poller({
				url: "/api/test",
				onError: onErrorMock,
			});
			poller.controller = new AbortController();
			poller.stopped = false; // Not stopped

			await poller.tick();

			// TimeoutError should increment consecutive failures
			expect(poller.consecutiveFailures).toBe(1);
			// onError should be called for TimeoutError
			expect(onErrorMock).toHaveBeenCalled();
			const error = onErrorMock.mock.calls[0][0];
			expect(error.name).toBe("TimeoutError");
		});
	});

	describe("calculateBackoffInterval", () => {
		it("should return base interval with no failures", () => {
			const poller = new Poller({ url: "/api/test", interval: 1000 });
			poller.consecutiveFailures = 0;

			expect(poller.calculateBackoffInterval()).toBe(1000);
		});

		it("should apply exponential backoff", () => {
			const poller = new Poller({
				url: "/api/test",
				interval: 1000,
				maxInterval: 30000,
			});

			poller.consecutiveFailures = 1;
			expect(poller.calculateBackoffInterval()).toBe(1000);

			poller.consecutiveFailures = 2;
			expect(poller.calculateBackoffInterval()).toBe(2000);

			poller.consecutiveFailures = 3;
			expect(poller.calculateBackoffInterval()).toBe(4000);

			poller.consecutiveFailures = 4;
			expect(poller.calculateBackoffInterval()).toBe(8000);
		});

		it("should cap at maxInterval", () => {
			const poller = new Poller({
				url: "/api/test",
				interval: 1000,
				maxInterval: 5000,
			});

			poller.consecutiveFailures = 10; // Would be 512000ms without cap
			expect(poller.calculateBackoffInterval()).toBe(5000);
		});
	});

	describe("emitEvent", () => {
		it("should emit custom event on element", () => {
			const element = document.createElement("div");
			const poller = new Poller({ url: "/api/test", element });

			let eventFired = false;
			let eventDetail = null;

			element.addEventListener("poll:data", (e) => {
				eventFired = true;
				eventDetail = e.detail;
			});

			poller.emitEvent("poll:data", { status: "running" });

			expect(eventFired).toBe(true);
			expect(eventDetail).toEqual({ status: "running" });
		});

		it("should emit error event with detailed payload", async () => {
			fetchMock.mockImplementation(() =>
				Promise.resolve({
					ok: false,
					status: 503,
				}),
			);

			const element = document.createElement("div");
			const poller = new Poller({ url: "/api/test", element });

			let errorDetail = null;
			element.addEventListener("poll:error", (e) => {
				errorDetail = e.detail;
			});

			poller.controller = new AbortController();
			poller.stopped = false; // Not stopped
			await poller.tick();

			expect(errorDetail).not.toBe(null);
			expect(errorDetail.name).toBe("HTTPError");
			expect(errorDetail.message).toBe("HTTP 503");
			expect(errorDetail.status).toBe(503);
			expect(errorDetail.attempts).toBe(1);
		});

		it("should not throw if element is null", () => {
			const poller = new Poller({ url: "/api/test", element: null });

			expect(() => poller.emitEvent("poll:data", {})).not.toThrow();
		});
	});

	describe("fetchWithTimeout", () => {
		it("should fetch successfully within timeout", async () => {
			fetchMock.mockImplementation(() =>
				Promise.resolve({
					ok: true,
					json: () => Promise.resolve({ data: "test" }),
				}),
			);

			const poller = new Poller({ url: "/api/test" });
			const response = await poller.fetchWithTimeout("/api/test", {}, 5000);

			expect(response.ok).toBe(true);
		});

		it("should throw TimeoutError if request takes too long", async () => {
			fetchMock.mockImplementation((_url, options) => {
				// Simulate a slow request that respects abort signal
				return new Promise((resolve, reject) => {
					const timeoutId = setTimeout(() => resolve({ ok: true }), 10000);
					if (options.signal) {
						options.signal.addEventListener("abort", () => {
							clearTimeout(timeoutId);
							const err = new Error("The operation was aborted");
							err.name = "AbortError";
							reject(err);
						});
					}
				});
			});

			const poller = new Poller({ url: "/api/test" });

			try {
				await poller.fetchWithTimeout("/api/test", {}, 100);
				expect(true).toBe(false); // Should not reach here
			} catch (error) {
				expect(error.name).toBe("TimeoutError");
				expect(error.message).toBe("Request timeout");
			}
		});

		it("should propagate AbortError from outer signal", async () => {
			fetchMock.mockImplementation((_url, options) => {
				// Simulate a request that respects abort signal
				return new Promise((resolve, reject) => {
					const timeoutId = setTimeout(() => resolve({ ok: true }), 1000);
					if (options.signal) {
						if (options.signal.aborted) {
							clearTimeout(timeoutId);
							const err = new Error("The operation was aborted");
							err.name = "AbortError";
							reject(err);
							return;
						}
						options.signal.addEventListener("abort", () => {
							clearTimeout(timeoutId);
							const err = new Error("The operation was aborted");
							err.name = "AbortError";
							reject(err);
						});
					}
				});
			});

			const poller = new Poller({ url: "/api/test" });
			const outerController = new AbortController();

			// Abort immediately
			outerController.abort();

			try {
				await poller.fetchWithTimeout("/api/test", { signal: outerController.signal }, 5000);
				expect(true).toBe(false); // Should not reach here
			} catch (error) {
				// Should propagate the AbortError, not TimeoutError
				expect(error.name).toBe("AbortError");
			}
		});

		it("should abort fetch on timeout", async () => {
			let abortCalled = false;
			fetchMock.mockImplementation((_url, options) => {
				// Simulate a request that respects abort signal
				return new Promise((resolve, reject) => {
					const timeoutId = setTimeout(() => resolve({ ok: true }), 10000);
					if (options.signal) {
						options.signal.addEventListener("abort", () => {
							abortCalled = true;
							clearTimeout(timeoutId);
							const err = new Error("The operation was aborted");
							err.name = "AbortError";
							reject(err);
						});
					}
				});
			});

			const poller = new Poller({ url: "/api/test" });

			try {
				await poller.fetchWithTimeout("/api/test", {}, 50);
			} catch (error) {
				// Expect TimeoutError
				expect(error.name).toBe("TimeoutError");
			}

			expect(abortCalled).toBe(true);
		});
	});
});
