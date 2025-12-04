/**
 * Types for Puppeteer Worker
 * Maintains compatibility with downstream data contracts
 */

import type { CDPSession, Page } from "puppeteer";

// ============================================================================
// PRESERVED DATA CONTRACTS (must match downstream expectations)
// ============================================================================

export interface SelfContainedEvent {
	readonly id: string;
	readonly method: string;
	readonly params: {
		readonly timestamp: number;
		readonly sessionId: string;
		readonly attribution: AttributionContext;
		readonly payload: EventPayload;
		readonly [key: string]: unknown;
	};
	readonly metadata: EventMetadata;
}

export interface AttributionContext {
	readonly url?: string;
	readonly referrer?: string;
	readonly userAgent?: string;
	readonly origin?: string;
	readonly frameDepth?: number;
	readonly stackTrace?: StackTraceFrame[];
}

export interface StackTraceFrame {
	readonly functionName?: string;
	readonly url?: string;
	readonly lineNumber?: number;
	readonly columnNumber?: number;
}

export interface EventMetadata {
	readonly category:
		| "network"
		| "security"
		| "runtime"
		| "page"
		| "dom"
		| "storage";
	readonly tags: string[];
	readonly processingHints: ProcessingHints;
	readonly correlationId?: string;
	readonly sequenceNumber: number;
}

export interface ProcessingHints {
	readonly requiresStaticAnalysis?: boolean;
	readonly containsSensitiveData?: boolean;
	readonly isHighPriority?: boolean;
	readonly suggestedTimeout?: number;
}

export type EventPayload =
	| NetworkEventPayload
	| SecurityEventPayload
	| RuntimeEventPayload
	| StorageEventPayload
	| WorkerScreenshotPayload
	| WorkerLogPayload
	| WorkerJobFailurePayload;

export interface NetworkEventPayload {
	readonly url: string;
	readonly requestId?: string;
	readonly method?: string;
	readonly headers?: Record<string, string>;
	readonly resourceType?: string;
	readonly initiatingPage?: string;
	readonly status?: number;
	readonly hasBody?: boolean;
	readonly bodyType?: string;
	readonly hasData?: boolean;
	readonly dataType?: string;
	readonly capturedFile?: EmbeddedFileContext;
}

export interface SecurityEventPayload {
	readonly eventType: string;

	readonly description?: string;
	readonly evidence?: Record<string, unknown>;
	readonly mitigationSuggestion?: string;
}

export interface RuntimeEventPayload {
	readonly eventType: string;
	readonly level?: "log" | "warn" | "error" | "debug";
	readonly message?: string;
	readonly source?: string;
	readonly lineNumber?: number;
	readonly columnNumber?: number;
}

export interface StorageEventPayload {
	readonly operation: "read" | "write" | "delete" | "clear";
	readonly storageType: "localStorage" | "sessionStorage";
	readonly key?: string;
	readonly value?: string;
	readonly valueSize?: number;
}

export interface WorkerScreenshotPayload {
	readonly type?: "screenshot";
	readonly image: string; // base64 (png preferred)
	readonly mime?: string; // e.g., image/png
	readonly width?: number;
	readonly height?: number;
	readonly size?: number; // bytes
	readonly caption?: string;
}

export interface WorkerLogPayload {
	readonly type?: "workerLog" | "log";
	readonly message: string;
}

export interface WorkerJobFailurePayload {
	readonly type: "jobFailure";
	readonly errorType: string; // e.g., "TimeoutError", "NavigationError", "SelectorNotFound", "ScriptError"
	readonly errorMessage: string; // Sanitized error message (no full stack traces)
	readonly failedStep?: string; // Description of which step/action failed (e.g., "page.goto", "page.click")
	readonly selector?: string; // CSS selector if failure was selector-related
	readonly url?: string; // Current page URL at time of failure
	readonly timestamp: number; // Failure timestamp (milliseconds since epoch)
	readonly screenshot?: string; // Base64-encoded PNG screenshot at point of failure (optional, size-conscious)
	readonly screenshotMime?: string; // MIME type of screenshot (e.g., "image/png")
	readonly stackTrace?: string; // Sanitized stack trace (first few lines only, no sensitive paths)
	readonly context?: Record<string, unknown>; // Additional context for LLM-driven analysis (future use)
}

