/**
 * WorkerLoop Orchestrator
 *
 * Orchestrates the main worker loop that:
 * 1. Polls for jobs using idle long-poll
 * 2. Executes jobs using PuppeteerRunner
 * 3. Sends heartbeats during execution
 * 4. Reports completion or failure
 * 5. Handles graceful shutdown
 */

import { type Job, JobClient } from "./job-client.js";
import { logger } from "./logger.js";
import { PuppeteerRunner } from "./puppeteer-runner.js";
import type { Config, WorkerConfig } from "./types.js";

export interface WorkerLoopConfig {
	readonly worker: WorkerConfig;
	readonly config: Config;
}

export interface WorkerLoopStats {
	readonly jobsProcessed: number;
	readonly jobsCompleted: number;
	readonly jobsFailed: number;
	readonly uptime: number;
	readonly lastJobAt?: Date;
}

export class WorkerLoop {
	private readonly jobClient: JobClient;
	private readonly workerConfig: WorkerConfig;
	private readonly config: Config;
	private readonly stats: {
		jobsProcessed: number;
		jobsCompleted: number;
		jobsFailed: number;
		startTime: number;
		lastJobAt?: Date;
	};

	private isRunning = false;
	private shouldStop = false;
	private _currentJob: Job | null = null;
	private heartbeatInterval: NodeJS.Timeout | null = null;
	private heartbeatInFlight = false;
	private abortController: AbortController | null = null;
	private consecutiveReserveErrors = 0;

	constructor(config: WorkerLoopConfig) {
		this.workerConfig = config.worker;
		this.config = config.config;
		this.jobClient = JobClient.fromConfig({ worker: this.workerConfig });
		this.stats = {
			jobsProcessed: 0,
			jobsCompleted: 0,
			jobsFailed: 0,
			startTime: 0, // Will be set when start() is called
		};
	}

	/**
	 * Start the worker loop
	 */
	async start(): Promise<void> {
		if (this.isRunning) {
			throw new Error("WorkerLoop is already running");
		}

		this.isRunning = true;
		this.shouldStop = false;
		this.stats.startTime = Date.now(); // Set start time when actually starting
		this.abortController = new AbortController();

		logger.info("Starting WorkerLoop", {
			jobType: this.workerConfig.jobType,
			leaseSeconds: this.workerConfig.leaseSeconds,
			waitSeconds: this.workerConfig.waitSeconds,
			heartbeatSeconds: this.workerConfig.heartbeatSeconds,
		});

		try {
			await this.runLoop();
		} finally {
			this.cleanup();
		}
	}

	/**
	 * Stop the worker loop gracefully
	 */
	async stop(): Promise<void> {
		if (!this.isRunning) {
			return;
		}

		logger.info("Stopping WorkerLoop gracefully...");
		this.shouldStop = true;

		// Cancel any ongoing operations
		if (this.abortController) {
			this.abortController.abort();
		}

		// Wait for current job to complete or timeout
		const maxWaitMs = 30000; // 30 seconds
		const startWait = Date.now();

		while (this.isRunning && Date.now() - startWait < maxWaitMs) {
			await new Promise((resolve) => setTimeout(resolve, 100));
		}

		if (this.isRunning) {
			logger.warn("WorkerLoop did not stop gracefully within timeout");
		}
	}

	/**
	 * Get current worker statistics
	 */
	getStats(): WorkerLoopStats {
		return {
			jobsProcessed: this.stats.jobsProcessed,
			jobsCompleted: this.stats.jobsCompleted,
			jobsFailed: this.stats.jobsFailed,
			uptime: Date.now() - this.stats.startTime,
			lastJobAt: this.stats.lastJobAt,
		};
	}

	/**
	 * Get current running state
	 */
	get running(): boolean {
		return this.isRunning;
	}

	/**
	 * Get currently processing job
	 */
	get currentJob(): Job | null {
		return this._currentJob;
	}

	/**
	 * Main worker loop implementation
	 */
	private async runLoop(): Promise<void> {
		while (!this.shouldStop) {
			try {
				// Long-poll for next job
				const job = await this.reserveNextJob();

				if (!job) {
					// No job available, continue polling
					continue;
				}

				// Process the job
				await this.processJob(job);
				// Reset error count on successful job processing
				this.consecutiveReserveErrors = 0;
			} catch (error) {
				if (this.shouldStop) {
					break;
				}

				// Enhanced error handling with better categorization
				const errorType = this.categorizeError(error);
				const shouldBackoff = this.shouldBackoffForError(errorType, error);

				logger.error(error, "Error in worker loop, continuing...", {
					errorType,
					consecutiveErrors: this.consecutiveReserveErrors + 1,
					willBackoff: shouldBackoff,
				});

				if (shouldBackoff) {
					// Exponential backoff on errors to avoid hot loops
					this.consecutiveReserveErrors++;
					const backoffMs = Math.min(
						30000,
						1000 * 2 ** (this.consecutiveReserveErrors - 1),
					);

					// Log backoff strategy for better observability
					logger.info("Backing off due to error", {
						backoffMs,
						consecutiveErrors: this.consecutiveReserveErrors,
						errorType,
					});

					await this.sleep(backoffMs);
				} else {
					// For transient errors, just continue with minimal delay
					await this.sleep(100);
				}
			}
		}

		logger.info("WorkerLoop stopped");
	}

