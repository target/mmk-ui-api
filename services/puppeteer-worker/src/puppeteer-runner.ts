/**
 * Puppeteer Runner
 * Main orchestrator that replaces complex service architecture
 */

import { randomUUID } from "node:crypto";
import { readFile } from "node:fs/promises";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import puppeteer, {
	type Browser,
	type CDPSession,
	type Page,
	type ScreenshotOptions,
} from "puppeteer";

let __clientMonitoringCache: string | null = null;
async function loadClientMonitoringSource(): Promise<string> {
	if (__clientMonitoringCache) return __clientMonitoringCache;
	const here = dirname(fileURLToPath(import.meta.url));
	const clientMonitoringPath = resolve(here, "./client-monitoring.js");
	__clientMonitoringCache = await readFile(clientMonitoringPath, "utf8");
	return __clientMonitoringCache;
}

import { EventMonitor } from "./event-monitor.js";
import { EventShipper } from "./event-shipper.js";
import { type CapturedFile, FileCapture } from "./file-capture.js";
import { logger, pageLogger } from "./logger.js";
import type {
	Config,
	ExecutionResult,
	EventPayload,
	NetworkEventPayload,
	SelfContainedEvent,
} from "./types.js";

import {
	type ClientEvent,
	mapClientEventToMethod,
	mapClientEventPayload,
	getClientEventCategory,
} from "./client-event-map.js";

type MonitoringGlobal = {
	__puppeteerMonitoringActive?: boolean;
	__puppeteerMonitoring?: {
		getEventCount(): number;
		getEvents(): ClientEvent[];
	};
};

type ScriptHarnessResult =
	| { ok: true; value: unknown }
	| {
			ok: false;
			error?: ScriptHarnessError;
	  };

type ScriptHarnessError = {
	name?: string;
	message?: string;
	stack?: string;
	isDomException?: boolean;
	source?: string;
};

export class PuppeteerRunner {
	private browser?: Browser;
	private page?: Page;
	private cdpSession?: CDPSession;
	private eventMonitor?: EventMonitor;
	private fileCapture?: FileCapture;
	private eventShipper?: EventShipper;
	private sessionId: string;
	private events: SelfContainedEvent[] = [];
	private screenshotCount = 0;

	private responseEventIndexByRequestId: Map<string, number> = new Map();

	// Streaming event shipping state
	private flushTimer?: NodeJS.Timeout;
	private lastFlushTime = 0;
	private isShipping = false;
	private shippingConfig?: Config["shipping"];

	constructor() {
		this.sessionId = randomUUID();
	}

	static async runWithConfig(script: string): Promise<ExecutionResult> {
		const { loadConfig } = await import("./config-loader.js");
		const cfg = await loadConfig();
		const runner = new PuppeteerRunner();
		return runner.runScript(script, cfg);
	}

	async runScript(
		script: string,
		config: Config = {},
		signal?: AbortSignal,
	): Promise<ExecutionResult> {
		// Intentionally unused; kept for API continuity and future cancellation
		void signal;
		const startTime = Date.now();

		try {
			await this.initialize(config);
			await this.executeScript(script);
			await this.collectClientEvents();

			const result: ExecutionResult = {
				sessionId: this.sessionId,
				success: true,
				executionTime: Date.now() - startTime,
				eventCount: this.events.length,
				fileCount: this.fileCapture?.getStats().totalFiles || 0,
			};

			return result;
		} catch (error) {
			// Capture failure context and emit failure event
			logger.error(error, "Script execution failed", {
				sessionId: this.sessionId,
				errorType:
					error instanceof Error ? error.constructor.name : typeof error,
				failedStep:
					error instanceof Error && "cause" in error
						? String(error.cause)
						: undefined,
			});
			await this.handleScriptFailure(error, script);

			return {
				sessionId: this.sessionId,
				success: false,
				error: error instanceof Error ? error.message : String(error),
				executionTime: Date.now() - startTime,
				eventCount: this.events.length,
				fileCount: 0,
			};
		} finally {
			// Final flush of any remaining events (including failure events)
			if (config.shipping?.endpoint) {
				try {
					// Wait for any in-flight flush to complete
					while (this.isShipping) {
						await new Promise((resolve) => setTimeout(resolve, 25));
					}
					// Attempt final flush if events remain
					await this.flushEvents("final");
				} catch (shipError) {
					logger.error(shipError, "Failed to ship events during final flush", {
						sessionId: this.sessionId,
						eventCount: this.events.length,
					});
				}
			}
			await this.cleanup();
		}
	}

