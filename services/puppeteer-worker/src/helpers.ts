/**
 * Shared helpers for puppeteer-worker
 * Centralizes common patterns: page event wrapping, safe async ops, error logging
 */

import type { Page } from "puppeteer";
import { logger, pageLogger } from "./logger.js";

/**
 * Attach a page event handler wrapped in standardized error logging.
 * Prevents unhandled errors from crashing the process when a page event handler throws.
 */
export function onPageEvent<K extends keyof import("puppeteer").PageEvents>(
	page: Page,
	event: K,
	handler: (arg: import("puppeteer").PageEvents[K]) => void,
): void {
	page.on(event, (arg: import("puppeteer").PageEvents[K]) => {
		try {
			handler(arg);
		} catch (err) {
			pageLogger.error(
				err instanceof Error ? err : new Error(String(err)),
				`Error in "${String(event)}" handler`,
				{ event: String(event) },
			);
		}
	});
}

/**
 * Attach browser console logging to a page using standardized log levels.
 * Extracts console type, text, and location for structured logging.
 */
export function attachConsoleLogger(
	page: Page,
	getPageUrl: () => string | undefined,
): void {
	onPageEvent(page, "console", (msg) => {
		const t = msg.type() as string;
		const text = msg.text();
		const loc = msg.location();
		const ctx = {
			source: "browser_console",
			consoleType: t,
			pageUrl: getPageUrl(),
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
}

/**
 * Attach pageerror logging to a page.
 */
export function attachPageErrorLogger(
	page: Page,
	getPageUrl: () => string | undefined,
): void {
	onPageEvent(page, "pageerror", (error) => {
		pageLogger.error(error, "[Page] runtime error", {
			source: "browser_pageerror",
			pageUrl: getPageUrl(),
		});
	});
}

/**
 * Run an async operation and swallow errors during shutdown,
 * logging them at warn level instead of letting them propagate.
 */
export async function safeAsync(
	label: string,
	fn: () => Promise<void>,
): Promise<void> {
	try {
		await fn();
	} catch (error) {
		logger.warn(`${label} error (swallowed)`, { err: error });
	}
}
