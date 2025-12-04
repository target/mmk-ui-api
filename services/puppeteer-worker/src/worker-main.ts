/**
 * Worker Main Entry Point
 *
 * Main entry point for running the puppeteer worker in production.
 * Uses the WorkerLoop orchestrator to handle job processing.
 */

import { loadConfig } from "./config-loader.js";
import { logger } from "./logger.js";
import { WorkerLoop } from "./worker-loop.js";

/**
 * Signal handler for graceful shutdown
 */
class GracefulShutdown {
	private workerLoop: WorkerLoop | null = null;
	private isShuttingDown = false;

	constructor() {
		// Handle shutdown signals
		process.on("SIGTERM", () => this.handleShutdown("SIGTERM"));
		process.on("SIGINT", () => this.handleShutdown("SIGINT"));
		process.on("SIGHUP", () => this.handleShutdown("SIGHUP"));

		// Handle uncaught exceptions
		process.on("uncaughtException", (error) => {
			logger.fatal(error, "Uncaught exception");
			this.handleShutdown("uncaughtException");
		});

		// Handle unhandled promise rejections
		process.on("unhandledRejection", (reason, promise) => {
			logger.fatal(reason, "Unhandled promise rejection", {
				promise: String(promise),
			});
			this.handleShutdown("unhandledRejection");
		});
	}

	setWorkerLoop(workerLoop: WorkerLoop): void {
		this.workerLoop = workerLoop;
	}

	private async handleShutdown(signal: string): Promise<void> {
		if (this.isShuttingDown) {
			logger.warn(`Received ${signal} during shutdown, forcing exit`);
			process.exit(1);
		}

		this.isShuttingDown = true;
		logger.info(`Received ${signal}, shutting down gracefully...`);

		try {
			if (this.workerLoop) {
				await this.workerLoop.stop();
			}
			logger.info("Graceful shutdown completed");

			// Allow time for async logger transports to flush
			await new Promise((resolve) => setTimeout(resolve, 100));
			process.exit(0);
		} catch (error) {
			logger.error(error, "Error during graceful shutdown");

			// Allow time for async logger transports to flush
			await new Promise((resolve) => setTimeout(resolve, 100));
			process.exit(1);
		}
	}
}

/**
 * Main function
 */
async function main(): Promise<void> {
	try {
		// Load configuration
		const config = await loadConfig();

		// Validate worker configuration
		if (!config.worker?.apiBaseUrl) {
			throw new Error("worker.apiBaseUrl is required");
		}

		logger.info("Starting Puppeteer Worker", {
			jobType: config.worker.jobType,
			apiBaseUrl: config.worker.apiBaseUrl,
			leaseSeconds: config.worker.leaseSeconds,
			waitSeconds: config.worker.waitSeconds,
			heartbeatSeconds: config.worker.heartbeatSeconds,
		});

		// Setup graceful shutdown
		const shutdown = new GracefulShutdown();

		// Create and start worker loop
		const workerLoop = new WorkerLoop({
			worker: config.worker,
			config: config,
		});

		shutdown.setWorkerLoop(workerLoop);

		// Log stats periodically
		const statsInterval = setInterval(() => {
			const stats = workerLoop.getStats();
			logger.info("Worker stats", {
				jobsProcessed: stats.jobsProcessed,
				jobsCompleted: stats.jobsCompleted,
				jobsFailed: stats.jobsFailed,
				uptime: Math.round(stats.uptime / 1000),
				lastJobAt: stats.lastJobAt?.toISOString(),
			});
		}, 60000); // Every minute

		// Don't block process exit
		statsInterval.unref();

		// Start the worker loop
		await workerLoop.start();

		// Cleanup
		clearInterval(statsInterval);
	} catch (error) {
		logger.fatal(error, "Failed to start worker");

		// Allow time for async logger transports to flush
		await new Promise((resolve) => setTimeout(resolve, 100));
		process.exit(1);
	}
}

// Run if this file is executed directly
if (import.meta.url === `file://${process.argv[1]}`) {
	main().catch(async (error) => {
		logger.fatal(error, "Unhandled error in main");

		// Allow time for async logger transports to flush
		await new Promise((resolve) => setTimeout(resolve, 100));
		process.exit(1);
	});
}

export { main };