	private async initialize(config: Config): Promise<void> {
		// Launch browser with optional schemaless overrides from config.launch
		type LaunchOpts = NonNullable<Parameters<typeof puppeteer.launch>[0]>;
		const provided = config.launch as Partial<LaunchOpts> | undefined;
		const opts: LaunchOpts = { ...(provided ?? {}) };

		// Apply safe defaults and coercions
		if (typeof opts.headless !== "boolean") {
			opts.headless = config.headless !== false;
		}

		// Coerce args from string to array if needed, then ensure a default
		const rawArgs = (opts as { args?: unknown }).args;
		if (typeof rawArgs === "string") {
			opts.args = rawArgs
				.split(",")
				.map((s: string) => s.trim())
				.filter(Boolean);
		}
		if (!Array.isArray(opts.args)) {
			opts.args = ["--disable-web-security", "--no-sandbox"];
		}

		this.browser = await puppeteer.launch(opts);

		// Create page and CDP session
		this.page = await this.browser.newPage();

		// Set default timeout for all Puppeteer operations (30 seconds)
		// This ensures selectors that don't exist will timeout and throw errors
		this.page.setDefaultTimeout(30000);
		// Align navigation timeout with default operation timeout for consistent behavior
		this.page.setDefaultNavigationTimeout(30000);

		this.cdpSession = await this.page.createCDPSession();

		// Log browser console consistently at info; include original type for context
		this.page.on("console", (msg) => {
			const t = msg.type() as string;
			const text = msg.text();
			const loc = msg.location();
			const ctx = {
				source: "browser_console",
				consoleType: t,
				pageUrl: this.page?.url(),
				...(loc && { location: loc }),
				consoleText: text,
			};
			switch (t) {
				case "error":
				case "assert":
					pageLogger.error(
						{ name: "BrowserConsoleError", message: text },
						"[Page] console error",
						ctx,
					);
					break;
				case "warning":
					pageLogger.warn("[Page] console warn", ctx);
					break;
				case "debug":
				case "trace":
				case "timeEnd":
					pageLogger.debug("[Page] console debug", ctx);
					break;
				default:
					pageLogger.info("[Page] console", ctx);
			}
		});

		this.page.on("pageerror", (error) => {
			pageLogger.error(error, "[Page] runtime error", {
				source: "browser_pageerror",
				pageUrl: this.page?.url(),
			});
		});

		// Initialize file capture
		if (config.fileCapture?.enabled) {
			this.fileCapture = new FileCapture(config.fileCapture, this.sessionId);
		}

		// Initialize event shipper
		if (config.shipping?.endpoint) {
			this.eventShipper = new EventShipper(config.shipping);
		}

		// Initialize event monitor
		this.eventMonitor = new EventMonitor({
			page: this.page,
			cdpSession: this.cdpSession,
			sessionId: this.sessionId,
			eventCallback: this.handleEvent.bind(this),
			config,
		});

		await this.eventMonitor.initialize();

		// Inject client-side monitoring if enabled
		if (config.clientMonitoring?.enabled !== false) {
			await this.injectClientMonitoring();
		}

		// Set up network response interception for file capture
		if (this.fileCapture) {
			await this.setupFileCapture();
		}

		// Start streaming event shipping
		this.startEventStreaming(config);
	}

	private async executeScript(script: string): Promise<void> {
		if (!this.page) {
			throw new Error("Page not initialized");
		}

		// Check if script contains Puppeteer page operations or uses helpers that require Node-side mode
		if (script.includes("page.") || script.includes("screenshot(")) {
			// Execute as Node.js code with access to the page object
			const AsyncFunction = Object.getPrototypeOf(async () => {}).constructor;
			const scriptFunction = new AsyncFunction(
				"page",
				"screenshot",
				"log",
				script,
			);
			// Pass a closure that does not rely on dynamic `this` binding to avoid context issues
			const pageRef = this.page;
			const logScripts =
				process.env.WORKER_LOG_SCRIPTS === "1" ||
				process.env.WORKER_LOG_SCRIPTS === "true";
			if (logScripts) {
				logger.debug("Executing script with page operations", {
					sessionId: this.sessionId,
					scriptPreview: script.substring(0, 100),
				});
			} else {
				logger.debug("Executing script with page operations", {
					sessionId: this.sessionId,
				});
			}
			await this.executeNodeSideScript(scriptFunction, pageRef);
		} else {
			await this.executeBrowserSideScript(script);
		}

		// Re-inject client monitoring after script execution (in case page content changed)
		await this.reinjectClientMonitoring();
	}

