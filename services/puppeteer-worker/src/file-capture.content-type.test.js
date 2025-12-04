import assert from "node:assert/strict";
import { describe, test } from "node:test";

// NOTE: Tests import the built artifact. Run `npm run build` before these tests.
import { FileCapture } from "../dist/file-capture.js";

function makeMemoryConfig(types = ["document"]) {
	return {
		enabled: true,
		types,
		maxFileSize: 1024 * 1024,
		storage: "memory",
		storageConfig: {},
	};
}

describe("FileCapture content-type coverage", () => {
	test("captures JSON as document", async () => {
		const sessionId = `sess-json-${Math.random().toString(36).slice(2)}`;
		const fc = new FileCapture(makeMemoryConfig(["document"]), sessionId);

		const content = Buffer.from('{"ok":true}');

		const ctx = await fc.captureFile({
			url: "https://example.com/data.json",
			content,
			contentType: "application/json; charset=utf-8",
			sessionId,
		});

		assert.ok(ctx, "expected JSON to be captured");
		assert.equal(ctx.contentType.includes("application/json"), true);

		await fc.cleanup();
	});

	test("captures XML (+xml) as document", async () => {
		const sessionId = `sess-xml-${Math.random().toString(36).slice(2)}`;
		const fc = new FileCapture(makeMemoryConfig(["document"]), sessionId);

		const content = Buffer.from('<rss version="2.0"><channel></channel></rss>');

		const ctx = await fc.captureFile({
			url: "https://example.com/feed",
			content,
			contentType: "application/rss+xml; charset=utf-8",
			sessionId,
		});

		assert.ok(ctx, "expected XML to be captured");
		assert.equal(
			ctx.contentType.includes("+xml") || ctx.contentType.includes("xml"),
			true,
		);

		await fc.cleanup();
	});
});
