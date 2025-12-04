/**
 * JobClient abort behavior tests
 */

import { describe, test, beforeEach, afterEach } from "node:test";
import assert from "node:assert";
import { MockAgent, setGlobalDispatcher, getGlobalDispatcher } from "undici";
import { JobClient } from "../dist/job-client.js";

describe("JobClient aborts", () => {
	let mockAgent;
	let originalDispatcher;
	let mockPool;

	beforeEach(() => {
		originalDispatcher = getGlobalDispatcher();
		mockAgent = new MockAgent();
		setGlobalDispatcher(mockAgent);
		mockPool = mockAgent.get("http://localhost:8080");
	});

	afterEach(async () => {
		setGlobalDispatcher(originalDispatcher);
		await mockAgent.close();
	});

	test("complete should reject with AbortError when external signal is aborted", async () => {
		// Arrange: slow complete endpoint so we can abort while in-flight
		mockPool
			.intercept({
				path: "/api/jobs/job-abc/complete",
				method: "POST",
			})
			.reply(204)
			.delay(500);

		const client = new JobClient("http://localhost:8080", {
			timeoutMs: 1000,
			retries: 0,
		});
		const controller = new AbortController();

		// Act: start request then abort shortly after
		const promise = client.complete("job-abc", controller.signal);
		setTimeout(() => controller.abort(), 50);

		// Assert
		await assert.rejects(promise, (err) => err && err.name === "AbortError");
	});

	test("fail should reject with AbortError when external signal is aborted", async () => {
		// Arrange: slow fail endpoint so we can abort while in-flight
		mockPool
			.intercept({
				path: "/api/jobs/job-abc/fail",
				method: "POST",
			})
			.reply(204)
			.delay(500);

		const client = new JobClient("http://localhost:8080", {
			timeoutMs: 1000,
			retries: 0,
		});
		const controller = new AbortController();

		// Act: start request then abort shortly after
		const promise = client.fail("job-abc", "boom", controller.signal);
		setTimeout(() => controller.abort(), 50);

		// Assert
		await assert.rejects(promise, (err) => err && err.name === "AbortError");
	});
});
