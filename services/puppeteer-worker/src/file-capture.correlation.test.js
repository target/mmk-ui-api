import assert from "node:assert";
import { createHash } from "node:crypto";
import http from "node:http";
import { test } from "node:test";

// We import the built JS output. Ensure to run `npm run build` before this test.
import { PuppeteerRunner } from "../dist/puppeteer-runner.js";

function sha256Hex(buf) {
	return createHash("sha256").update(buf).digest("hex");
}

function startTestServer() {
	const aBody = Buffer.from("console.log('A');");
	const bBody = Buffer.from("console.log('B');");

	const server = http.createServer((req, res) => {
		if (req.url === "/") {
			const html = `<!doctype html><html><head>
        <script src="/a.js"></script>
        <script src="/b.js"></script>
      </head><body>OK</body></html>`;
			res.writeHead(200, { "Content-Type": "text/html; charset=utf-8" });
			res.end(html);
			return;
		}
		if (req.url === "/a.js") {
			// Small delay to create interleaving potential
			setTimeout(() => {
				res.writeHead(200, { "Content-Type": "application/javascript" });
				res.end(aBody);
			}, 60);
			return;
		}
		if (req.url === "/b.js") {
			res.writeHead(200, { "Content-Type": "application/javascript" });
			res.end(bBody);
			return;
		}
		res.writeHead(404).end();
	});

	return new Promise((resolve) => {
		server.listen(0, "127.0.0.1", () => {
			const { port } = server.address();
			resolve({ server, port, aBody, bBody });
		});
	});
}

// Main test

test("capturedFile is correlated to the correct Network.responseReceived by requestId (and requestWillBeSent has matching requestId)", async (_t) => {
	const { server, port, aBody, bBody } = await startTestServer();
	const runner = new PuppeteerRunner();

	try {
		const config = {
			headless: true,
			fileCapture: { enabled: true, types: ["script"], storage: "memory" },
		};

		// initialize and navigate
		await runner.initialize(config);
		const page = runner.page;
		assert.ok(page, "Page should be initialized");

		// Start waiting for responses before navigation to avoid races
		const waitA = page.waitForResponse(
			(r) => r.url().endsWith("/a.js") && r.status() === 200,
		);
		const waitB = page.waitForResponse(
			(r) => r.url().endsWith("/b.js") && r.status() === 200,
		);
		await page.goto(`http://127.0.0.1:${port}/`);
		await Promise.all([waitA, waitB]);

		// Poll until capturedFile is attached to both response events (up to ~2s)
		{
			const deadline = Date.now() + 2000;
			while (Date.now() < deadline) {
				const resps = runner.events.filter(
					(e) => e.method === "Network.responseReceived",
				);
				const a = resps.find((e) => e.params?.payload?.url?.endsWith("/a.js"));
				const b = resps.find((e) => e.params?.payload?.url?.endsWith("/b.js"));
				if (
					a?.params?.payload?.capturedFile &&
					b?.params?.payload?.capturedFile
				) {
					break;
				}
				await new Promise((r) => setTimeout(r, 50));
			}
		}

		const events = runner.events;
		const responses = events.filter(
			(e) => e.method === "Network.responseReceived",
		);
		const requests = events.filter(
			(e) => e.method === "Network.requestWillBeSent",
		);

		// Find events by URL
		const evtA = responses.find((e) =>
			e.params?.payload?.url?.endsWith("/a.js"),
		);
		const evtB = responses.find((e) =>
			e.params?.payload?.url?.endsWith("/b.js"),
		);

		assert.ok(evtA, "Response event for /a.js should exist");
		assert.ok(evtB, "Response event for /b.js should exist");

		// Validate requestId presence
		assert.ok(evtA.params.payload.requestId, "evtA should have requestId");
		assert.ok(evtB.params.payload.requestId, "evtB should have requestId");
		assert.notStrictEqual(
			evtA.params.payload.requestId,
			evtB.params.payload.requestId,
			"requestIds should differ",
		);

		// Validate request events have matching requestIds
		const reqA = requests.find(
			(r) =>
				r.params?.payload?.url?.endsWith("/a.js") &&
				r.params?.payload?.requestId === evtA.params.payload.requestId,
		);
		const reqB = requests.find(
			(r) =>
				r.params?.payload?.url?.endsWith("/b.js") &&
				r.params?.payload?.requestId === evtB.params.payload.requestId,
		);
		assert.ok(reqA, "Matching requestWillBeSent for /a.js with same requestId");
		assert.ok(reqB, "Matching requestWillBeSent for /b.js with same requestId");

		// Validate capturedFile correlation and hash
		assert.ok(
			evtA.params.payload.capturedFile,
			"evtA should have capturedFile",
		);
		assert.ok(
			evtB.params.payload.capturedFile,
			"evtB should have capturedFile",
		);

		assert.strictEqual(
			evtA.params.payload.capturedFile.originalUrl,
			evtA.params.payload.url,
			"evtA capturedFile.originalUrl should match payload.url",
		);
		assert.strictEqual(
			evtB.params.payload.capturedFile.originalUrl,
			evtB.params.payload.url,
			"evtB capturedFile.originalUrl should match payload.url",
		);

		assert.strictEqual(
			evtA.params.payload.capturedFile.hash,
			sha256Hex(aBody),
			"evtA capturedFile.hash should match body of /a.js",
		);
		assert.strictEqual(
			evtB.params.payload.capturedFile.hash,
			sha256Hex(bBody),
			"evtB capturedFile.hash should match body of /b.js",
		);
	} finally {
		// Cleanup
		try {
			await runner.cleanup();
		} catch {}
		server.close();
	}
});
