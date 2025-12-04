/**
 * A zero-dependency, high-performance, structured JSON logger
 *
 */
import os, { EOL } from "node:os";
import process from "node:process";

// Type definitions
interface LoggerOptions {
	stream?: NodeJS.WritableStream;
	errorStream?: NodeJS.WritableStream;

	level?: LogLevel;
	redact?: string[];
	pretty?: boolean;
	name?: string;
	splitErrorStream?: boolean; // if true, error/fatal go to stderr
}

interface LogObject {
	[key: string]: unknown;
}

interface ErrorLike {
	message: string;
	stack?: string;
	name?: string;
	code?: string | number;
	[key: string]: unknown;
}

type LogLevel = "trace" | "debug" | "info" | "warn" | "error" | "fatal";

// ANSI escape codes for pretty printing
const colors = {
	reset: "\x1b[0m",
	dim: "\x1b[2m",
	red: "\x1b[31m",
	green: "\x1b[32m",
	yellow: "\x1b[33m",
	blue: "\x1b[34m",
	magenta: "\x1b[35m",
	cyan: "\x1b[36m",
} as const;

const levels = {
	trace: 10,
	debug: 20,
	info: 30,
	warn: 40,
	error: 50,
	fatal: 60,
} as const;

const levelNames: Record<number, string> = {
	10: "TRACE",
	20: "DEBUG",
	30: "INFO",
	40: "WARN",
	50: "ERROR",
	60: "FATAL",
};

const levelColors: Record<number, string> = {
	10: colors.blue,
	20: colors.cyan,
	30: colors.green,
	40: colors.yellow,
	50: colors.red,
	60: colors.red,
};

const levelNamesByValue = Object.fromEntries(
	Object.entries(levels).map(([key, value]) => [value, key]),
) as Record<number, LogLevel>;

// Cache frequently used values
const hostname = os.hostname();
const pid = process.pid;

/**
 * Safely redacts specified paths in an object.
 * Avoids recursion and prototype pollution.
 */
function safeRedact<T>(obj: T, paths: string[]): T {
	if (!obj || typeof obj !== "object" || paths.length === 0) {
		return obj;
	}

	// Create a shallow copy to avoid modifying the original object.
	// This is much faster than a full deep clone.
	const newObj = { ...(obj as object) };

	for (const path of paths) {
		const parts = path.split(".");
		let current: unknown = newObj;

		for (let i = 0; i < parts.length; i++) {
			const part = parts[i];
			const isLastPart = i === parts.length - 1;

			if (
				current === null ||
				typeof current !== "object" ||
				!Object.hasOwn(current, part)
			) {
				// Path doesn't exist, nothing to redact.
				break;
			}

			if (isLastPart) {
				(current as LogObject)[part] = "[REDACTED]";
				break; // Done with this path
			}

			// Copy-on-write: Before descending, create a shallow copy of the next
			// object/array to avoid mutating nested properties of the original object.
			const next = (current as LogObject)[part];
			if (typeof next === "object" && next !== null) {
				(current as LogObject)[part] = Array.isArray(next)
					? [...next]
					: { ...next };
			} else {
				// Path leads to a non-object before the end, cannot redact further.
				break;
			}

			current = (current as LogObject)[part];
		}
	}
	return newObj as T;
}

export class Logger {
	readonly #stream: NodeJS.WritableStream;
	readonly #levelValue: number;
	readonly #redactPaths: string[];
	readonly #bindings: LogObject;
	readonly #pretty: boolean;
	readonly #name?: string;
	readonly #splitErrorStream: boolean;
	readonly #errorStream?: NodeJS.WritableStream;

	// Pre-allocated objects for zero-allocation logging
	readonly #baseLogObject: LogObject;

	// Reusable Set for circular reference detection to avoid allocation per call
	readonly #seen = new Set<unknown>();