	/**
	 * Reserve next job using long-poll
	 */
	private async reserveNextJob(): Promise<Job | null> {
		try {
			logger.debug("Polling for next job...");

			const job = await this.jobClient.reserveNext(this.workerConfig.jobType, {
				leaseSeconds: this.workerConfig.leaseSeconds,
				waitSeconds: this.workerConfig.waitSeconds,
				signal: this.abortController?.signal,
			});

			if (job) {
				logger.info("Reserved job", { jobId: job.id, jobType: job.type });
			}

			return job;
		} catch (error) {
			if (this.shouldStop) {
				return null;
			}

			logger.error(error, "Failed to reserve job");
			throw error;
		}
	}

	/**
	 * Process a single job
	 */
	private async processJob(job: Job): Promise<void> {
		this._currentJob = job;
		this.stats.jobsProcessed++;
		this.stats.lastJobAt = new Date();

		// Create a safe payload preview for logging
		const payloadPreview =
			typeof job.payload === "string"
				? job.payload.slice(0, 200) + (job.payload.length > 200 ? "..." : "")
				: "[object payload]";

		logger.info("Processing job", {
			jobId: job.id,
			jobType: job.type,
			payloadPreview,
		});

		// Start heartbeat
		this.startHeartbeat(job.id);

		try {
			// Execute the job
			const result = await this.executeJob(job);

			// Check if script execution succeeded or failed
			if (result.success) {
				// Complete the job successfully
				const completed = await this.jobClient.complete(
					job.id,
					this.abortController?.signal,
				);
				if (completed) {
					this.stats.jobsCompleted++;
					logger.info("Job completed successfully", {
						jobId: job.id,
						executionTime: result.executionTime,
						eventCount: result.eventCount,
						fileCount: result.fileCount,
					});
				} else {
					logger.warn("Failed to mark job as completed", { jobId: job.id });
				}
			} else {
				// Script failed - mark job as failed
				const errorMessage = result.error || "Script execution failed";
				logger.debug("Calling jobClient.fail()", {
					jobId: job.id,
					errorMessage,
					jobStatus: job.status,
				});
				const failed = await this.jobClient.fail(
					job.id,
					errorMessage,
					this.abortController?.signal,
				);
				logger.debug("jobClient.fail() returned", {
					jobId: job.id,
					failed,
				});
				if (failed) {
					this.stats.jobsFailed++;
					logger.info("Job failed due to script error", {
						jobId: job.id,
						error: errorMessage,
						executionTime: result.executionTime,
						eventCount: result.eventCount,
					});
				} else {
					logger.warn("Failed to mark job as failed (API returned false)", {
						jobId: job.id,
						jobStatus: job.status,
						errorMessage,
					});
				}
			}
		} catch (error) {
			// Fail the job unless we're shutting down/aborted
			const errorMessage =
				error instanceof Error ? error.message : String(error);
			const aborted =
				(error instanceof Error && error.name === "AbortError") ||
				this.abortController?.signal?.aborted ||
				this.shouldStop;

			if (aborted) {
				logger.info("Job processing aborted; skipping fail() due to shutdown", {
					jobId: job.id,
					err: error,
				});
			} else {
				try {
					const failed = await this.jobClient.fail(
						job.id,
						errorMessage,
						this.abortController?.signal,
					);

					if (failed) {
						this.stats.jobsFailed++;
						logger.error(error, "Job failed", {
							jobId: job.id,
						});
					} else {
						logger.error(error, "Job failed and could not mark as failed", {
							jobId: job.id,
						});
					}
				} catch (reportErr) {
					const reportAborted =
						reportErr instanceof Error && reportErr.name === "AbortError";
					if (
						reportAborted ||
						this.shouldStop ||
						this.abortController?.signal?.aborted
					) {
						logger.info("Job failed; fail() aborted due to shutdown", {
							jobId: job.id,
							err: error,
							reportErr,
						});
					} else {
						logger.error(error, "Job failed and fail() also errored", {
							jobId: job.id,
							reportErr,
						});
					}
				}
			}
		} finally {
			// Stop heartbeat
			this.stopHeartbeat();
			this._currentJob = null;
		}
	}

