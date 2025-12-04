/**
 * Test that events are shipped even when script execution fails
 */

import { describe, it, before, after } from "node:test";
import assert from "node:assert";
import { PuppeteerRunner } from "../dist/puppeteer-runner.js";
import http from "node:http";

describe("PuppeteerRunner event shipping on failure", () => {
	let server;
	let shippedEvents = [];
	const port = 58888;

	before(async () => {
		// Create a simple HTTP server to receive events
		server = http.createServer((req, res) => {
			if (req.method === "POST" && req.url === "/api/events/bulk") {
				let body = "";
				req.on("data", (chunk) => {
					body += chunk.toString();
				});
				req.on("end", () => {
					try {
						const data = JSON.parse(body);
						shippedEvents.push(...(data.events || []));
						res.writeHead(200, { "Content-Type": "application/json" });
						res.end(
							JSON.stringify({
								success: true,
								count: data.events?.length || 0,
							}),
						);
					} catch (_err) {
						res.writeHead(400);
						res.end(JSON.stringify({ error: "Invalid JSON" }));
					}
				});
			} else {
				res.writeHead(404);
				res.end();
			}
		});

		await new Promise((resolve) => {
			server.listen(port, "127.0.0.1", resolve);
		});
	});

	after(async () => {
		if (server) {
			await new Promise((resolve) => server.close(resolve));
		}
	});

	it("should ship events even when script fails", async () => {
		shippedEvents = [];
		const runner = new PuppeteerRunner();

		// Script that will fail but should still ship console events
		// Use page.evaluate to run console calls in browser context
		const script = `
			await page.evaluate(() => {
				console.log("Event before failure");
				console.error("Error event before failure");
			});
			await page.waitForSelector('#nonexistent-element', { timeout: 100 });
		`;

		const config = {
			shipping: {
				endpoint: `http://127.0.0.1:${port}/api/events/bulk`,
				jobId: "test-job-failure",
				sourceId: "test-source",
			},
		};

		const result = await runner.runScript(script, config);

		// Script should fail
		assert.strictEqual(result.success, false, "Script should fail");
		assert.ok(result.error, "Should have error message");

		// Give the event shipper time to complete the final flush
		await new Promise((resolve) => setTimeout(resolve, 1500));

		// Verify events were shipped
		assert.ok(
			shippedEvents.length > 0,
			"Events should have been shipped despite failure",
		);

		// Verify we got the console events (check method field instead of type)
		const consoleEvents = shippedEvents.filter(
			(e) => e.method?.includes("console") || e.method === "Page.console",
		);
		assert.ok(consoleEvents.length > 0, "Should have console events");
	});

	it("should ship events when script succeeds", async () => {
		shippedEvents = [];
		const runner = new PuppeteerRunner();

		// Script that succeeds
		const script = `
			await page.evaluate(() => {
				console.log("Success event");
			});
			await page.goto('about:blank');
		`;

		const config = {
			shipping: {
				endpoint: `http://127.0.0.1:${port}/api/events/bulk`,
				jobId: "test-job-success",
				sourceId: "test-source",
			},
		};

		const result = await runner.runScript(script, config);

		// Script should succeed
		assert.strictEqual(result.success, true, "Script should succeed");

		// Give the event shipper a moment to complete
		await new Promise((resolve) => setTimeout(resolve, 500));

		// Verify events were shipped
		assert.ok(
			shippedEvents.length > 0,
			"Events should have been shipped on success",
		);
	});

	it("should not lose events that arrive during shipping", async () => {
		shippedEvents = [];
		const runner = new PuppeteerRunner();

		// Script that generates many events quickly to test concurrent shipping
		const script = `
			for (let i = 0; i < 150; i++) {
				await page.evaluate((n) => {
					console.log(\`Event \${n}\`);
				}, i);
			}
		`;

		const config = {
			shipping: {
				endpoint: `http://127.0.0.1:${port}/api/events/bulk`,
				jobId: "test-job-concurrent",
				sourceId: "test-source",
				batchSize: 50, // Small batch to trigger multiple flushes
				maxBatchAge: 10000, // Long interval so only batch-size triggers
			},
		};

		const result = await runner.runScript(script, config);

		// Script should succeed
		assert.strictEqual(result.success, true, "Script should succeed");

		// Give the event shipper time to complete all flushes
		await new Promise((resolve) => setTimeout(resolve, 2000));

		// Verify all events were shipped (should be 150 console events + some overhead)
		assert.ok(
			shippedEvents.length >= 150,
			`Should have shipped at least 150 events, got ${shippedEvents.length}`,
		);
	});
});
