/**
 * File Capture System
 * Basic file capture with simple deduplication and temp storage
 */

import { createHash, randomUUID } from "node:crypto";
import * as IORedis from "ioredis";
import type { Redis as IORedisClient, RedisOptions } from "ioredis";
import { logger } from "./logger.js";
import type {
	EmbeddedFileContext,
	FileCaptureConfig,
	FileMetadata,
} from "./types.js";

type RedisClient = Pick<
	IORedisClient,
	"set" | "get" | "getBuffer" | "del" | "quit"
>;

type RedisStorageConfig = Partial<RedisOptions> & {
	redisClient?: RedisClient;
	prefix?: string;
	ttlSeconds?: number;
	hashTtlSeconds?: number;
	masterName?: string;
};

export interface CapturedFile {
	readonly url: string;
	readonly content: Buffer;
	readonly contentType: string;
	readonly sessionId: string;
}

function isRedisClient(value: unknown): value is RedisClient {
	if (!value || typeof value !== "object") return false;
	const candidate = value as Partial<RedisClient>;
	return (
		typeof candidate.set === "function" &&
		typeof candidate.get === "function" &&
		typeof candidate.getBuffer === "function" &&
		typeof candidate.del === "function" &&
		typeof candidate.quit === "function"
	);
}

export interface StorageProvider {
	store(key: string, content: Buffer): Promise<void>;
	retrieve(key: string): Promise<Buffer | null>;
	delete(key: string): Promise<void>;
	cleanup(): Promise<void>;
}

export class MemoryStorageProvider implements StorageProvider {
	private storage = new Map<string, Buffer>();

	async store(key: string, content: Buffer): Promise<void> {
		this.storage.set(key, content);
	}

	async retrieve(key: string): Promise<Buffer | null> {
		return this.storage.get(key) || null;
	}

	async delete(key: string): Promise<void> {
		this.storage.delete(key);
	}

	async cleanup(): Promise<void> {
		this.storage.clear();
	}

	getSize(): number {
		return this.storage.size;
	}
}

/**
 * Redis-backed storage provider with simple key prefixing and TTL handling.
 * Also exposes a small cross-run dedupe index keyed by content hash.
 */
export class RedisStorageProvider implements StorageProvider {
	private readonly redis: RedisClient;
	private readonly ttlSeconds: number;
	private readonly hashTtlSeconds: number;
	private readonly prefix: string;
	private readonly ownsClient: boolean;

	constructor(opts: {
		redis: RedisClient;
		ttlSeconds?: number;
		hashTtlSeconds?: number;
		prefix?: string;
		ownsClient?: boolean;
	}) {
		this.redis = opts.redis;
		this.ttlSeconds = opts.ttlSeconds ?? 60 * 60; // default 1 hour for files
		this.hashTtlSeconds = opts.hashTtlSeconds ?? 24 * 60 * 60; // default 24 hours for hash index
		this.prefix = `${(opts.prefix ?? "filecap:").replace(/:+$/, "")}:`;
		this.ownsClient = opts.ownsClient ?? true;
	}

	private k(key: string): string {
		return this.prefix + key;
	}

	async store(key: string, content: Buffer): Promise<void> {
		// Store with TTL; overwrite to refresh if same key appears again
		await this.redis.set(this.k(key), content, "EX", this.ttlSeconds);
	}

	async retrieve(key: string): Promise<Buffer | null> {
		const result = await this.redis.getBuffer(this.k(key));
		return result || null;
	}

	async delete(key: string): Promise<void> {
		await this.redis.del(this.k(key));
	}

	async cleanup(): Promise<void> {
		// Close our Redis connection if we created it
		try {
			if (
				this.ownsClient &&
				this.redis &&
				typeof this.redis.quit === "function"
			) {
				await this.redis.quit();
			}
		} catch (_err) {
			// ignore shutdown errors
		}
	}

	// Cross-run dedupe index: hash -> storageKey
	async getStorageKeyByHash(hash: string): Promise<string | null> {
		return (await this.redis.get(this.k(`h:${hash}`))) || null;
	}

	async setStorageKeyForHash(hash: string, storageKey: string): Promise<void> {
		await this.redis.set(
			this.k(`h:${hash}`),
			storageKey,
			"EX",
			this.hashTtlSeconds,
		);
	}
}