export interface EmbeddedFileContext {
	readonly fileId: string;
	readonly originalUrl: string;
	readonly contentType: string;
	readonly size: number;
	readonly hash: string;
	readonly captureTimestamp: number;
	readonly storageProvider: string;
	readonly storageKey: string;
	readonly metadata?: FileMetadata;
}

export interface FileMetadata {
	readonly encoding?: string;
	readonly isCompressed?: boolean;
	readonly originalSize?: number;
	readonly captureReason: string;
	readonly sessionId: string;
}

export interface EventBatch {
	readonly batchId: string;
	readonly sessionId: string;
	readonly events: SelfContainedEvent[];
	readonly batchMetadata: BatchMetadata;
	readonly sequenceInfo: BatchSequenceInfo;
}

export interface BatchMetadata {
	readonly createdAt: number;
	readonly eventCount: number;
	readonly totalSize: number;
	readonly compressionInfo?: CompressionInfo;
	readonly checksumInfo: ChecksumInfo;
	readonly retryCount: number;
	readonly processingDeadline?: number;
	readonly jobId?: string;
}

export interface BatchSequenceInfo {
	readonly sequenceNumber: number;
	readonly isFirstBatch: boolean;
	readonly isLastBatch: boolean;
	readonly totalBatches?: number;
}

export interface CompressionInfo {
	readonly algorithm: string;
	readonly originalSize: number;
	readonly compressedSize: number;
	readonly ratio: number;
}

export interface ChecksumInfo {
	readonly algorithm: string;
	readonly value: string;
}

// ============================================================================
// INTERNAL TYPES
// ============================================================================

export interface Config {
	readonly headless?: boolean;
	readonly timeout?: number;
	readonly fileCapture?: FileCaptureConfig;
	readonly shipping?: ShippingConfig;
	readonly clientMonitoring?: ClientMonitoringConfig;
	readonly worker?: WorkerConfig;
	// Schemaless Puppeteer launch options (YAML only; not bound to env)
	readonly launch?: Record<string, unknown>;
}

export interface FileCaptureConfig {
	readonly enabled: boolean;
	readonly types: string[];
	/**
	 * Optional list of substrings to match against the Content-Type header.
	 * If any matcher is found in the header, the file will be eligible for capture
	 * even if the derived resource type is not included in `types`.
	 */
	readonly contentTypeMatchers?: string[];
	readonly maxFileSize?: number;
	readonly storage: "memory" | "redis" | "cloud";
	readonly storageConfig?: Record<string, unknown>;
}

export interface ShippingConfig {
	readonly endpoint?: string;
	readonly batchSize?: number;
	readonly maxBatchAge?: number;
	readonly sourceJobId?: string;
}

export interface ClientMonitoringConfig {
	readonly enabled: boolean;
	readonly events: string[];
}

export interface WorkerConfig {
	readonly apiBaseUrl?: string;
	readonly jobType: "browser" | "rules";
	readonly leaseSeconds: number;
	readonly waitSeconds: number;
	readonly heartbeatSeconds: number;
}

export interface ExecutionResult {
	readonly sessionId: string;
	readonly success: boolean;
	readonly error?: string;
	readonly executionTime: number;
	readonly eventCount: number;
	readonly fileCount: number;
}

// Event callback type for internal use
export type EventCallback = (event: SelfContainedEvent) => void;

// CDP event handler context
export interface CDPEventContext {
	readonly page: Page;
	readonly cdpSession: CDPSession;
	readonly sessionId: string;
	readonly eventCallback: EventCallback;
	readonly config: Config;
}
