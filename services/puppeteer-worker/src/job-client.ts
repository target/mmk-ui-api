import type { WorkerConfig } from "./types.js";

export type JobType = "browser" | "rules";
export type JobStatus = "pending" | "running" | "completed" | "failed";

export interface Job {
	id: string;
	type: JobType;
	status: JobStatus;
	priority?: number;
	payload?: unknown;
	metadata?: unknown;
	session_id?: string | null;
	scheduled_at?: string;
	started_at?: string | null;
	completed_at?: string | null;
	retry_count?: number;
	max_retries?: number;
	last_error?: string | null;
	lease_expires_at?: string | null;
	created_at?: string;
	updated_at?: string;
}

interface RequestOptions {
	method?: string;
	headers?: Record<string, string>;
	body?: BodyInit | null;
	timeoutMs?: number;
	retries?: number;
	signal?: AbortSignal;
}

export class JobClient {
	private readonly baseUrl: string;
	private readonly defaultTimeout: number;
	private readonly defaultRetries: number;

	constructor(
		apiBaseUrl: string,
		opts?: { timeoutMs?: number; retries?: number },
	) {
		this.baseUrl = apiBaseUrl.replace(/\/$/, "");
		this.defaultTimeout = opts?.timeoutMs ?? 15000;
		this.defaultRetries = Math.max(0, Math.min(3, opts?.retries ?? 1));
	}

	static fromConfig(cfg: Required<{ worker: WorkerConfig }>): JobClient {
		if (!cfg.worker.apiBaseUrl) {
			throw new Error("worker.apiBaseUrl is required to create JobClient");
		}
		return new JobClient(cfg.worker.apiBaseUrl);
	}

	async reserveNext(
		jobType: JobType,
		options: {
			leaseSeconds: number;
			waitSeconds: number;
			signal?: AbortSignal;
		},
	): Promise<Job | null> {
		const url = this.url(
			`/api/jobs/${jobType}/reserve_next?lease=${options.leaseSeconds}&wait=${options.waitSeconds}`,
		);
		const res = await this.request(url, {
			method: "GET",
			timeoutMs: options.waitSeconds * 1000 + 5000,
			signal: options.signal,
		});
		if (res.status === 204) return null;
		await this.ensureOk(res, "reserve_next");
		return (await res.json()) as Job;
	}

	async heartbeat(
		jobId: string,
		extendSeconds: number,
		signal?: AbortSignal,
	): Promise<boolean> {
		const url = this.url(
			`/api/jobs/${encodeURIComponent(jobId)}/heartbeat?extend=${extendSeconds}`,
		);
		const res = await this.request(url, { method: "POST", signal });
		if (res.status === 204) return true;
		await this.ensureOk(res, "heartbeat");
		const text = await res.text();
		const body = text ? (JSON.parse(text) as { ok?: unknown }) : {};
		return body.ok === true || res.status === 200;
	}

	async complete(jobId: string, signal?: AbortSignal): Promise<boolean> {
		const url = this.url(`/api/jobs/${encodeURIComponent(jobId)}/complete`);
		const res = await this.request(url, { method: "POST", signal });
		if (res.status === 204) return true;
		await this.ensureOk(res, "complete");
		const text = await res.text();
		const body = text ? (JSON.parse(text) as { ok?: unknown }) : {};
		return body.ok === true || res.status === 200;
	}

	async fail(
		jobId: string,
		error: string,
		signal?: AbortSignal,
	): Promise<boolean> {
		const url = this.url(`/api/jobs/${encodeURIComponent(jobId)}/fail`);
		const res = await this.request(url, {
			method: "POST",
			headers: { "Content-Type": "application/json" },
			body: JSON.stringify({ error }),
			signal,
		});
		if (res.status === 204) return true;
		await this.ensureOk(res, "fail");
		const text = await res.text();
		const body = text ? (JSON.parse(text) as { ok?: unknown }) : {};
		return body.ok === true || res.status === 200;
	}

	// --- internals ---
	private url(path: string): string {
		if (path.startsWith("http://") || path.startsWith("https://")) return path;
		return `${this.baseUrl}${path.startsWith("/") ? "" : "/"}${path}`;
	}

	private async request(url: string, opts: RequestOptions): Promise<Response> {
		const timeoutMs = opts.timeoutMs ?? this.defaultTimeout;
		const retries = opts.retries ?? this.defaultRetries;

		let attempt = 0;
		// eslint-disable-next-line no-constant-condition
		while (true) {
			// Check if external signal is already aborted
			if (opts.signal?.aborted) {
				throw new DOMException("Request was aborted", "AbortError");
			}

			const controller = new AbortController();
			const t = setTimeout(() => controller.abort(), timeoutMs);

			// Combine external signal with timeout signal
			const combinedSignal = opts.signal
				? AbortSignal.any([opts.signal, controller.signal])
				: controller.signal;

			try {
				const res = await fetch(url, {
					method: opts.method ?? "GET",
					headers: opts.headers,
					body: opts.body,
					signal: combinedSignal,
				});
				clearTimeout(t);
				if (this.shouldRetry(res) && attempt < retries) {
					attempt++;
					await this.backoff(attempt);
					continue;
				}
				return res;
			} catch (err: unknown) {
				clearTimeout(t);
				// Don't retry if external signal was aborted
				if (opts.signal?.aborted) {
					throw err;
				}
				if (this.isAbortError(err) || this.isNetworkError(err)) {
					if (attempt < retries) {
						attempt++;
						await this.backoff(attempt);
						continue;
					}
					// Retries exhausted: attach context for clearer logs
					const message =
						err instanceof Error && err.message ? err.message : String(err);
					throw new Error(
						`fetch failed for ${url}${message ? `: ${message}` : ""}`,
						{ cause: err },
					);
				}
				throw err;
			}
		}
	}

	private async ensureOk(res: Response, op: string): Promise<void> {
		if (!res.ok) {
			let snippet = "";
			try {
				snippet = (await res.text()).slice(0, 200);
			} catch {}
			throw new Error(
				`[${op}] HTTP ${res.status} ${res.statusText}${snippet ? `: ${snippet}` : ""}`,
			);
		}
	}

	private shouldRetry(res: Response): boolean {
		return res.status >= 500 && res.status < 600;
	}

	private isAbortError(err: unknown): boolean {
		return err instanceof Error && err.name === "AbortError";
	}

	private isNetworkError(err: unknown): boolean {
		return err instanceof TypeError;
	}

	private async backoff(attempt: number): Promise<void> {
		const base = 100; // ms
		const cap = 1000;
		const delay = Math.min(cap, base * 2 ** (attempt - 1));
		await new Promise((r) => setTimeout(r, delay));
	}
}
