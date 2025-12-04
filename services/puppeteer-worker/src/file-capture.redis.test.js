import assert from "node:assert/strict";
import { describe, test } from "node:test";
import Redis from "ioredis";

// NOTE: Tests import the built artifact. Run `npm run build` before these tests.
import { FileCapture } from "../dist/file-capture.js";

async function canConnectRedis(host = "127.0.0.1", port = 6380) {
	const client = new Redis({ host, port, lazyConnect: true });
	try {
		await client.connect();
		const pong = await client.ping();
		await client.quit();
		return pong === "PONG";
	} catch {
		try {
			await client.quit();
		} catch {}
		return false;
	}
}

function makeConfig(overrides = {}) {
	return {
		enabled: true,
		types: ["script", "document", "stylesheet"],
		maxFileSize: 1024 * 1024, // 1MB
		storage: "redis",
		storageConfig: {
			host: "127.0.0.1",
			port: 6380,
			ttlSeconds: 30,
			prefix: "filecap:test:",
			...overrides,
		},
	};
}

describe("RedisStorageProvider integration", async () => {
	const available = await canConnectRedis();
	if (!available) {
		test(
			"skipped: Redis not available on localhost:6380",
			{ skip: true },
			() => {},
		);
		return;
	}

	test("stores and retrieves a new file", async () => {
		const sessionId = `sess-${Math.random().toString(36).slice(2)}`;
		const fc = new FileCapture(makeConfig(), sessionId);

		const payload = Buffer.from("hello world");
		const ctx = await fc.captureFile({
			url: "https://example.com/app.js",
			content: payload,
			contentType: "application/javascript; charset=utf-8",
			sessionId,
		});

		assert.ok(ctx, "context should be returned");
		assert.equal(ctx.storageProvider, "redis");
		assert.match(ctx.storageKey, new RegExp(`^${sessionId}/`));
		assert.equal(ctx.metadata.captureReason, "new_content");

		// Verify round-trip via Redis provider through FileCapture API (same session)
		const fileId = ctx.fileId;
		const roundTrip = await fc.retrieveFile(fileId);
		assert.ok(roundTrip);
		assert.equal(roundTrip.toString("utf8"), "hello world");

		await fc.cleanup();
	});

	test("avoids re-caching duplicates across sessions via Redis hash index", async () => {
		const payload = Buffer.from("same content across runs");

		// First run
		const sessionA = `sessA-${Math.random().toString(36).slice(2)}`;
		const fcA = new FileCapture(makeConfig(), sessionA);
		const ctxA = await fcA.captureFile({
			url: "https://example.com/app.js",
			content: payload,
			contentType: "application/javascript; charset=utf-8",
			sessionId: sessionA,
		});
		assert.ok(ctxA);

		// Second run with new session, same content
		const sessionB = `sessB-${Math.random().toString(36).slice(2)}`;
		const fcB = new FileCapture(makeConfig(), sessionB);
		const ctxB = await fcB.captureFile({
			url: "https://example.com/app.js",
			content: payload,
			contentType: "application/javascript; charset=utf-8",
			sessionId: sessionB,
		});

		assert.ok(ctxB);
		assert.equal(ctxB.metadata.captureReason, "duplicate");
		// Should reference the same storage key as the first run
		assert.equal(ctxB.storageKey, ctxA.storageKey);

		// Verify retrieval via FileCapture API for deduped file in new session
		const roundTripB = await fcB.retrieveFile(ctxB.fileId);
		assert.ok(roundTripB);
		assert.equal(roundTripB.toString("utf8"), payload.toString("utf8"));

		await fcA.cleanup();
		await fcB.cleanup();
	});
});
