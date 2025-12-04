/**
 * Puppeteer Worker - Main Library Entry Point
 *
 * Exports core functionality for browser automation and event monitoring.
 * When run directly, executes a simple demo script.
 */

import { loadConfig } from "./config-loader.js";
import { logger } from "./logger.js";
import { PuppeteerRunner } from "./puppeteer-runner.js";

/**
 * Simple demo for development/testing
 * Run with: npm run start, npm run dev, or npm run demo
 */
async function runDemo(): Promise<void> {
	const runner = new PuppeteerRunner();
	const config = await loadConfig();

	logger.info("Starting Puppeteer Worker demo...");
	logger.info(
		`Config: headless=${config.headless}, clientMonitoring=${config.clientMonitoring?.enabled}`,
	);

	const demoScript = `
    console.log('Demo: navigating to test page...');
    await page.goto('about:blank');
    await page.setContent('<html><body><h1>Demo Page</h1></body></html>');
    console.log('Demo: page loaded');
  `;

	try {
		const result = await runner.runScript(demoScript, config);
		logger.info(`Demo completed in ${result.executionTime.toFixed(2)}ms`);
		logger.info(
			`Captured ${result.eventCount} events, ${result.fileCount} files`,
		);
	} catch (error) {
		logger.error(error, "Demo failed");
		process.exit(1);
	}
}

// Run demo if executed directly
if (import.meta.url === `file://${process.argv[1]}`) {
	runDemo().catch(console.error);
}

export { loadConfig } from "./config-loader.js";
export { EventMonitor } from "./event-monitor.js";
export { EventShipper } from "./event-shipper.js";
export * as Examples from "./examples.js";
export { FileCapture } from "./file-capture.js";
export { JobClient } from "./job-client.js";
// Export main components for use as a library
export { PuppeteerRunner } from "./puppeteer-runner.js";
export * from "./types.js";
export { WorkerLoop } from "./worker-loop.js";