export class FileCapture {
	private deduplicationCache = new Map<string, string>(); // hash -> fileId
	private capturedFiles = new Set<string>(); // fileIds
	private readonly fileKeyById = new Map<
		string,
		{ key: string; storedByThisInstance: boolean }
	>();
	private storageProvider: StorageProvider;
	private readonly ctMatchers: string[];

	constructor(
		private readonly config: FileCaptureConfig,
		private readonly sessionId: string,
		deps?: { storageProvider?: StorageProvider },
	) {
		this.storageProvider =
			deps?.storageProvider ?? this.createStorageProvider();
		// Pre-normalize content-type matchers for performance
		this.ctMatchers = (this.config.contentTypeMatchers ?? []).map((m) =>
			String(m).toLowerCase(),
		);
	}

	async captureFile(file: CapturedFile): Promise<EmbeddedFileContext | null> {
		if (!this.config.enabled) {
			return null;
		}

		// Check file size limit
		if (
			this.config.maxFileSize &&
			file.content.length > this.config.maxFileSize
		) {
			return null;
		}

		// Check if file type should be captured (by resource type or content-type matchers)
		const resourceType = this.getResourceType(file.url, file.contentType);
		const matchesCT = this.matchesContentType(file.contentType);
		if (!this.config.types.includes(resourceType) && !matchesCT) {
			return null;
		}

		// Calculate hash for deduplication
		const hash = createHash("sha256").update(file.content).digest("hex");

		// 1) In-run (process local) dedupe
		const existingFileId = this.deduplicationCache.get(hash);
		if (existingFileId) {
			return this.createFileContext(existingFileId, file, hash, "duplicate");
		}

		// 2) Cross-run dedupe via Redis index (if available)

		if (this.storageProvider instanceof RedisStorageProvider) {
			try {
				const foundKey = await this.storageProvider.getStorageKeyByHash(hash);
				if (foundKey) {
					// Do not store again; reuse previous storageKey
					const fileId = randomUUID();
					// Still record in local maps to avoid rework in this run
					this.deduplicationCache.set(hash, fileId);
					this.capturedFiles.add(fileId);
					this.fileKeyById.set(fileId, {
						key: foundKey,
						storedByThisInstance: false,
					});
					return this.createFileContext(
						fileId,
						file,
						hash,
						"duplicate",
						foundKey,
					);
				}
			} catch (_err) {
				// Fall through to storing
			}
		}

		// Store new file
		const fileId = randomUUID();
		const storageKey = `${this.sessionId}/${fileId}`;

		try {
			await this.storageProvider.store(storageKey, file.content);

			// Update caches
			this.deduplicationCache.set(hash, fileId);
			this.capturedFiles.add(fileId);
			this.fileKeyById.set(fileId, {
				key: storageKey,
				storedByThisInstance: true,
			});

			// Update cross-run dedupe index if using Redis
			if (this.storageProvider instanceof RedisStorageProvider) {
				await this.storageProvider.setStorageKeyForHash(hash, storageKey);
			}

			return this.createFileContext(
				fileId,
				file,
				hash,
				"new_content",
				storageKey,
			);
		} catch (error) {
			logger.error(error, "Failed to store file");
			return null;
		}
	}

	async retrieveFile(fileId: string): Promise<Buffer | null> {
		const meta = this.fileKeyById.get(fileId);
		if (!meta) return null;
		return await this.storageProvider.retrieve(meta.key);
	}

	async cleanup(): Promise<void> {
		// Delete only keys we actually stored in this instance
		for (const [fileId, meta] of this.fileKeyById) {
			if (!meta.storedByThisInstance) continue;
			try {
				await this.storageProvider.delete(meta.key);
			} catch (error) {
				logger.error(error, "Failed to delete file", { fileId });
			}
		}

		// Clear caches
		this.deduplicationCache.clear();
		this.capturedFiles.clear();
		this.fileKeyById.clear();

		// Cleanup storage provider
		await this.storageProvider.cleanup();
	}

	getStats() {
		return {
			totalFiles: this.capturedFiles.size,
			cacheSize: this.deduplicationCache.size,
			storageProvider: this.storageProvider.constructor.name,
		};
	}

