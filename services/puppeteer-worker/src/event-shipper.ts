/**
 * Event Shipper
 * Ships events to downstream task queues with basic batching
 */

import { createHash, randomUUID } from "node:crypto";
import { logger } from "./logger.js";
import type {
	BatchMetadata,
	BatchSequenceInfo,
	ChecksumInfo,
	EventBatch,
	SelfContainedEvent,
	ShippingConfig,
} from "./types.js";

export class EventShipper {
	private batchSequence = 0;

	constructor(private readonly config: ShippingConfig) {}

	async shipEvents(events: SelfContainedEvent[]): Promise<void> {
		if (events.length === 0) {
			return;
		}

		const batches = this.createBatches(events);

		for (const batch of batches) {
			await this.shipBatch(batch);
		}
	}

	private createBatches(events: SelfContainedEvent[]): EventBatch[] {
		const configured = this.config.batchSize ?? 100;
		const batchSize =
			Number.isFinite(configured) && configured > 0 ? configured : 100;
		const batches: EventBatch[] = [];

		for (let i = 0; i < events.length; i += batchSize) {
			const batchEvents = events.slice(i, i + batchSize);
			const batch = this.createBatch(
				batchEvents,
				i === 0,
				i + batchSize >= events.length,
			);
			batches.push(batch);
		}

		return batches;
	}

	private createBatch(
		events: SelfContainedEvent[],
		isFirst: boolean,
		isLast: boolean,
	): EventBatch {
		const batchId = randomUUID();
		const sessionId = events[0]?.params.sessionId || "unknown";
		const batchContent = JSON.stringify(events);
		const totalSize = Buffer.byteLength(batchContent, "utf8");

		const batchMetadata: BatchMetadata = {
			createdAt: Date.now(),
			eventCount: events.length,
			totalSize,
			checksumInfo: this.createChecksum(batchContent),
			retryCount: 0,
			...(this.config.sourceJobId ? { jobId: this.config.sourceJobId } : {}),
		};

		const sequenceInfo: BatchSequenceInfo = {
			sequenceNumber: ++this.batchSequence,
			isFirstBatch: isFirst,
			isLastBatch: isLast,
		};

		return {
			batchId,
			sessionId,
			events,
			batchMetadata,
			sequenceInfo,
		};
	}

	private createChecksum(content: string): ChecksumInfo {
		const hash = createHash("sha256").update(content, "utf8").digest("hex");
		return {
			algorithm: "sha256",
			value: hash,
		};
	}

	private async shipBatch(batch: EventBatch): Promise<void> {
		if (!this.config.endpoint) {
			logger.info("No shipping endpoint configured, skipping batch shipment");
			return;
		}

		const timeoutMs = 10000; // safeguard against hanging requests
		try {
			const controller = new AbortController();
			const timeoutId = setTimeout(() => controller.abort(), timeoutMs);

			try {
				const response = await fetch(this.config.endpoint, {
					method: "POST",
					headers: {
						"Content-Type": "application/json",
						"X-Batch-Id": batch.batchId,
						"X-Session-Id": batch.sessionId,
					},
					body: JSON.stringify(batch),
					signal: controller.signal,
				});

				if (!response.ok) {
					const body = await response.text().catch(() => "");
					throw new Error(
						`HTTP ${response.status}: ${response.statusText}${body ? ` - ${body}` : ""}`,
					);
				}

				logger.info(
					`Successfully shipped batch ${batch.batchId} with ${batch.batchMetadata.eventCount} events`,
				);
			} finally {
				clearTimeout(timeoutId);
			}
		} catch (error) {
			logger.error(error, "Failed to ship batch", {
				batchId: batch.batchId,
				endpoint: this.config.endpoint,
				timeoutMs,
				eventCount: batch.batchMetadata.eventCount,
			});
			throw error;
		}
	}
}