	/**
	 * Execute a job using PuppeteerRunner
	 */
	private async executeJob(job: Job) {
		// Extract script from job payload
		const script = this.extractScriptFromJob(job);

		// Create runner and execute
		const cfg = {
			...this.config,
			shipping: { ...(this.config.shipping ?? {}), sourceJobId: job.id },
		};
		const runner = new PuppeteerRunner();
		return await runner.runScript(script, cfg, this.abortController?.signal);
	}

	/**
	 * Extract script from job payload
	 */
	private extractScriptFromJob(job: Job): string {
		if (typeof job.payload === "string") {
			return job.payload;
		}

		if (typeof job.payload === "object" && job.payload !== null) {
			const payload = job.payload as Record<string, unknown>;
			const script = payload.script;
			if (typeof script === "string") {
				return script;
			}
			const url = payload.url;
			if (typeof url === "string" && url.length > 0) {
				// Generate a minimal script that navigates to the URL
				return `await page.goto(${JSON.stringify(url)}, { waitUntil: 'load' });`;
			}
		}

		throw new Error(
			`Invalid job payload: expected script string or object with script|url property`,
		);
	}

	/**
	 * Start sending heartbeats for the current job
	 */
	private startHeartbeat(jobId: string): void {
		if (this.heartbeatInterval) {
			clearInterval(this.heartbeatInterval);
		}

		const intervalMs = this.workerConfig.heartbeatSeconds * 1000;

		this.heartbeatInterval = setInterval(async () => {
			if (this.heartbeatInFlight || this.shouldStop) return;

			this.heartbeatInFlight = true;
			try {
				const success = await this.jobClient.heartbeat(
					jobId,
					this.workerConfig.leaseSeconds,
					this.abortController?.signal,
				);
				if (!success) {
					logger.warn("Heartbeat failed - job may have been reassigned", {
						jobId,
					});
				} else {
					logger.debug("Heartbeat sent", { jobId });
				}
			} catch (error) {
				logger.error(error, "Heartbeat error", { jobId });
			} finally {
				this.heartbeatInFlight = false;
			}
		}, intervalMs);
	}

	/**
	 * Stop sending heartbeats
	 */
	private stopHeartbeat(): void {
		if (this.heartbeatInterval) {
			clearInterval(this.heartbeatInterval);
			this.heartbeatInterval = null;
		}
	}

	/**
	 * Cleanup resources
	 */
	private cleanup(): void {
		this.isRunning = false;
		this.stopHeartbeat();

		if (this.abortController) {
			this.abortController.abort();
			this.abortController = null;
		}
	}

	/**
	 * Sleep for specified milliseconds
	 */
	private sleep(ms: number): Promise<void> {
		return new Promise((resolve) => setTimeout(resolve, ms));
	}

	/**
	 * Categorize errors for better handling and logging
	 */
	private categorizeError(error: unknown): string {
		if (!error) return "unknown";

		if (error instanceof Error) {
			// Network/connection errors
			if (
				error.name === "TypeError" &&
				error.message.includes("fetch failed")
			) {
				return "network_connection";
			}
			if (error.name === "AbortError") {
				return "request_aborted";
			}
			if (error.message.includes("ECONNREFUSED")) {
				return "connection_refused";
			}
			if (error.message.includes("ENOTFOUND")) {
				return "dns_resolution";
			}
			if (error.message.includes("timeout")) {
				return "timeout";
			}

			// HTTP errors
			if (error.message.includes("HTTP 5")) {
				return "server_error";
			}
			if (error.message.includes("HTTP 4")) {
				return "client_error";
			}

			// Job processing errors
			if (error.message.includes("Job execution")) {
				return "job_execution";
			}

			return "application_error";
		}

		return "unknown";
	}

	/**
	 * Determine if we should back off for this type of error
	 */
	private shouldBackoffForError(errorType: string, error: unknown): boolean {
		void error;
		switch (errorType) {
			case "network_connection":
			case "connection_refused":
			case "dns_resolution":
			case "server_error":
				// Always back off for infrastructure issues
				return true;

			case "timeout":
				// Back off for timeouts to avoid overwhelming the server
				return true;

			case "request_aborted":
				// Don't back off for aborted requests (likely shutdown)
				return false;

			case "client_error":
				// Don't back off for client errors (likely config issue)
				return false;

			case "job_execution":
				// Don't back off for job execution errors (job-specific issue)
				return false;

			default:
				// Conservative approach: back off for unknown errors
				return true;
		}
	}
}
