/**
 * @file puppeteer-runner.failure.test.js
 * @description Tests for failure detection and Worker.jobFailure event emission
 */

import { describe, it, before, after } from "node:test";
import assert from "node:assert/strict";
import { PuppeteerRunner } from "../dist/puppeteer-runner.js";
import { createTestPage, cleanupTestPage } from "./test-utils.js";

describe("PuppeteerRunner - Failure Detection", () => {
	let page;

	before(async () => {
		page = await createTestPage();
	});

	after(async () => {
		await cleanupTestPage(page);
	});

	it("should emit Worker.jobFailure event when selector is not found", async () => {
		const runner = new PuppeteerRunner();
		const script = `
			await page.goto('data:text/html,<html><body><input id="realInput" /></body></html>', { waitUntil: 'load' });
			await page.type('[id="nonExistentInput"]', 'test');
		`;

		const config = {
			headless: true,
			shipping: null, // Disable shipping for test
		};

		// Execute script - should fail
		const result = await runner.runScript(script, config);

		// Verify execution failed
		assert.equal(result.success, false, "Script execution should fail");
		assert.ok(result.error, "Should have error message");
		assert.ok(
			result.error.includes("waiting for selector") ||
				result.error.includes("Timeout") ||
				result.error.includes("No element found for selector"),
			`Error should indicate missing selector or timeout, got: ${result.error}`,
		);

		// Verify Worker.jobFailure event was emitted
		const events = runner.events || [];
		const failureEvent = events.find((e) => e.method === "Worker.jobFailure");

		assert.ok(failureEvent, "Should emit Worker.jobFailure event");
		assert.equal(failureEvent.params.payload.type, "jobFailure");
		assert.ok(failureEvent.params.payload.errorType, "Should have errorType");
		assert.ok(
			failureEvent.params.payload.errorMessage,
			"Should have errorMessage",
		);
		assert.ok(failureEvent.params.payload.timestamp, "Should have timestamp");

		// Verify error categorization
		const payload = failureEvent.params.payload;
		assert.ok(
			payload.errorType.includes("Timeout") ||
				payload.errorType.includes("Selector"),
			`Error type should indicate timeout/selector issue, got: ${payload.errorType}`,
		);
	});

	it("should emit Worker.jobFailure event when navigation fails", async () => {
		const runner = new PuppeteerRunner();
		const script = `
			await page.goto('http://this-domain-does-not-exist-12345.invalid', { waitUntil: 'load' });
		`;

		const config = {
			headless: true,
			shipping: null,
		};

		const result = await runner.runScript(script, config);

		assert.equal(result.success, false, "Script execution should fail");
		assert.ok(result.error, "Should have error message");

		const events = runner.events || [];
		const failureEvent = events.find((e) => e.method === "Worker.jobFailure");

		assert.ok(failureEvent, "Should emit Worker.jobFailure event");
		const payload = failureEvent.params.payload;
		assert.ok(payload.errorType, "Should have errorType");
		assert.ok(payload.errorMessage, "Should have errorMessage");
	});

	it("should capture screenshot on failure when page is usable", async () => {
		const runner = new PuppeteerRunner();
		const script = `
			await page.goto('data:text/html,<html><body><h1>Test Page</h1></body></html>', { waitUntil: 'load' });
			await page.type('[id="nonExistent"]', 'test');
		`;

		const config = {
			headless: true,
			shipping: null,
		};

		const result = await runner.runScript(script, config);

		assert.equal(result.success, false, "Script execution should fail");

		const events = runner.events || [];
		const failureEvent = events.find((e) => e.method === "Worker.jobFailure");

		assert.ok(failureEvent, "Should emit Worker.jobFailure event");
		const payload = failureEvent.params.payload;

		// Screenshot may or may not be present depending on timing
		// but if present, should be valid base64
		if (payload.screenshot) {
			assert.ok(
				payload.screenshot.length > 0,
				"Screenshot should not be empty",
			);
			assert.ok(payload.screenshotMime, "Should have screenshot MIME type");
			assert.ok(
				payload.screenshotMime === "image/png" ||
					payload.screenshotMime === "image/jpeg",
				`Screenshot MIME should be image/png or image/jpeg, got: ${payload.screenshotMime}`,
			);
		}
	});

	it("should sanitize error messages to remove sensitive paths", async () => {
		const runner = new PuppeteerRunner();
		// This will fail with a script error that might contain file paths
		const script = `
			throw new Error("Failed at /usr/local/app/sensitive/path.js:123");
		`;

		const config = {
			headless: true,
			shipping: null,
		};

		const result = await runner.runScript(script, config);

		assert.equal(result.success, false, "Script execution should fail");

		const events = runner.events || [];
		const failureEvent = events.find((e) => e.method === "Worker.jobFailure");

		assert.ok(failureEvent, "Should emit Worker.jobFailure event");
		const payload = failureEvent.params.payload;

		// Error message should be sanitized
		assert.ok(
			!payload.errorMessage.includes("/usr/local/app/sensitive"),
			"Error message should not contain full file paths",
		);
		assert.ok(
			payload.errorMessage.includes("/...") ||
				payload.errorMessage.includes("..."),
			"Error message should contain sanitized path placeholder",
		);
	});

	it("should include failure context for debugging", async () => {
		const runner = new PuppeteerRunner();
		const script = `
			await page.goto('data:text/html,<html><body><input id="test" /></body></html>', { waitUntil: 'load' });
			await page.click('[data-test="missing"]');
		`;

		const config = {
			headless: true,
			shipping: null,
		};

		const result = await runner.runScript(script, config);

		assert.equal(result.success, false, "Script execution should fail");

		const events = runner.events || [];
		const failureEvent = events.find((e) => e.method === "Worker.jobFailure");

		assert.ok(failureEvent, "Should emit Worker.jobFailure event");
		const payload = failureEvent.params.payload;

		// Should have context for LLM analysis
		assert.ok(payload.context, "Should have context object");
		assert.ok(
			typeof payload.context.scriptLength === "number",
			"Should include script length",
		);
		assert.ok(
			typeof payload.context.hasPageOperations === "boolean",
			"Should indicate if script has page operations",
		);
	});

	it("should include page URL in failure context", async () => {
		const runner = new PuppeteerRunner();
		const testUrl = "data:text/html,<html><body><h1>Test</h1></body></html>";
		const script = `
			await page.goto('${testUrl}', { waitUntil: 'load' });
			await page.type('[id="missing"]', 'test');
		`;

		const config = {
			headless: true,
			shipping: null,
		};

		const result = await runner.runScript(script, config);

		assert.equal(result.success, false, "Script execution should fail");

		const events = runner.events || [];
		const failureEvent = events.find((e) => e.method === "Worker.jobFailure");

		assert.ok(failureEvent, "Should emit Worker.jobFailure event");
		const payload = failureEvent.params.payload;

		// Should capture the page URL at time of failure
		assert.ok(payload.url, "Should have page URL");
		assert.ok(
			payload.url.startsWith("data:text/html"),
			"URL should match test page",
		);
	});

	it("should capture DOMException without crashing the runner", async () => {
		const runner = new PuppeteerRunner();
		const script = `
			await page.goto('about:blank', { waitUntil: 'load' });
			await page.evaluate(() => {
				throw new DOMException('Access is denied for this document.', 'SecurityError');
			});
		`;

		const config = {
			headless: true,
			shipping: null,
		};

		const result = await runner.runScript(script, config);

		assert.equal(
			result.success,
			false,
			"Script execution should fail on DOMException",
		);

		const events = runner.events || [];
		const failureEvent = events.find((e) => e.method === "Worker.jobFailure");

		assert.ok(failureEvent, "Should emit Worker.jobFailure event");
		const payload = failureEvent.params.payload;

		assert.equal(payload.errorType, "DOMException");
		assert.ok(
			payload.errorMessage.includes("Access is denied"),
			"Error message should include DOMException detail",
		);
	});
});
