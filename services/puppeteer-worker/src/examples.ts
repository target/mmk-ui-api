/**
 * Integration Examples for Puppeteer Worker
 * Clear examples showing how to use the simplified system
 */

import { loadConfig } from "./config-loader.js";
import { logger } from "./logger.js";
import { PuppeteerRunner } from "./puppeteer-runner.js";
import type { Config, SelfContainedEvent } from "./types.js";

// ============================================================================
// BASIC USAGE EXAMPLES
// ============================================================================

/**
 * Example 1: Basic Script Execution
 * Simplest possible usage - just run a script and get events
 */
export async function basicScriptExecution() {
	const runner = new PuppeteerRunner();

	const script = `
    console.log('Starting test...');
    await page.goto('https://example.com');
    console.log('Page loaded');
  `;

	const result = await runner.runScript(script);

	logger.info(`Execution completed: ${result.success}`);
	logger.info(`Events captured: ${result.eventCount}`);
	logger.info(`Execution time: ${result.executionTime}ms`);

	return result;
}

/**
 * Example 2: Script with Client-Side Monitoring
 * Captures localStorage operations and dynamic code execution
 */
export async function scriptWithClientMonitoring() {
	const runner = new PuppeteerRunner();

	const config: Config = {
		headless: true,
		clientMonitoring: {
			enabled: true,
			events: ["storage", "dynamicCode"],
		},
	};

	const script = `
    await page.goto('https://example.com');

    // These operations will be captured by client-side monitoring
    await page.evaluate(() => {
      localStorage.setItem('user-id', '12345');
      localStorage.setItem('session-token', 'abc123xyz');

      eval('console.log("Dynamic code executed")');

      new Function('console.log("Function constructor used")')();
    });
  `;

	const result = await runner.runScript(script, config);
	const events = runner.getEvents();

	logger.info("Captured events:");
	events.forEach((event) => {
		logger.info("Captured event", {
			method: event.method,
			payload: event.params.payload,
		});
	});

	return result;
}

/**
 * Example 3: File Capture Configuration
 * Captures JavaScript, CSS, and HTML files with deduplication
 */
export async function scriptWithFileCapture() {
	const runner = new PuppeteerRunner();

	const config: Config = {
		headless: true,
		fileCapture: {
			enabled: true,
			types: ["script", "document", "stylesheet"],
			maxFileSize: 512 * 1024, // 512KB
			storage: "memory",
		},
	};

	const script = `
    await page.goto('https://example.com');

    // Navigate to a few pages to capture different files
    await page.goto('https://httpbin.org/html');
  `;

	const result = await runner.runScript(script, config);

	logger.info(`Files captured: ${result.fileCount}`);

	return result;
}

/**
 * Example 4: Event Shipping to Downstream API
 * Ships captured events to a downstream processing service
 */
export async function scriptWithEventShipping() {
	const runner = new PuppeteerRunner();

	const config: Config = {
		headless: true,
		shipping: {
			endpoint: "https://api.example.com/events/batch",
			batchSize: 50,
			maxBatchAge: 5000,
		},
		clientMonitoring: {
			enabled: true,
			events: ["storage", "dynamicCode"],
		},
	};

	const script = `
    await page.goto('about:blank');
    await page.setContent('<html><body><h1>Test</h1></body></html>');

    await page.evaluate(() => {
      for (let i = 0; i < 10; i++) {
        localStorage.setItem(\`key-\${i}\`, \`value-\${i}\`);
      }
    });
  `;

	const result = await runner.runScript(script, config);

	logger.info("Events shipped to downstream API");

	return result;
}

// ============================================================================
// CONFIGURATION EXAMPLES
// ============================================================================

/**
 * Example 5: Environment-Based Configuration
 * Uses environment variables for configuration
 */
export async function environmentBasedConfiguration() {
	// Set environment variables (in real usage, these would be set externally)
	process.env.PUPPETEER_HEADLESS = "true";
	process.env.FILE_CAPTURE_ENABLED = "true";
	process.env.FILE_CAPTURE_TYPES = "script,document";
	process.env.CLIENT_MONITORING_EVENTS = "storage,dynamicCode";

	const config = await loadConfig();
	const runner = new PuppeteerRunner();

	const script = `
    await page.goto('https://example.com');
    await page.evaluate(() => {
      localStorage.setItem('env-test', 'configured-from-env');
    });
  `;

	const result = await runner.runScript(script, config);

	return result;
}

/**
 * Example 6: Test Configuration
 * Optimized configuration for testing scenarios
 */
export async function testConfiguration() {
	const config: Config = {
		headless: true,
		timeout: 5000,
		clientMonitoring: {
			enabled: true,
			events: ["storage"],
		},
	};

	const runner = new PuppeteerRunner();

	const script = `
    await page.goto('about:blank');
    await page.setContent('<html><body><h1>Test</h1></body></html>');
    await page.evaluate(() => {
      localStorage.setItem('test-key', 'test-value');
    });
  `;

	const result = await runner.runScript(script, config);

	return result;
}