	private createStorageProvider(): StorageProvider {
		switch (this.config.storage) {
			case "memory":
				return new MemoryStorageProvider();

			case "redis": {
				const sc = (this.config.storageConfig ?? {}) as RedisStorageConfig;
				// Allow DI via prebuilt client
				const injected = isRedisClient(sc.redisClient)
					? sc.redisClient
					: undefined;
				const prefix: string | undefined = sc.keyPrefix ?? sc.prefix;
				const ttlSeconds: number | undefined = sc.ttlSeconds;
				const hashTtlSeconds: number | undefined = sc.hashTtlSeconds;

				let client: RedisClient;
				let ownsClient = true;
				if (injected) {
					client = injected;
					ownsClient = false;
				} else if (Array.isArray(sc.sentinels) && sc.sentinels.length > 0) {
					// Sentinel config
					const {
						redisClient: _redisClient,
						prefix: _prefix,
						ttlSeconds: _ttlSeconds,
						hashTtlSeconds: _hashTtlSeconds,
						masterName,
						...rest
					} = sc;
					const sentinelOptions: RedisOptions = {
						...rest,
						name: masterName ?? rest.name,
					} as RedisOptions;
					client = new IORedis.Redis(sentinelOptions);
				} else {
					// Single instance
					const {
						redisClient: _redisClient,
						prefix: _prefix,
						ttlSeconds: _ttlSeconds,
						hashTtlSeconds: _hashTtlSeconds,
						masterName: _masterName,
						...rest
					} = sc;
					const options: RedisOptions = {
						...rest,
						host: rest.host ?? "127.0.0.1",
						port: rest.port ?? 6379,
					} as RedisOptions;
					client = new IORedis.Redis(options);
				}
				return new RedisStorageProvider({
					redis: client,
					ttlSeconds,
					hashTtlSeconds,
					prefix,
					ownsClient,
				});
			}

			case "cloud":
				throw new Error(
					"Cloud storage not implemented in this simplified version",
				);

			default:
				return new MemoryStorageProvider();
		}
	}

	private createFileContext(
		fileId: string,
		file: CapturedFile,
		hash: string,
		captureReason: string,
		storageKeyOverride?: string,
	): EmbeddedFileContext {
		const metadata: FileMetadata = {
			captureReason,
			sessionId: this.sessionId,
			encoding: this.detectEncoding(file.contentType),
		};

		return {
			fileId,
			originalUrl: file.url,
			contentType: file.contentType,
			size: file.content.length,
			hash,
			captureTimestamp: Date.now(),
			storageProvider: this.config.storage,
			storageKey: storageKeyOverride ?? `${this.sessionId}/${fileId}`,
			metadata,
		};
	}
	private matchesContentType(contentType: string): boolean {
		const ct = (contentType ?? "").toLowerCase();
		if (!ct) return false;
		return this.ctMatchers.some((m) => ct.includes(m));
	}

	private getResourceType(url: string, contentType: string): string {
		// Expanded resource type detection with broader content-type coverage
		const ct = (contentType ?? "").toLowerCase();
		let pathname = "";
		try {
			pathname = new URL(url).pathname.toLowerCase();
		} catch {
			pathname = url.split("?")[0].split("#")[0].toLowerCase();
		}
		if (
			ct.includes("javascript") ||
			ct.includes("ecmascript") ||
			pathname.endsWith(".js")
		) {
			return "script";
		}
		if (ct.includes("css") || pathname.endsWith(".css")) {
			return "stylesheet";
		}
		if (ct.includes("html") || pathname.endsWith(".html")) {
			return "document";
		}
		// Treat common data/text formats as documents for gating purposes
		if (ct.includes("json") || pathname.endsWith(".json")) {
			return "document";
		}
		if (
			ct.includes("xml") ||
			ct.includes("+xml") ||
			pathname.endsWith(".xml")
		) {
			return "document";
		}
		if (ct.startsWith("text/")) {
			return "document";
		}
		return "other";
	}

	private detectEncoding(contentType: string): string {
		const match = contentType.match(/charset=([^;]+)/i);
		return match ? match[1] : "utf-8";
	}
}