	private async executeNodeSideScript(
		scriptFunction: (
			page: Page,
			screenshot: (
				opts?: ScreenshotOptions & { encoding?: "base64" },
			) => Promise<void>,
			log: (message: unknown) => void,
		) => Promise<unknown>,
		pageRef: Page,
	): Promise<void> {
		try {
			await scriptFunction(
				pageRef,
				async (opts: ScreenshotOptions & { encoding?: "base64" } = {}) => {
					// Cap at 25 screenshots per job using increment-then-check to avoid overruns
					const next = ++this.screenshotCount;
					if (next > 25) {
						this.screenshotCount = 25; // clamp
						return;
					}
					try {
						if (!pageRef) throw new Error("page not initialized");
						const imageBase64: string = await pageRef.screenshot({
							...opts,
							type: "png",
							encoding: "base64",
						});
						this.emitInternalEvent(
							"Worker.screenshot",
							{ type: "screenshot", image: imageBase64, mime: "image/png" },
							["worker", "screenshot"],
						);
					} catch (err) {
						const msg = err instanceof Error ? err.message : String(err);
						this.emitInternalEvent(
							"Worker.log",
							{
								type: "workerLog",
								message: stackMessage("screenshot failed", msg, err),
							},
							["worker", "log"],
						);
					}
				},
				this.helperLog.bind(this),
			);
		} catch (error) {
			logger.error(error, "Node-side script execution error", {
				sessionId: this.sessionId,
				pageUrl: this.page?.url(),
			});

			throw this.wrapScriptError(error, "node-script");
		}
	}

	private async executeBrowserSideScript(script: string): Promise<void> {
		if (!this.page) {
			throw new Error("Page not initialized");
		}

		const harness = this.buildBrowserHarness();
		let result: ScriptHarnessResult;
		try {
			result = (await this.page.evaluate(
				harness,
				script,
			)) as ScriptHarnessResult;
		} catch (error) {
			logger.warn("page.evaluate threw before harness could handle error", {
				sessionId: this.sessionId,
				err: error instanceof Error ? error.message : String(error),
			});
			throw this.wrapScriptError(error, "page.evaluate");
		}

		if (!result || typeof result !== "object" || !("ok" in result)) {
			const unexpectedResult: unknown = result;
			const resultType = typeof unexpectedResult;
			const preview = (() => {
				if (unexpectedResult === null) return "null";
				if (resultType === "string")
					return (unexpectedResult as string).slice(0, 120);
				try {
					const json = JSON.stringify(unexpectedResult);
					return typeof json === "string" && json.length > 0
						? json.slice(0, 120)
						: resultType;
				} catch {
					return resultType;
				}
			})();
			throw this.wrapScriptError(
				new Error(
					`Script returned unexpected result shape: type=${resultType}; preview=${preview}`,
				),
				"page-context",
			);
		}

		if (this.isHarnessError(result)) {
			const err = this.restoreHarnessError(result.error);
			this.emitInternalEvent(
				"Worker.log",
				{
					type: "workerLog",
					message: `Captured script error in page context: ${err.name}: ${err.message}`,
				},
				["worker", "log", "script-error"],
			);
			logger.debug("Browser-side script error captured", {
				sessionId: this.sessionId,
				errorName: err.name,
				errorMessage: err.message,
				errorStack: err.stack,
				pageUrl: this.page.url(),
			});
			throw this.wrapScriptError(err, result.error?.source ?? "page-context");
		}
	}

