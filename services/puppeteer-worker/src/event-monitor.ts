/**
 * Event Monitor
 * Direct CDP event capture without service layer complexity
 */

import { randomUUID } from "node:crypto";
import type { ConsoleMessage } from "puppeteer";
import type {
	AttributionContext,
	CDPEventContext,
	EventPayload,
	EventMetadata,
	NetworkEventPayload,
	RuntimeEventPayload,
	SecurityEventPayload,
	SelfContainedEvent,
} from "./types.js";

export class EventMonitor {
	private sequenceNumber = 0;
	private isActive = false;

	constructor(private readonly context: CDPEventContext) {}

	async initialize(): Promise<void> {
		if (this.isActive) return;

		const { page, cdpSession } = this.context;

		// Enable CDP domains
		await cdpSession.send("Network.enable");
		await cdpSession.send("Runtime.enable");
		await cdpSession.send("Security.enable");

		// Network event handlers
		cdpSession.on(
			"Network.requestWillBeSent",
			this.handleNetworkRequest.bind(this),
		);
		cdpSession.on(
			"Network.responseReceived",
			this.handleNetworkResponse.bind(this),
		);
		cdpSession.on("Network.loadingFailed", this.handleNetworkError.bind(this));

		// Runtime event handlers
		cdpSession.on(
			"Runtime.consoleAPICalled",
			this.handleConsoleMessage.bind(this),
		);
		cdpSession.on(
			"Runtime.exceptionThrown",
			this.handleRuntimeException.bind(this),
		);

		// Security event handlers
		cdpSession.on(
			"Security.securityStateChanged",
			this.handleSecurityStateChange.bind(this),
		);

		// Page event handlers
		page.on("console", this.handlePageConsole.bind(this));
		page.on("pageerror", this.handlePageError.bind(this));

		this.isActive = true;
	}

	async cleanup(): Promise<void> {
		if (!this.isActive) return;

		const { page, cdpSession } = this.context;

		// Remove all listeners
		cdpSession.removeAllListeners();
		page.removeAllListeners("console");
		page.removeAllListeners("pageerror");

		this.isActive = false;
	}

	private async handleNetworkRequest(params: unknown): Promise<void> {
		const p = params as {
			request?: {
				url?: string;
				method?: string;
				headers?: Record<string, string>;
				postData?: unknown;
			};
			type?: string;
			documentURL?: string;
			requestId?: string;
		};

		if (!p?.request?.url || !p?.request?.method || !p?.request?.headers) {
			return;
		}

		const payload: NetworkEventPayload = {
			url: p.request.url,
			requestId: p.requestId,
			method: p.request.method,
			headers: p.request.headers,
			resourceType: p.type,
			initiatingPage: p.documentURL,
			hasData: !!p.request.postData,
			dataType:
				typeof p.request.headers["content-type"] === "string"
					? p.request.headers["content-type"]
					: undefined,
		};

		await this.emitEvent("Network.requestWillBeSent", payload, {
			category: "network",
			tags: ["network", "request"],
		});
	}

	private handleNetworkResponse(params: {
		requestId?: string;
		response: { url: string; status: number; headers: Record<string, string> };
		type?: string;
	}): void {
		const payload: NetworkEventPayload = {
			url: params.response.url,
			requestId: params.requestId,
			status: params.response.status,
			headers: params.response.headers,
			resourceType: params.type,
			hasBody: params.response.status >= 200 && params.response.status < 300,
			bodyType: params.response.headers["content-type"],
		};

		this.emitEvent("Network.responseReceived", payload, {
			category: "network",
			tags: ["network", "response"],
			processingHints: {
				requiresStaticAnalysis: this.shouldAnalyzeResource(params.type),
				isHighPriority: params.response.status >= 400,
			},
		});
	}

	private handleNetworkError(params: unknown): void {
		const record = asRecord(params);
		const request = record?.request && asRecord(record.request);
		const url =
			typeof request?.url === "string" && request.url.length > 0
				? request.url
				: "unknown";
		const resourceType =
			typeof record?.type === "string" ? record.type : undefined;

		const payload: NetworkEventPayload = {
			url,
			resourceType,
		};

		void this.emitEvent("Network.loadingFailed", payload, {
			category: "network",
			tags: ["network", "error"],
			processingHints: {
				isHighPriority: true,
			},
		});
	}