// ============================================================================
// ADVANCED USAGE EXAMPLES
// ============================================================================

/**
 * Example 7: Custom Event Processing
 * Shows how to process events after capture
 */
export async function customEventProcessing() {
	const runner = new PuppeteerRunner();

	const config: Config = {
		headless: true,
		clientMonitoring: {
			enabled: true,
			events: ["storage", "dynamicCode"],
		},
	};

	const script = `
    await page.goto('about:blank');
    await page.setContent('<html><body><h1>Security Test</h1></body></html>');

    await page.evaluate(() => {
      // Simulate basic monitoring activities
      localStorage.setItem('test-data', 'test-value');
      eval('console.log("Dynamic code executed")');
    });
  `;

	const result = await runner.runScript(script, config);
	const events = runner.getEvents();

	// Process events by category
	const eventsByCategory = events.reduce(
		(acc, event) => {
			const category = event.metadata.category;
			if (!acc[category]) acc[category] = [];
			acc[category].push(event);
			return acc;
		},
		{} as Record<string, SelfContainedEvent[]>,
	);

	logger.info("Events by category:");
	Object.entries(eventsByCategory).forEach(([category, categoryEvents]) => {
		logger.info(`${category}: ${categoryEvents.length} events`);
	});

	// All events are now low severity (no security checks)
	logger.info("All events processed with simplified monitoring");

	return { result, events };
}

/**
 * Example 8: Batch Processing Multiple Scripts
 * Process multiple scripts in sequence
 */
export async function batchProcessing() {
	const scripts = [
		'await page.goto("https://example.com");',
		'await page.goto("https://httpbin.org/html");',
		'await page.evaluate(() => localStorage.setItem("batch-test", "script-1"));',
	];

	const config: Config = {
		headless: true,
		clientMonitoring: { enabled: true, events: ["storage"] },
	};

	const results = [];

	for (let i = 0; i < scripts.length; i++) {
		const runner = new PuppeteerRunner();
		const result = await runner.runScript(scripts[i], config);

		results.push({
			scriptIndex: i,
			success: result.success,
			eventCount: result.eventCount,
			executionTime: result.executionTime,
		});

		logger.info(`Script ${i + 1} completed: ${result.eventCount} events`);
	}

	return results;
}

// ============================================================================
// ERROR HANDLING EXAMPLES
// ============================================================================

/**
 * Example 9: Error Handling and Recovery
 * Shows proper error handling patterns
 */
export async function errorHandlingExample() {
	const runner = new PuppeteerRunner();

	const config: Config = {
		headless: true,
		timeout: 5000,
	};

	// Script that will cause an error
	const faultyScript = `
    await page.goto('https://invalid-url-that-does-not-exist.com');
  `;

	try {
		const result = await runner.runScript(faultyScript, config);

		if (!result.success) {
			logger.warn("Script execution failed as expected", {
				error: result.error,
				eventCount: result.eventCount,
			});
		}

		return result;
	} catch (error) {
		logger.error(error, "Caught exception");
		throw error;
	}
}

// ============================================================================
// EXPORT ALL EXAMPLES
// ============================================================================

export const EXAMPLES = {
	basicScriptExecution,
	scriptWithClientMonitoring,
	scriptWithFileCapture,
	scriptWithEventShipping,
	environmentBasedConfiguration,
	testConfiguration,
	customEventProcessing,
	batchProcessing,
	errorHandlingExample,
};

// Helper function to run all examples
export async function runAllExamples() {
	logger.info("Running all examples...\n");

	for (const [name, exampleFn] of Object.entries(EXAMPLES)) {
		logger.info(`\n=== ${name} ===`);
		try {
			await exampleFn();
			logger.info("✅ Success");
		} catch (error) {
			logger.error(error, "❌ Failed");
		}
	}
}

// CLI runner for examples
if (import.meta.url === `file://${process.argv[1]}`) {
	const exampleName = process.argv[2];

	if (exampleName && EXAMPLES[exampleName as keyof typeof EXAMPLES]) {
		logger.info(`Running example: ${exampleName}`);
		EXAMPLES[exampleName as keyof typeof EXAMPLES]()
			.then(() => logger.info("Example completed"))
			.catch((error) => {
				logger.error(error, "Example failed");
				process.exit(1);
			});
	} else {
		logger.info("Available examples:");
		Object.keys(EXAMPLES).forEach((name) => {
			logger.info(`- ${name}`);
		});
		logger.info("\nUsage: node examples.js <example-name>");
		logger.info("       node examples.js (to run all examples)");

		if (!exampleName) {
			runAllExamples().catch((err) =>
				logger.error(err, "Error running all examples"),
			);
		}
	}
}