	private buildBrowserHarness(): (
		script: string,
	) => Promise<ScriptHarnessResult> {
		return (scriptSource: string): Promise<ScriptHarnessResult> => {
			// Construct an async function from the provided script source to avoid
			// injecting it into a template string where backticks/escapes could break syntax.
			const AsyncFunction = Object.getPrototypeOf(async () => {})
				.constructor as new (
				...args: string[]
			) => () => Promise<unknown>;
			const __puppeteerWorkerScript = new AsyncFunction(scriptSource);

			const toPlainError = (error: unknown): ScriptHarnessError => {
				const name =
					error && typeof (error as { name?: unknown }).name === "string"
						? (error as { name: string }).name
						: undefined;
				const message =
					error && typeof (error as { message?: unknown }).message === "string"
						? (error as { message: string }).message
						: typeof error === "string"
							? error
							: "Unknown page error";
				const stack =
					error && typeof (error as { stack?: unknown }).stack === "string"
						? (error as { stack: string }).stack
								.split("\n")
								.slice(0, 6)
								.join("\n")
						: undefined;
				const isDomException =
					typeof DOMException !== "undefined" && error instanceof DOMException;

				return {
					name: name || (isDomException ? "DOMException" : "PageScriptError"),
					message,
					stack,
					isDomException,
					source: "page-context",
				};
			};

			return Promise.resolve()
				.then(() => __puppeteerWorkerScript())
				.then((value) => ({ ok: true, value }))
				.catch((error) => ({ ok: false, error: toPlainError(error) }));
		};
	}

	private restoreHarnessError(error?: ScriptHarnessError): Error {
		const message =
			typeof error?.message === "string" && error.message.length > 0
				? error.message
				: "Unknown page error";
		const err = new Error(message);
		err.name = error?.name || "PageScriptError";
		if (typeof error?.stack === "string") {
			err.stack = `${err.name}: ${message}\n${error.stack}`;
		}
		return err;
	}

	private isHarnessError(
		result: ScriptHarnessResult,
	): result is Extract<ScriptHarnessResult, { ok: false }> {
		return result.ok === false;
	}

	private wrapScriptError(error: unknown, phase: string): Error {
		if (error instanceof Error) {
			const causeValue = (error as { cause?: unknown }).cause;
			if (causeValue === undefined) {
				Object.assign(error, { cause: phase });
			}
			logger.debug("Wrapped script error", {
				sessionId: this.sessionId,
				phase,
				errorName: error.name,
				errorMessage: error.message,
			});
			return error;
		}

		const err = new Error(
			typeof error === "string"
				? error
				: (() => {
						try {
							return JSON.stringify(error);
						} catch {
							return String(error);
						}
					})(),
		);
		err.name = "ScriptExecutionError";
		Object.assign(err, { cause: phase });
		logger.debug("Wrapped non-Error script failure", {
			sessionId: this.sessionId,
			phase,
			valueType: typeof error,
		});
		return err;
	}

	private async reinjectClientMonitoring(): Promise<void> {
		if (!this.page) return;

		try {
			// Check if monitoring is already active
			const isActive = await this.page.evaluate(() => {
				const globalObj = window as MonitoringGlobal;
				return globalObj.__puppeteerMonitoringActive ?? false;
			});

			if (!isActive) {
				const cm = await loadClientMonitoringSource();
				await this.page.evaluate(cm);
				logger.info("Client monitoring re-injected after script execution");
			}
		} catch (error) {
			logger.warn("Failed to re-inject client monitoring", { err: error });
		}
	}

	private async collectClientEvents(): Promise<void> {
		if (!this.page) return;

		try {
			// Wait a bit for client events to be collected
			await new Promise((resolve) => setTimeout(resolve, 100));

			// Check if monitoring is active
			const monitoringStatus = await this.page.evaluate(() => {
				const globalObj = window as MonitoringGlobal;
				const monitoring = globalObj.__puppeteerMonitoring;
				return {
					active: globalObj.__puppeteerMonitoringActive ?? false,
					hasMonitoring: Boolean(monitoring),
					eventCount: monitoring?.getEventCount() ?? 0,
					events: monitoring?.getEvents() ?? [],
				};
			});

			logger.info("Client monitoring status", { monitoringStatus });

			// Convert client events to SelfContainedEvent format
			for (const clientEvent of monitoringStatus.events) {
				this.handleClientEvent(clientEvent);
			}
		} catch (error) {
			logger.warn("Failed to collect client events", { err: error });
		}
	}

	private handleEvent(event: SelfContainedEvent): void {
		this.events.push(event);
		if (event.method === "Network.responseReceived") {
			const p = event.params.payload as NetworkEventPayload;
			const rid = p?.requestId;
			if (typeof rid === "string") {
				this.responseEventIndexByRequestId.set(rid, this.events.length - 1);
			}
		}

		// Trigger batch-size flush if needed (non-blocking)
		void this.maybeFlushEvents("batch-size");
	}
	private emitInternalEvent(
		method: string,
		payload: EventPayload,
		tags: string[] = [],
	): void {
		const event: SelfContainedEvent = {
			id: randomUUID(),
			method,
			params: {
				timestamp: Date.now(),
				sessionId: this.sessionId,
				attribution: {
					url: this.page?.url(),
					userAgent: "puppeteer-worker",
				},
				payload,
			},
			metadata: {
				category: "runtime",
				tags,
				processingHints: {},
				sequenceNumber: this.events.length + 1,
			},
		};
		this.handleEvent(event);
	}