	constructor(options: LoggerOptions = {}, bindings: LogObject = {}) {
		// Validate and set options
		const level = options.level || "info";
		if (!(level in levels)) {
			throw new Error(
				`Invalid log level: ${level}. Valid levels: ${Object.keys(levels).join(", ")}`,
			);
		}

		this.#stream = options.stream || process.stdout;
		this.#levelValue = levels[level];
		this.#redactPaths = Array.isArray(options.redact)
			? options.redact.slice()
			: [];
		this.#pretty = Boolean(options.pretty);
		this.#name = options.name;
		this.#splitErrorStream = options.splitErrorStream !== false; // default true
		this.#errorStream = options.errorStream;

		this.#bindings = Object.freeze({ ...bindings });

		// Pre-allocate base log object
		this.#baseLogObject = Object.freeze({
			pid,
			hostname,
			...(this.#name && { name: this.#name }),
		});
	}

	/**
	 * Creates a child logger with additional bound context.
	 */
	child(bindings: LogObject): Logger {
		if (!bindings || typeof bindings !== "object") {
			throw new Error("Child bindings must be an object");
		}

		const newBindings = { ...this.#bindings, ...bindings };
		const levelKey = levelNamesByValue[this.#levelValue];

		return new Logger(
			{
				stream: this.#stream,
				errorStream: this.#errorStream,
				level: levelKey,
				redact: this.#redactPaths,
				pretty: this.#pretty,
				name: this.#name,
				splitErrorStream: this.#splitErrorStream,
			},
			newBindings,
		);
	}

	/**
	 * Internal write method optimized for performance
	 */
	#write(level: number, msg: string, obj?: LogObject): void {
		if (level < this.#levelValue) {
			return;
		}

		// Combine and redact dynamic context first to reduce the scope of redaction.
		const context = { ...this.#bindings, ...(obj || {}) };
		const redactedContext =
			this.#redactPaths.length > 0
				? safeRedact(context, this.#redactPaths)
				: context;

		// Create final log object with minimal allocations
		const finalObject: LogObject = {
			level,
			time: Date.now(),
			...this.#baseLogObject,
			msg,
			...redactedContext,
		};

		// Format and write
		const output = this.#pretty
			? this.#formatPretty(finalObject)
			: this.#formatJson(finalObject);

		// Decide target stream
		const targetStream =
			this.#splitErrorStream && level >= levels.error
				? (this.#errorStream ?? process.stderr)
				: this.#stream;
		targetStream.write(output);
	}

	/**
	 * Safe JSON formatting with circular reference handling
	 */
	#formatJson(logObject: LogObject): string {
		try {
			// Using a class-level Set that is cleared after each use avoids
			// re-allocating a new Set for every single log line, which is
			// a significant performance improvement in hot paths.
			return (
				JSON.stringify(logObject, (_key, value) => {
					if (typeof value === "object" && value !== null) {
						if (this.#seen.has(value)) {
							return "[Circular]";
						}
						this.#seen.add(value);
					}
					// JSON.stringify does not support BigInts
					if (typeof value === "bigint") {
						return value.toString();
					}
					return value;
				}) + EOL
			);
		} catch {
			// Fallback for objects that can't be serialized
			return (
				JSON.stringify({
					level: logObject.level,
					time: logObject.time,
					pid: logObject.pid,
					hostname: logObject.hostname,
					msg: String(logObject.msg),
					error: "Failed to serialize log object",
				}) + EOL
			);
		} finally {
			// Ensure the Set is cleared for the next log operation.
			this.#seen.clear();
		}
	}

	/**
	 * Pretty formatter optimized for development
	 */
	#formatPretty(logObject: LogObject): string {
		const { time, level, hostname: h, pid: p, msg, ...rest } = logObject;
		const timestamp = new Date(time as number).toISOString();
		const levelName = levelNames[level as number] || "INFO";
		const levelColor = levelColors[level as number] || colors.green;

		// Use array join for better performance than string concatenation
		const mainParts = [
			`${colors.dim}[${timestamp}]${colors.reset}`,
			`${levelColor}${levelName}${colors.reset}`,
			`${colors.dim}(${p} on ${h})${colors.reset}:`,
			`${colors.magenta}${msg}${colors.reset}`,
		];

		let result = mainParts.join(" ") + EOL;

		// Add additional fields
		const restKeys = Object.keys(rest);
		if (restKeys.length > 0) {
			const details = restKeys
				.map(
					(key) =>
						`    ${colors.dim}${key}:${colors.reset} ${this.#safeStringify(rest[key])}`,
				)
				.join(EOL);
			result += details + EOL;
		}

		return result;
	}

	/**
	 * Safe stringify for pretty printing
	 */
	#safeStringify(value: unknown): string {
		if (typeof value === "string") {
			return JSON.stringify(value);
		}

		try {
			return JSON.stringify(value);
		} catch {
			return String(value);
		}
	}

	/**
	 * Normalize error-like objects
	 */
	#normalizeError(err: unknown): ErrorLike {
		if (err instanceof Error) {
			return err as ErrorLike;
		}

		if (typeof err === "object" && err !== null && "message" in err) {
			return err as ErrorLike;
		}

		return {
			message: String(err),
			name: "Error",
		};
	}

	// Public logging methods
	trace(msg: string, obj?: LogObject): void {
		this.#write(levels.trace, msg, obj);
	}

	debug(msg: string, obj?: LogObject): void {
		this.#write(levels.debug, msg, obj);
	}

	info(msg: string, obj?: LogObject): void {
		this.#write(levels.info, msg, obj);
	}

	warn(msg: string, obj?: LogObject): void {
		this.#write(levels.warn, msg, obj);
	}

	error(err: unknown, msg?: string, obj?: LogObject): void {
		const normalizedError = this.#normalizeError(err);
		const errorObj: LogObject = {
			err: {
				message: normalizedError.message,
				stack: normalizedError.stack,
				name: normalizedError.name,
				...(normalizedError.code && { code: normalizedError.code }),
				...normalizedError,
			},
			...(obj || {}),
		};

		this.#write(levels.error, msg || normalizedError.message, errorObj);
	}

	fatal(err: unknown, msg?: string, obj?: LogObject): void {
		const normalizedError = this.#normalizeError(err);
		const errorObj: LogObject = {
			err: {
				message: normalizedError.message,
				stack: normalizedError.stack,
				name: normalizedError.name,
				...(normalizedError.code && { code: normalizedError.code }),
				...normalizedError,
			},
			...(obj || {}),
		};

		this.#write(levels.fatal, msg || normalizedError.message, errorObj);
	}

	/**
	 * Check if a log level is enabled
	 */
	isLevelEnabled(level: LogLevel): boolean {
		return levels[level] >= this.#levelValue;
	}

	/**
	 * Get current log level
	 */
	get level(): LogLevel {
		return levelNamesByValue[this.#levelValue];
	}

	/**
	 * Graceful shutdown - flush any pending writes
	 */
	async flush(): Promise<void> {
		return new Promise<void>((resolve) => {
			if (this.#stream === process.stdout || this.#stream === process.stderr) {
				resolve(); // stdout/stderr don't need explicit flushing
				return;
			}

			if ("flush" in this.#stream && typeof this.#stream.flush === "function") {
				this.#stream.flush();
				resolve();
			} else if (
				"end" in this.#stream &&
				typeof this.#stream.end === "function"
			) {
				this.#stream.end(() => resolve());
			} else {
				resolve();
			}
		});
	}

	/**
	 * Create a logger instance with sensible defaults
	 */
	static create(options?: LoggerOptions): Logger {
		return new Logger(options);
	}
}

// Export convenience function for quick logger creation
export function createLogger(options?: LoggerOptions): Logger {
	return Logger.create(options);
}

// Export types for external use
export type { LoggerOptions, LogObject, ErrorLike, LogLevel };
export { levels };