	private handleConsoleMessage(params: unknown): void {
		const record = asRecord(params);
		const args = Array.isArray(record?.args) ? record?.args : [];
		const message = args
			.map((arg) => serializeConsoleArg(arg))
			.filter((value) => value.length > 0)
			.join(" ");
		const levelSource = typeof record?.type === "string" ? record.type : "log";
		const payload: RuntimeEventPayload = {
			eventType: "console",
			level: this.mapConsoleLevel(levelSource),
			message,
		};

		void this.emitEvent("Runtime.consoleAPICalled", payload, {
			category: "runtime",
			tags: ["console", levelSource],
		});
	}

	private handleRuntimeException(params: unknown): void {
		const record = asRecord(params);
		const details =
			record?.exceptionDetails && asRecord(record.exceptionDetails);
		const message =
			typeof details?.text === "string" ? details.text : "Unknown exception";
		const payload: RuntimeEventPayload = {
			eventType: "exception",
			level: "error",
			message,
			source:
				typeof details?.url === "string" && details.url.length > 0
					? details.url
					: undefined,
			lineNumber: asNumber(details?.lineNumber),
			columnNumber: asNumber(details?.columnNumber),
		};

		void this.emitEvent("Runtime.exceptionThrown", payload, {
			category: "runtime",
			tags: ["exception", "error"],
			processingHints: {
				isHighPriority: true,
			},
		});
	}

	private handleSecurityStateChange(params: unknown): void {
		const record = asRecord(params);
		const securityState =
			typeof record?.securityState === "string"
				? record.securityState
				: "unknown";
		const payload: SecurityEventPayload = {
			eventType: "securityStateChanged",
			description: `Security state changed to: ${securityState}`,
			evidence: {
				securityState,
				explanations: record?.explanations,
			},
		};

		void this.emitEvent("Security.securityStateChanged", payload, {
			category: "security",
			tags: ["security", "state"],
			processingHints: {
				isHighPriority: securityState === "insecure",
			},
		});
	}

	private handlePageConsole(msg: ConsoleMessage): void {
		// Handle page-level console events (complement to CDP console events)
		const payload: RuntimeEventPayload = {
			eventType: "pageConsole",
			level: this.mapConsoleLevel(msg.type()),
			message: msg.text(),
		};

		void this.emitEvent("Page.console", payload, {
			category: "page",
			tags: ["page", "console"],
			processingHints: {},
		});
	}

	private handlePageError(error: Error): void {
		const payload: RuntimeEventPayload = {
			eventType: "pageError",
			level: "error",
			message: error.message,
			source: error.stack?.split("\n")[0],
		};

		this.emitEvent("Page.error", payload, {
			category: "page",
			tags: ["page", "error"],
			processingHints: {
				isHighPriority: true,
			},
		});
	}

	private async emitEvent(
		method: string,
		payload: EventPayload,
		metadata: Partial<EventMetadata>,
	): Promise<void> {
		const event: SelfContainedEvent = {
			id: randomUUID(),
			method,
			params: {
				timestamp: Date.now(),
				sessionId: this.context.sessionId,
				attribution: await this.getAttributionContext(),
				payload,
			},
			metadata: {
				category: "runtime",
				tags: [],
				processingHints: {},
				sequenceNumber: ++this.sequenceNumber,
				...metadata,
			},
		};

		this.context.eventCallback(event);
	}

	private async getAttributionContext(): Promise<AttributionContext> {
		const page = this.context.page;
		const userAgent = await page.browser().userAgent();
		return {
			url: page.url(),
			userAgent,
		};
	}

	private shouldAnalyzeResource(resourceType: string): boolean {
		void resourceType;
		return true; // analyze all resources
	}

	private mapConsoleLevel(cdpType: string): "log" | "warn" | "error" | "debug" {
		if (cdpType === "warn" || cdpType === "error" || cdpType === "debug") {
			return cdpType;
		}
		return "log";
	}
}

function asRecord(value: unknown): Record<string, unknown> | undefined {
	if (value && typeof value === "object" && !Array.isArray(value)) {
		return value as Record<string, unknown>;
	}
	return undefined;
}

function serializeConsoleArg(arg: unknown): string {
	if (isRecord(arg) && "value" in arg) {
		return stringifyConsoleValue((arg as { value?: unknown }).value);
	}
	return stringifyConsoleValue(arg);
}

function stringifyConsoleValue(value: unknown): string {
	if (value === null || value === undefined) return "";
	if (typeof value === "string") return value;
	if (typeof value === "number" || typeof value === "boolean") {
		return String(value);
	}
	try {
		return JSON.stringify(value);
	} catch {
		return String(value);
	}
}

function isRecord(value: unknown): value is Record<string, unknown> {
	return value !== null && typeof value === "object" && !Array.isArray(value);
}

function asNumber(value: unknown): number | undefined {
	return typeof value === "number" && Number.isFinite(value)
		? value
		: undefined;
}
