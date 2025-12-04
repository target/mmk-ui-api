import { readFile } from "node:fs/promises";
import path from "node:path";
import { parse as parseYaml } from "yaml";
import { configSchema, loadFromSources } from "./config-schema.js";
import { logger } from "./logger.js";
import type { Config } from "./types.js";

function isPlainObject(v: unknown): v is Record<string, unknown> {
	return v !== null && typeof v === "object" && !Array.isArray(v);
}

type ConfigSource = "envVar" | "default";

const CONFIG_ENV_VAR = "PUPPETEER_WORKER_CONFIG" as const;

interface YamlConfigResult {
	data: Record<string, unknown>;
	path: string;
	source: ConfigSource;
}

interface CandidatePath {
	path: string;
	source: ConfigSource;
}

async function loadYamlConfigIfPresent(): Promise<
	YamlConfigResult | undefined
> {
	const envPathRaw = process.env[CONFIG_ENV_VAR];
	const envPath = envPathRaw?.trim() ?? "";
	const hasEnvPath = envPath.length > 0;

	const candidates: CandidatePath[] = hasEnvPath
		? [
				{
					path: path.isAbsolute(envPath)
						? envPath
						: path.resolve(process.cwd(), envPath),
					source: "envVar",
				},
			]
		: [
				{
					path: path.resolve(process.cwd(), "config/puppeteer-worker.yaml"),
					source: "default",
				},
				{
					path: path.resolve(process.cwd(), "puppeteer-worker.config.yaml"),
					source: "default",
				},
			];

	if (hasEnvPath) {
		logger.info(`${CONFIG_ENV_VAR} provided; attempting to load config`, {
			envVar: CONFIG_ENV_VAR,
			path: candidates[0]?.path,
		});
	} else {
		logger.info("No explicit config path set; checking default locations", {
			candidatePaths: candidates.map((c) => c.path),
		});
	}

	const searchedPaths: string[] = [];

	for (const candidate of candidates) {
		const { path: candidatePath, source } = candidate;
		searchedPaths.push(candidatePath);
		logger.debug("Attempting to load YAML config", {
			path: candidatePath,
			source,
		});
		try {
			const content = await readFile(candidatePath, "utf8");
			const parsed = parseYaml(content);
			if (!isPlainObject(parsed)) {
				throw new Error("YAML root must be a mapping/object");
			}
			logger.info("Loaded YAML config file", {
				path: candidatePath,
				source,
			});
			return {
				data: parsed,
				path: candidatePath,
				source,
			};
		} catch (err: unknown) {
			if (isFileNotFoundError(err)) {
				if (source === "envVar") {
					logger.warn("Config file not found at env-provided path", {
						envVar: CONFIG_ENV_VAR,
						path: candidatePath,
					});
				} else {
					logger.debug("Config file not found at default path", {
						path: candidatePath,
					});
				}
				continue;
			}
			logger.error(
				{ err, path: candidatePath, source },
				"Failed to load YAML config file",
			);
			throw new Error(`Failed to load config file at ${candidatePath}`, {
				cause: err,
			});
		}
	}

	logger.info(
		"No YAML config file located; falling back to environment variables",
		{
			searchedPaths,
			cwd: process.cwd(),
			...(hasEnvPath ? { envVar: CONFIG_ENV_VAR } : {}),
		},
	);
	return undefined;
}

function validateStorageConfigByType(cfg: Required<Config>): void {
	const storage = cfg.fileCapture.storage;
	const sc = cfg.fileCapture.storageConfig ?? {};
	const keys = Object.keys(sc);
	if (storage === "memory") {
		if (keys.length > 0)
			throw new Error(
				"Unknown option: fileCapture.storageConfig.* for memory storage",
			);
	} else if (storage === "redis") {
		// Support both single instance and Sentinel options + sane extras
		const allowed = new Set([
			"host",
			"port",
			"username",
			"password",
			"db",
			"keyPrefix",
			"prefix",
			"ttlSeconds",
			"hashTtlSeconds",
			"redisClient", // DI: allow injecting a pre-built client in tests
			// Sentinel
			"sentinels", // array of { host, port }
			"masterName",
			"sentinelPassword",
		]);
		for (const k of keys) {
			if (!allowed.has(k))
				throw new Error(`Unknown option: fileCapture.storageConfig.${k}`);
		}
	} else if (storage === "cloud") {
		const allowed = new Set([
			"provider",
			"bucket",
			"region",
			"accessKeyId",
			"secretAccessKey",
		]);
		for (const k of keys) {
			if (!allowed.has(k))
				throw new Error(`Unknown option: fileCapture.storageConfig.${k}`);
		}
	}
}

function validateEndpointUrl(cfg: Required<Config>): void {
	const url = cfg.shipping.endpoint;
	if (url) {
		try {
			// eslint-disable-next-line no-new
			new URL(url);
		} catch {
			throw new Error("Invalid shipping endpoint URL");
		}
	}
	const base = cfg.worker.apiBaseUrl;
	if (base) {
		try {
			// eslint-disable-next-line no-new
			new URL(base);
		} catch {
			throw new Error("Invalid worker.apiBaseUrl URL");
		}
	}
}

export async function loadConfig(): Promise<Required<Config>> {
	const yamlResult = await loadYamlConfigIfPresent();
	const merged = loadFromSources(configSchema, {
		yaml: yamlResult?.data,
		env: process.env,
	}) as Required<Config>;
	// Post-merge validations that require cross-field context
	validateStorageConfigByType(merged);
	// Derive shipping endpoint from apiBaseUrl if not explicitly set
	const resolved = ensureShippingEndpoint(merged);
	validateEndpointUrl(resolved);
	logger.info("Configuration loaded", {
		yamlSource: yamlResult
			? {
					path: yamlResult.path,
					source: yamlResult.source,
				}
			: null,
		envSource: "process.env",
	});
	return resolved;
}

function ensureShippingEndpoint(cfg: Required<Config>): Required<Config> {
	if (cfg.shipping.endpoint || !cfg.worker.apiBaseUrl) {
		return cfg;
	}
	return {
		...cfg,
		shipping: {
			...cfg.shipping,
			endpoint: new URL("/api/events/bulk", cfg.worker.apiBaseUrl).toString(),
		},
	};
}

function isFileNotFoundError(err: unknown): boolean {
	if (!err) return false;
	if (typeof err === "object" && "code" in err) {
		const code = (err as { code?: unknown }).code;
		if (code === "ENOENT") return true;
	}
	return /ENOENT/.test(String(err));
}
