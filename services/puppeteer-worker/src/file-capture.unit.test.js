import assert from "node:assert/strict";
import { describe, test } from "node:test";

// NOTE: Tests import the built artifact. Run `npm run build` before these tests.
import { FileCapture } from "../dist/file-capture.js";

function makeMemoryConfig(overrides = {}) {
	return {
		enabled: true,
		types: ["script", "document", "stylesheet"],
		maxFileSize: 1024 * 1024,
		storage: "memory",
		storageConfig: { ...overrides },
	};
}

describe("FileCapture in-run dedupe (memory storage)", () => {
	test("same-session duplicate uses same fileId and is retrievable", async () => {
		const sessionId = `sess-${Math.random().toString(36).slice(2)}`;
		const fc = new FileCapture(makeMemoryConfig(), sessionId);

		const content = Buffer.from("same content");

		const first = await fc.captureFile({
			url: "https://example.com/a.js",
			content,
			contentType: "application/javascript; charset=utf-8",
			sessionId,
		});

		assert.ok(first);

		const second = await fc.captureFile({
			url: "https://example.com/a.js",
			content,
			contentType: "application/javascript; charset=utf-8",
			sessionId,
		});

		assert.ok(second);
		assert.equal(second.fileId, first.fileId);
		assert.equal(second.metadata.captureReason, "duplicate");

		const roundTrip = await fc.retrieveFile(first.fileId);
		assert.ok(roundTrip);
		assert.equal(roundTrip.toString("utf8"), "same content");

		await fc.cleanup();
	});
});
