/**
 * JobClient abort behavior tests
 */

import { describe, test, beforeEach, afterEach } from "node:test";
import assert from "node:assert";
import { createServer } from "node:http";
import { JobClient } from "../dist/job-client.js";

describe("JobClient aborts", () => {
	let server;
	let port;

	beforeEach(async () => {
		server = createServer((req, res) => {
			// Simulate a slow response (500 ms); cancel the timer if the client disconnects
			const t = setTimeout(() => {
				if (!res.writableEnded) res.writeHead(204).end();
			}, 500);
			req.on("close", () => clearTimeout(t));
		});
		await new Promise((resolve) => server.listen(0, "127.0.0.1", resolve));
		port = server.address().port;
	});

	afterEach(
		() => new Promise((resolve, reject) => server.close((e) => (e ? reject(e) : resolve()))),
	);

	test("complete should reject with AbortError when external signal is aborted", async () => {
		const client = new JobClient(`http://127.0.0.1:${port}`, {
			timeoutMs: 1000,
			retries: 0,
		});
		const controller = new AbortController();

		const promise = client.complete("job-abc", controller.signal);
		setTimeout(() => controller.abort(), 50);

		await assert.rejects(promise, (err) => err != null && err.name === "AbortError");
	});

	test("fail should reject with AbortError when external signal is aborted", async () => {
		const client = new JobClient(`http://127.0.0.1:${port}`, {
			timeoutMs: 1000,
			retries: 0,
		});
		const controller = new AbortController();

		const promise = client.fail("job-abc", "boom", controller.signal);
		setTimeout(() => controller.abort(), 50);

		await assert.rejects(promise, (err) => err != null && err.name === "AbortError");
	});
});
