import assert from "node:assert";
import { readFile, unlink, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { afterEach, beforeEach, describe, test } from "node:test";
import puppeteer from "puppeteer";

import { pageLogger } from "../dist/logger.js";

// ============================================================================
// SIMPLE TEST CONFIGURATION
// ============================================================================

export const TestConfig = {
	browser: {
		headless: true,
		args: ["--no-sandbox", "--disable-setuid-sandbox"],
	},
	timeouts: {
		test: 10000, // 10 seconds max per test
		navigation: 3000, // 3 seconds for page navigation
		wait: 100, // 100ms for simple waits
	},
};

// Convenience helpers to match older tests
export async function createTestPage() {
	const browser = await puppeteer.launch(TestConfig.browser);
	const page = await browser.newPage();
	// attach for cleanup
	page.__browserForTest = browser;
	return page;
}

export async function cleanupTestPage(page) {
	try {
		await page?.close();
	} catch {}
	try {
		await page?.__browserForTest?.close?.();
	} catch {}
}

// ============================================================================
// BASIC BROWSER MANAGEMENT
// ============================================================================

export class SimpleBrowserManager {
	constructor() {
		this.browser = null;
		this.pages = new Set();
	}

	async createBrowser() {
		if (this.browser) {
			return this.browser;
		}

		this.browser = await puppeteer.launch(TestConfig.browser);
		return this.browser;
	}

	async createPage() {
		const browser = await this.createBrowser();
		const page = await browser.newPage();
		this.pages.add(page);
		return page;
	}

	async cleanup() {
		// Close all pages
		for (const page of this.pages) {
			try {
				await page.close();
			} catch (_error) {
				// Ignore errors during cleanup
			}
		}
		this.pages.clear();

		// Close browser
		if (this.browser) {
			try {
				await this.browser.close();
			} catch (_error) {
				// Ignore errors during cleanup
			}
			this.browser = null;
		}
	}
}

// ============================================================================
// SIMPLE TEST PATTERNS
// ============================================================================

export function withBrowser(testFn) {
	return async () => {
		const browserManager = new SimpleBrowserManager();
		try {
			const browser = await browserManager.createBrowser();
			await testFn(browser, browserManager);
		} finally {
			await browserManager.cleanup();
		}
	};
}

export function withPage(testFn) {
	return async () => {
		const browserManager = new SimpleBrowserManager();
		try {
			const page = await browserManager.createPage();
			await testFn(page);
		} finally {
			await browserManager.cleanup();
		}
	};
}

export function withMonitoredPage(testFn) {
	return async () => {
		const browserManager = new SimpleBrowserManager();
		try {
			const page = await browserManager.createPage();

			// Enable console logging for debugging
			page.on("console", (msg) => {
				if (msg.type() === "error") {
					pageLogger.error(
						{ name: "BrowserConsoleError", message: msg.text() },
						"[Page] console error",
						{
							source: "browser_console",
							consoleType: msg.type(),
						},
					);
				}
			});

			page.on("pageerror", (error) => {
				pageLogger.error(error, "[Page] runtime error", {
					source: "browser_pageerror",
				});
			});

			// Inject client monitoring
			const clientScript = await readFile(
				join(process.cwd(), "src/client-monitoring.js"),
				"utf8",
			);

			// Inject on new documents
			await page.evaluateOnNewDocument(clientScript);

			await testFn(page);
		} finally {
			await browserManager.cleanup();
		}
	};
}

// ============================================================================
// SIMPLE ASSERTIONS
// ============================================================================

export function assertEventExists(events, eventType) {
	const found = events.some((event) => event.type === eventType);
	assert.ok(found, `Expected event type '${eventType}' not found`);
}

export function assertEventCount(events, eventType, expectedCount) {
	const count = events.filter((event) => event.type === eventType).length;
	assert.strictEqual(
		count,
		expectedCount,
		`Expected ${expectedCount} events of type '${eventType}', got ${count}`,
	);
}

export function assertEventData(events, eventType, expectedData) {
	const event = events.find((event) => event.type === eventType);
	assert.ok(event, `Event type '${eventType}' not found`);

	for (const [key, value] of Object.entries(expectedData)) {
		assert.strictEqual(
			event.data[key],
			value,
			`Expected event.data.${key} to be ${value}, got ${event.data[key]}`,
		);
	}
}

export function assertNoErrors(events) {
	const errorEvents = events.filter(
		(event) => event.type.includes("error") || event.type.includes("Error"),
	);
	assert.strictEqual(
		errorEvents.length,
		0,
		`Unexpected error events: ${JSON.stringify(errorEvents)}`,
	);
}

// ============================================================================
// SIMPLE EVENT COLLECTION
// ============================================================================

export async function collectClientEvents(page, options = {}) {
	const timeout = options.timeout || TestConfig.timeouts.wait;

	// Wait for events to be collected
	await new Promise((resolve) => setTimeout(resolve, timeout));

	// Get events from client-side monitoring
	const events = await page.evaluate(() => {
		return window.__puppeteerMonitoring
			? window.__puppeteerMonitoring.getEvents()
			: [];
	});

	return events;
}

export async function triggerAndCollectEvents(page, triggerFn, options = {}) {
	// Ensure monitoring is active and clear existing events
	await page.evaluate(() => {
		if (window.__puppeteerMonitoring) {
			window.__puppeteerMonitoring.clearEvents();
		} else {
			console.warn("Monitoring not available, events may not be captured");
		}
	});

	// Trigger the action
	await triggerFn();

	// Wait a bit for events to be processed
	await new Promise((resolve) => setTimeout(resolve, TestConfig.timeouts.wait));

	// Collect events
	return await collectClientEvents(page, options);
}

// ============================================================================
// SIMPLE TEST DATA
// ============================================================================

export const TestData = {
	simpleHtml: `
    <!DOCTYPE html>
    <html>
    <head><title>Test Page</title></head>
    <body>
      <h1>Test Page</h1>
      <script>console.log('Page loaded');</script>
    </body>
    </html>
  `,

	scriptWithStorage: `
    localStorage.setItem('test-key', 'test-value');
    sessionStorage.setItem('session-key', 'session-value');
    console.log('Storage operations completed');
  `,

	scriptWithDynamicCode: `
    eval('console.log("Dynamic code executed")');
    new Function('console.log("Function constructor used")')();
  `,

	urls: {
		example: "https://example.com",
		httpbin: "https://httpbin.org/html",
		local: "data:text/html,<h1>Local Test</h1>",
	},
};

// ============================================================================
// SIMPLE TEST HELPERS
// ============================================================================

export async function navigateToTestPage(page, html = TestData.simpleHtml) {
	// Create a temporary HTML file to avoid data: URL storage restrictions
	const fullHtml = `<!DOCTYPE html>
<html>
<head>
  <title>Test Page</title>
  <meta charset="utf-8">
</head>
<body>
  ${html.includes("<body>") ? html.replace(/.*<body[^>]*>|<\/body>.*/g, "") : html}
  <script>
    // Ensure storage APIs are available
    if (typeof Storage === "undefined") {
      console.warn("Storage not supported");
    } else {
      console.log("Storage APIs are available");
    }
  </script>
</body>
</html>`;

	// Create a temporary file
	const tempFile = join(tmpdir(), `puppeteer-test-${Date.now()}.html`);
	await writeFile(tempFile, fullHtml, "utf8");

	try {
		// Use file:// URL which supports localStorage
		const fileUrl = `file://${tempFile}`;
		await page.goto(fileUrl, {
			waitUntil: "domcontentloaded",
			timeout: TestConfig.timeouts.navigation,
		});

		// Wait for page to be ready
		await page.waitForFunction(() => document.readyState === "complete", {
			timeout: TestConfig.timeouts.navigation,
		});

		// Re-inject monitoring script after navigation (if not already injected)
		await ensureMonitoringInjected(page);
	} finally {
		// Clean up the temporary file
		try {
			await unlink(tempFile);
		} catch (_error) {
			// Ignore cleanup errors
		}
	}
}

export async function ensureMonitoringInjected(page) {
	// Check if monitoring is already active
	const isActive = await page.evaluate(() => {
		return window.__puppeteerMonitoringActive;
	});

	if (!isActive) {
		// Inject the monitoring script
		const clientScript = await readFile(
			join(process.cwd(), "src/client-monitoring.js"),
			"utf8",
		);

		await page.evaluate(clientScript);

		// Wait a bit for initialization
		await new Promise((resolve) =>
			setTimeout(resolve, TestConfig.timeouts.wait),
		);
	}
}

export async function executeScript(page, script) {
	return await page.evaluate(script);
}

export async function waitForEvents(
	page,
	expectedCount = 1,
	timeout = TestConfig.timeouts.wait,
) {
	const startTime = Date.now();

	while (Date.now() - startTime < timeout) {
		const events = await collectClientEvents(page);
		if (events.length >= expectedCount) {
			return events;
		}
		await new Promise((resolve) => setTimeout(resolve, 10));
	}

	// Return whatever events we have
	return await collectClientEvents(page);
}

// ============================================================================
// SIMPLE TEST SUITE HELPERS
// ============================================================================

export function createTestSuite(suiteName, tests) {
	describe(suiteName, () => {
		for (const [testName, testFn] of Object.entries(tests)) {
			test(testName, testFn);
		}
	});
}

export function createBrowserTestSuite(suiteName, tests) {
	describe(suiteName, () => {
		for (const [testName, testFn] of Object.entries(tests)) {
			test(testName, withBrowser(testFn));
		}
	});
}

export function createPageTestSuite(suiteName, tests) {
	describe(suiteName, () => {
		for (const [testName, testFn] of Object.entries(tests)) {
			test(testName, withPage(testFn));
		}
	});
}

export function createMonitoringTestSuite(suiteName, tests) {
	describe(suiteName, () => {
		for (const [testName, testFn] of Object.entries(tests)) {
			test(testName, withMonitoredPage(testFn));
		}
	});
}

// ============================================================================
// EXPORTS
// ============================================================================

export { describe, test, beforeEach, afterEach, assert };