	private helperLog(message: unknown): void {
		let msg: string;
		if (typeof message === "string") msg = message;
		else {
			try {
				msg = JSON.stringify(message);
			} catch {
				msg = String(message);
			}
		}
		this.emitInternalEvent("Worker.log", { type: "workerLog", message: msg }, [
			"worker",
			"log",
		]);
	}

	/**
	 * Handle script execution failure by capturing context and emitting a failure event.
	 */
	private async handleScriptFailure(
		error: unknown,
		script: string,
	): Promise<void> {
		try {
			const errorInfo = this.extractErrorInfo(error, script);
			const screenshot = await this.captureFailureScreenshot();

			const failurePayload: import("./types.js").WorkerJobFailurePayload = {
				type: "jobFailure",
				errorType: errorInfo.errorType,
				errorMessage: errorInfo.errorMessage,
				failedStep: errorInfo.failedStep,
				selector: errorInfo.selector,
				url: this.page?.url(),
				timestamp: Date.now(),
				screenshot: screenshot?.image,
				screenshotMime: screenshot?.mime,
				stackTrace: errorInfo.stackTrace,
				context: errorInfo.context,
			};

			this.emitInternalEvent("Worker.jobFailure", failurePayload, [
				"worker",
				"failure",
				"error",
			]);

			logger.error(error, "Script execution failed", {
				sessionId: this.sessionId,
				errorType: errorInfo.errorType,
				failedStep: errorInfo.failedStep,
			});
		} catch (handlerError) {
			// Don't let failure handling itself crash the process
			logger.error(
				handlerError,
				"Failed to handle script failure (meta-error)",
				{
					sessionId: this.sessionId,
					originalError: error instanceof Error ? error.message : String(error),
				},
			);
		}
	}

