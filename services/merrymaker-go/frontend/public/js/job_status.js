import { Lifecycle } from "./core/lifecycle.js";

class JobStatusPoller {
	constructor() {
		this.pollers = new Map();
		this.defaultInterval = 2000;
		this.maxAttempts = 30;
	}

	startPolling(jobId, element, options = {}) {
		if (!(jobId && element)) {
			return () => {};
		}

		let poller = this.pollers.get(jobId);
		const resolvedMaxAttempts = options.maxAttempts ?? this.maxAttempts;
		const config = {
			interval: options.interval ?? this.defaultInterval,
			maxAttempts: resolvedMaxAttempts,
			maxErrorAttempts: options.maxErrorAttempts ?? Math.min(3, resolvedMaxAttempts),
			onComplete: options.onComplete || null,
			onError: options.onError || null,
			showProgress: options.showProgress !== false,
		};

		if (poller) {
			poller.targets.add(element);
			this.updateStatus(element, jobId, { status: "pending" }, poller.config);
			return () => this.stopPollingForElement(jobId, element);
		}

		poller = {
			config,
			targets: new Set([element]),
			stopped: false,
			timerId: null,
			attempts: 0,
			inFlight: false,
			controller: null,
		};

		this.pollers.set(jobId, poller);

		const shouldStopPolling = () => poller.stopped || !this.pollers.has(jobId);
		const isTerminalStatus = (status) =>
			status.status === "completed" || status.status === "failed";

		const updateTargets = (status) => {
			for (const target of Array.from(poller.targets)) {
				if (!target?.isConnected) {
					poller.targets.delete(target);
					continue;
				}
				this.updateStatus(target, jobId, status, poller.config);
			}
		};

		const handleTimeout = () => {
			this.stopPolling(jobId);
			updateTargets({ status: "timeout" });
		};

		const handleSuccess = (status) => {
			updateTargets(status);
			if (isTerminalStatus(status)) {
				this.stopPolling(jobId);
				if (poller.config.onComplete) poller.config.onComplete(status);
				return true;
			}
			return false;
		};

		const handleError = (error) => {
			if (error.name === "AbortError") return false;

			console.error("Job status polling error:", error);
			if (poller.config.onError) poller.config.onError(error);

			if (poller.attempts >= poller.config.maxErrorAttempts) {
				this.stopPolling(jobId);
				updateTargets({ status: "error", error: error.message });
				return true;
			}
			return false;
		};

		const scheduleNext = () => {
			if (shouldStopPolling()) return;
			poller.timerId = window.setTimeout(tick, poller.config.interval);
		};

		const tick = async () => {
			if (shouldStopPolling()) return;

			if (poller.inFlight) {
				scheduleNext();
				return;
			}

			if (poller.attempts >= poller.config.maxAttempts) {
				handleTimeout();
				return;
			}

			poller.attempts++;
			poller.inFlight = true;
			poller.controller = new AbortController();

			try {
				const response = await fetch(`/api/jobs/${jobId}/status`, {
					signal: poller.controller.signal,
				});
				if (!response.ok) throw new Error(`HTTP ${response.status}`);
				const status = await response.json();
				if (handleSuccess(status)) return;
			} catch (error) {
				if (handleError(error)) return;
			} finally {
				poller.inFlight = false;
			}

			scheduleNext();
		};

		tick();

		return () => this.stopPollingForElement(jobId, element);
	}

	stopPollingForElement(jobId, element) {
		const poller = this.pollers.get(jobId);
		if (!poller) return;

		poller.targets.delete(element);
		if (poller.targets.size === 0) {
			this.stopPolling(jobId);
		}
	}

	stopPolling(jobId) {
		const poller = this.pollers.get(jobId);
		if (!poller) return;

		poller.stopped = true;
		if (poller.controller) {
			poller.controller.abort();
		}
		if (poller.timerId) {
			clearTimeout(poller.timerId);
		}
		this.pollers.delete(jobId);
	}

	stopAll() {
		for (const [jobId] of this.pollers) {
			this.stopPolling(jobId);
		}
	}

	hasActivePoller(jobId) {
		return this.pollers.has(jobId);
	}

	updateStatus(element, jobId, status, config) {
		const statusClass = this.getStatusClass(status.status);
		const statusText = this.getStatusText(status.status);
		const jobLink = `/jobs/${jobId}`;

		let html = `<a href="${jobLink}" class="job-status ${statusClass}">${statusText}</a>`;

		if (config.showProgress && (status.status === "pending" || status.status === "running")) {
			html += ' <span class="spinner">⟳</span>';
		}

		if (status.status === "error") {
			html = `<span class="job-status error">Error polling status</span>`;
		} else if (status.status === "timeout") {
			html = `<a href="${jobLink}" class="job-status timeout">Check status</a>`;
		}

		element.innerHTML = html;
	}

	getStatusClass(status) {
		switch (status) {
			case "completed":
				return "completed";
			case "failed":
				return "failed";
			case "running":
				return "running";
			case "pending":
				return "pending";
			case "timeout":
				return "timeout";
			case "error":
				return "error";
			default:
				return "unknown";
		}
	}

	getStatusText(status) {
		switch (status) {
			case "completed":
				return "Completed ✓";
			case "failed":
				return "Failed ✗";
			case "running":
				return "Running...";
			case "pending":
				return "Pending...";
			case "timeout":
				return "View Status";
			case "error":
				return "Error";
			default:
				return "Unknown";
		}
	}
}

const jobStatusPoller = new JobStatusPoller();

Lifecycle.register(
	"job-status-result",
	(element) => {
		const jobId = element.getAttribute("data-job-id");
		if (!jobId) return;
		element.__jobStatusCleanup = jobStatusPoller.startPolling(jobId, element, {
			showProgress: true,
			maxAttempts: 30,
		});
	},
	(element) => {
		if (element.__jobStatusCleanup) {
			element.__jobStatusCleanup();
		}
		element.__jobStatusCleanup = undefined;
	},
);

export default jobStatusPoller;
