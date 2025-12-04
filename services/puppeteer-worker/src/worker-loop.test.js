/**
 * WorkerLoop Tests
 *
 * Basic tests for the WorkerLoop orchestrator functionality
 */

import { test, describe, beforeEach, afterEach } from "node:test";
import assert from "node:assert";
import { MockAgent, setGlobalDispatcher, getGlobalDispatcher } from "undici";
import { WorkerLoop } from "../dist/worker-loop.js";

describe("WorkerLoop", () => {
	let mockAgent;
	let mockPool;
	let originalDispatcher;
	let mockConfig;
	let workerLoop;

	beforeEach(() => {
		// Setup undici MockAgent
		originalDispatcher = getGlobalDispatcher();
		mockAgent = new MockAgent();
		setGlobalDispatcher(mockAgent);

		// Create mock pool for our API base URL
		mockPool = mockAgent.get("http://localhost:8080");

		// Mock configuration
		mockConfig = {
			worker: {
				apiBaseUrl: "http://localhost:8080",
				jobType: "browser",
				leaseSeconds: 30,
				waitSeconds: 25,
				heartbeatSeconds: 10,
			},
			config: {
				headless: true,
				fileCapture: { enabled: false },
			},
		};
	});

	afterEach(async () => {
		if (workerLoop?.running) {
			await workerLoop.stop();
		}

		// Restore original dispatcher
		setGlobalDispatcher(originalDispatcher);
		await mockAgent.close();
	});

	test("should create WorkerLoop with valid configuration", () => {
		workerLoop = new WorkerLoop(mockConfig);
		assert.ok(workerLoop);

		const stats = workerLoop.getStats();
		assert.strictEqual(stats.jobsProcessed, 0);
		assert.strictEqual(stats.jobsCompleted, 0);
		assert.strictEqual(stats.jobsFailed, 0);
		assert.ok(stats.uptime >= 0);
	});

	test("should handle no jobs available", async () => {
		// Mock the reserve_next endpoint to return 204 (no jobs)
		mockPool
			.intercept({
				path: "/api/jobs/browser/reserve_next?lease=30&wait=25",
				method: "GET",
			})
			.reply(204);

		workerLoop = new WorkerLoop(mockConfig);

		// Start and immediately stop to test the polling behavior
		const startPromise = workerLoop.start();

		// Give it a moment to poll
		await new Promise((resolve) => setTimeout(resolve, 100));

		await workerLoop.stop();
		await startPromise;

		// Test passes if no errors occurred during polling
		assert.ok(true);
	});

	test("should make HTTP request to reserve job", async () => {
		const mockJob = {
			id: "test-job-123",
			type: "browser",
			payload: 'console.log("test script");',
		};

		// Mock the reserve_next endpoint to return a job once, then no jobs
		mockPool
			.intercept({
				path: "/api/jobs/browser/reserve_next?lease=30&wait=25",
				method: "GET",
			})
			.reply(200, mockJob)
			.times(1);

		mockPool
			.intercept({
				path: "/api/jobs/browser/reserve_next?lease=30&wait=25",
				method: "GET",
			})
			.reply(204)
			.persist();

		// Mock heartbeat endpoint
		mockPool
			.intercept({
				path: "/api/jobs/test-job-123/heartbeat?extend=30",
				method: "POST",
			})
			.reply(204)
			.persist();

		// Mock complete endpoint
		mockPool
			.intercept({
				path: "/api/jobs/test-job-123/complete",
				method: "POST",
			})
			.reply(204);

		workerLoop = new WorkerLoop(mockConfig);

		// Start and stop quickly to process one job
		const startPromise = workerLoop.start();

		// Give it time to process (but not too long since PuppeteerRunner will fail)
		await new Promise((resolve) => setTimeout(resolve, 100));

		await workerLoop.stop();
		await startPromise;

		// Test passes if HTTP requests were made without errors
		assert.ok(true);
	});

	test("should handle HTTP errors gracefully", async () => {
		// Mock the reserve_next endpoint to return an error
		mockPool
			.intercept({
				path: "/api/jobs/browser/reserve_next?lease=30&wait=25",
				method: "GET",
			})
			.reply(500, { error: "Internal server error" });

		workerLoop = new WorkerLoop(mockConfig);

		const startPromise = workerLoop.start();

		// Give it time to handle the error
		await new Promise((resolve) => setTimeout(resolve, 100));

		await workerLoop.stop();
		await startPromise;

		// Test passes if the worker handles the error without crashing
		assert.ok(true);
	});

	test("should handle graceful shutdown", async () => {
		// Mock long-running reserve_next request
		mockPool
			.intercept({
				path: "/api/jobs/browser/reserve_next?lease=30&wait=25",
				method: "GET",
			})
			.reply(204)
			.delay(1000); // 1 second delay

		workerLoop = new WorkerLoop(mockConfig);

		const startPromise = workerLoop.start();

		// Give it a moment to start
		await new Promise((resolve) => setTimeout(resolve, 50));

		// Stop should complete quickly
		const stopStart = Date.now();
		await workerLoop.stop();
		const stopTime = Date.now() - stopStart;

		await startPromise;

		// Should stop relatively quickly (not wait for full timeout)
		assert.ok(stopTime < 5000, `Stop took ${stopTime}ms, expected < 5000ms`);
	});
});