	/**
	 * Extract structured error information from an error object.
	 */
	private extractErrorInfo(
		error: unknown,
		script: string,
	): {
		errorType: string;
		errorMessage: string;
		failedStep?: string;
		selector?: string;
		stackTrace?: string;
		context?: Record<string, unknown>;
	} {
		if (!(error instanceof Error)) {
			return {
				errorType: "UnknownError",
				errorMessage: String(error),
			};
		}

		// Determine error type from error name or message patterns
		let errorType = error.name || "Error";
		let failedStep: string | undefined;
		let selector: string | undefined;

		// Pattern matching for common Puppeteer errors
		const message = error.message;

		if (message.includes("Navigation timeout") || message.includes("timeout")) {
			errorType = "TimeoutError";
			failedStep = "navigation";
		} else if (
			message.includes("waiting for selector") ||
			message.includes("No node found") ||
			message.includes("No element found for selector")
		) {
			errorType = "SelectorNotFoundError";
			// Try to extract selector from error message
			const selectorMatch = message.match(/selector[:\s]+["']([^"']+)["']/i);
			if (selectorMatch) {
				selector = selectorMatch[1];
			}
			failedStep = "selector wait";
		} else if (message.includes("Navigation failed")) {
			errorType = "NavigationError";
			failedStep = "page.goto";
		} else if (message.includes("Execution context was destroyed")) {
			errorType = "ContextDestroyedError";
			failedStep = "script execution";
		} else if (message.includes("Protocol error")) {
			errorType = "ProtocolError";
		}

		// Sanitize stack trace: take first 5 lines, remove absolute paths
		let stackTrace: string | undefined;
		if (error.stack) {
			const lines = error.stack.split("\n").slice(0, 6); // Error message + 5 stack frames
			stackTrace = lines
				.map((line) => {
					// Remove absolute file paths, keep relative info
					return line.replace(/\(.*\/([^/]+:\d+:\d+)\)/, "($1)");
				})
				.join("\n");
		}

		// Sanitize error message (remove sensitive paths more conservatively)
		const sanitizedMessage = message
			.replace(/\bfile:\/\/[^\s)]+/gi, "file://...") // Redact file URLs
			.replace(/\b[A-Za-z]:\\[^\s)]+/g, "C:\\...") // Windows absolute paths
			.replace(/\b\/[^\s)]+/g, "/..."); // POSIX absolute paths

		return {
			errorType,
			errorMessage: sanitizedMessage,
			failedStep,
			selector,
			stackTrace,
			context: {
				scriptLength: script.length,
				hasPageOperations: script.includes("page."),
			},
		};
	}

	/**
	 * Attempt to capture a screenshot at the point of failure.
	 * Returns null if screenshot capture fails or browser is not in a usable state.
	 */
	private async captureFailureScreenshot(): Promise<{
		image: string;
		mime: string;
	} | null> {
		try {
			if (!this.page) {
				return null;
			}

			// Check if page is still usable
			const url = this.page.url();
			if (!url || url === "about:blank") {
				return null;
			}

			// Capture PNG screenshot with size limit (max 500KB)
			const png = await this.page.screenshot({
				type: "png",
				encoding: "base64",
				fullPage: false, // Viewport only to keep size down
			});

			// Use accurate base64 size calculation
			const pngSize = Buffer.byteLength(png, "base64");
			if (pngSize <= 500 * 1024) {
				return {
					image: png,
					mime: "image/png",
				};
			}

			// PNG too large, try JPEG fallback at quality 70
			logger.debug("PNG screenshot too large, trying JPEG fallback", {
				pngSize,
				sessionId: this.sessionId,
			});

			const jpg = await this.page.screenshot({
				type: "jpeg",
				quality: 70,
				encoding: "base64",
				fullPage: false,
			});

			const jpgSize = Buffer.byteLength(jpg, "base64");
			if (jpgSize <= 500 * 1024) {
				return {
					image: jpg,
					mime: "image/jpeg",
				};
			}

			// Both formats too large, skip screenshot
			logger.warn("Failure screenshot too large even with JPEG, skipping", {
				pngSize,
				jpgSize,
				sessionId: this.sessionId,
			});
			return null;
		} catch (screenshotError) {
			logger.warn("Failed to capture failure screenshot", {
				err: screenshotError,
				sessionId: this.sessionId,
			});
			return null;
		}
	}

	private handleClientEvent(clientEvent: ClientEvent): void {
		// Convert client event to SelfContainedEvent format
		const event: SelfContainedEvent = {
			id: randomUUID(),
			method: mapClientEventToMethod(clientEvent.type),
			params: {
				timestamp: clientEvent.timestamp,
				sessionId: this.sessionId,
				attribution: {
					url: this.page?.url(),
					userAgent: "simplified-puppeteer-worker",
				},
				payload: mapClientEventPayload(clientEvent) as unknown as EventPayload,
			},
			metadata: {
				category: getClientEventCategory(clientEvent.type),
				tags: [clientEvent.type],
				processingHints: {},
				sequenceNumber: this.events.length + 1,
			},
		};

		this.handleEvent(event);
	}

	private async injectClientMonitoring(): Promise<void> {
		if (!this.page) return;

		try {
			logger.info("Injecting client monitoring script...");

			// Inject on new documents
			const cmSrc = await loadClientMonitoringSource();
			await this.page.evaluateOnNewDocument(cmSrc);

			// Inject immediately if page is already loaded
			if (this.page.url() !== "about:blank") {
				const cm2 = await loadClientMonitoringSource();
				await this.page.evaluate(cm2);
				logger.info("Client monitoring injected into current page");
			}
		} catch (error) {
			logger.warn("Failed to inject client monitoring", { err: error });
		}
	}

	private async setupFileCapture(): Promise<void> {
		if (!this.cdpSession || !this.fileCapture) return;

		// Enable network domain for response body capture
		await this.cdpSession.send("Network.enable");

		this.cdpSession.on("Network.responseReceived", async (params) => {
			if (!this.fileCapture) return;

			try {
				// Get response body for file capture
				const session = this.cdpSession;
				if (!session) {
					return;
				}
				const response = await session.send("Network.getResponseBody", {
					requestId: params.requestId,
				});

				if (response.body) {
					const content = Buffer.from(
						response.body,
						response.base64Encoded ? "base64" : "utf8",
					);

					const capturedFile: CapturedFile = {
						url: params.response.url,
						content,
						contentType:
							params.response.headers["content-type"] ||
							"application/octet-stream",
						sessionId: this.sessionId,
					};

					const fileContext = await this.fileCapture.captureFile(capturedFile);

					// Attach file context to the exact response event via requestId when possible
					if (fileContext) {
						let targetIndex: number | undefined;
						const rid: string | undefined = params.requestId;
						if (rid) {
							const idx = this.responseEventIndexByRequestId.get(rid);
							if (idx !== undefined) {
								targetIndex = idx;
							}
						}
						// Fallback: scan backward for most recent response event
						if (targetIndex === undefined && this.events.length > 0) {
							for (let i = this.events.length - 1; i >= 0; i--) {
								if (this.events[i].method === "Network.responseReceived") {
									targetIndex = i;
									break;
								}
							}
						}
						if (targetIndex !== undefined) {
							const existing = this.events[targetIndex];
							if (existing.method === "Network.responseReceived") {
								const networkPayload = existing.params
									.payload as NetworkEventPayload;
								const updatedPayload: NetworkEventPayload = {
									...networkPayload,
									capturedFile: fileContext,
								};
								const updatedEvent: SelfContainedEvent = {
									...existing,
									params: {
										...existing.params,
										payload: updatedPayload,
									},
								};
								this.events[targetIndex] = updatedEvent;
							}
							if (rid) this.responseEventIndexByRequestId.delete(rid);
						}
					}
				}
			} catch (_error) {
				// Ignore file capture errors - they shouldn't break the main flow
			}
		});
	}

	/**
	 * Start periodic event flushing based on config
	 */
	private startEventStreaming(config: Config): void {
		this.shippingConfig = config.shipping;
		if (!this.shippingConfig?.endpoint) return;

		const flushIntervalMs = this.shippingConfig.maxBatchAge ?? 5000;
		this.lastFlushTime = Date.now();

		// Set up periodic flush timer
		this.flushTimer = setInterval(() => {
			void this.maybeFlushEvents("timer");
		}, flushIntervalMs);
	}

	/**
	 * Stop periodic event flushing
	 */
	private stopEventStreaming(): void {
		if (this.flushTimer) {
			clearInterval(this.flushTimer);
			this.flushTimer = undefined;
		}
	}

	/**
	 * Check if events should be flushed and flush if needed
	 */
	private async maybeFlushEvents(
		reason: "timer" | "batch-size" | "final",
	): Promise<void> {
		if (!this.shippingConfig?.endpoint || this.events.length === 0) return;

		const batchSize = this.shippingConfig.batchSize ?? 100;
		const maxBatchAge = this.shippingConfig.maxBatchAge ?? 5000;
		const timeSinceLastFlush = Date.now() - this.lastFlushTime;

		const shouldFlush =
			reason === "final" ||
			this.events.length >= batchSize ||
			timeSinceLastFlush >= maxBatchAge;

		if (shouldFlush) {
			await this.flushEvents(reason);
		}
	}

	/**
	 * Flush events to backend immediately
	 */
	private async flushEvents(reason: string): Promise<void> {
		if (this.isShipping || this.events.length === 0) return;

		this.isShipping = true;
		// Atomically swap buffers so new events go into fresh array during shipping
		const eventsToShip = this.events;
		this.events = [];
		const eventCount = eventsToShip.length;

		try {
			await this.eventShipper?.shipEvents(eventsToShip);
			this.lastFlushTime = Date.now();
			logger.debug("Flushed events", {
				reason,
				count: eventCount,
				sessionId: this.sessionId,
			});
		} catch (error) {
			logger.error(error, "Failed to flush events", {
				reason,
				count: eventCount,
				sessionId: this.sessionId,
			});
			// Prepend failed batch back to preserve order
			this.events = eventsToShip.concat(this.events);
		} finally {
			this.isShipping = false;
		}
	}

	private async cleanup(): Promise<void> {
		try {
			this.stopEventStreaming();
			this.responseEventIndexByRequestId.clear();
			await this.eventMonitor?.cleanup();
			await this.fileCapture?.cleanup();
			await this.page?.close();
			await this.browser?.close();
		} catch (error) {
			logger.warn("Cleanup error", { err: error });
		}
	}

	// Getter for events (useful for testing)
	getEvents(): SelfContainedEvent[] {
		return [...this.events];
	}
}

function stackMessage(prefix: string, message: string, error: unknown): string {
	const segments = [prefix, message].filter(Boolean);
	if (error instanceof Error && error.stack) {
		segments.push(error.stack);
	}
	return segments.join(": ");
}
