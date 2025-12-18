import { Lifecycle } from "./core/lifecycle.js";

const MAX_DOM_EVENTS = 300;
const POLL_INTERVALS = {
	fast: 2000,
	medium: 5000,
	slow: 10000,
};

function genToken() {
	if (window.crypto && typeof window.crypto.randomUUID === "function") {
		return window.crypto.randomUUID();
	}
	return Math.random().toString(36).slice(2) + Date.now().toString(36);
}

function disableForm(form, disabled) {
	if (!form) return;
	Array.from(form.elements).forEach((element) => {
		element.disabled = !!disabled;
	});
}

function scheduleFrame(callback, registry) {
	const raf =
		typeof window.requestAnimationFrame === "function"
			? window.requestAnimationFrame.bind(window)
			: null;
	if (raf) {
		let active = true;
		let id;
		const cancel = () => {
			if (!active) return;
			active = false;
			if (typeof window.cancelAnimationFrame === "function") {
				window.cancelAnimationFrame(id);
			}
			registry?.delete(cancel);
		};
		id = raf(() => {
			if (!active) return;
			active = false;
			registry?.delete(cancel);
			callback();
		});
		registry?.add(cancel);
		return cancel;
	}

	let active = true;
	const timeoutId = window.setTimeout(() => {
		if (!active) return;
		active = false;
		registry?.delete(cancel);
		callback();
	}, 0);
	const cancel = () => {
		if (!active) return;
		active = false;
		clearTimeout(timeoutId);
		registry?.delete(cancel);
	};
	registry?.add(cancel);
	return cancel;
}

function executeScripts(scripts) {
	scripts.forEach((script) => {
		const newScript = document.createElement("script");
		if (script.type) {
			newScript.type = script.type;
		}
		if (script.src) {
			newScript.src = script.src;
		} else {
			newScript.textContent = script.text ?? "";
		}
		document.body.appendChild(newScript);
		newScript.remove();
	});
}

class SourceFormComponent {
	constructor(root) {
		this.root = root;
		this.form = root.querySelector("#source-form");
		this.tokenInput = root.querySelector("#client_token");
		this.testButton = root.querySelector("#btn-test");
		this.testPanel = root.querySelector("#source-test-panel");
		this.testEventsContainer = root.querySelector("#test-events");
		this.progressEl = root.querySelector("#test-progress");
		this.progressText = root.querySelector("#test-progress-text");
		this.filterControls = root.querySelector('[data-role="event-filters"]');
		this.allFilterButton = this.filterControls?.querySelector('[data-filter="all"]') || null;
		this.categoryButtons = this.filterControls
			? Array.from(this.filterControls.querySelectorAll("[data-category]"))
			: [];
		this.filterSummaryText =
			this.filterControls?.querySelector('[data-role="event-filter-summary"]') || null;
		this.activeCategories = new Set();
		this.displayedEventCount = 0;
		this.filterEmptyNotice = null;
		this.pendingFrameCancels = new Set();

		this.jobId = this.testEventsContainer?.getAttribute("data-job-id") || null;
		this.since =
			Number.parseInt(this.testEventsContainer?.getAttribute("data-since") || "0", 10) || 0;
		this.totalEventCount = 0;
		this.isTerminal = this.testEventsContainer?.getAttribute("data-terminal") === "true";
		this.currentPollSpeed = "fast";
		this.pollIntervalId = null;
		this.isFetching = false;
		this.abortController = null;
		this.pruneScheduled = false;
		this.eventCardCache = null;

		this.boundHandlers = [];

		this.handleTokenClick = this.handleTokenClick.bind(this);
		this.handleFilterControlsClick = this.handleFilterControlsClick.bind(this);
		this.handleCategoryToggle = this.handleCategoryToggle.bind(this);
		this.handleAllToggle = this.handleAllToggle.bind(this);
		this.pollEvents = this.pollEvents.bind(this);

		this.initialize();
	}

	initialize() {
		this.setupTokenButton();
		this.setupFilterControls();
		this.setupEventDetailsLoader();
		this.prepareEventDetailPanels([this.testEventsContainer]);
		this.syncFormDisabledState();
		this.startPollingIfNeeded();
	}

	setupTokenButton() {
		if (!(this.testButton && this.tokenInput)) {
			return;
		}

		this.testButton.addEventListener("click", this.handleTokenClick);
		this.boundHandlers.push({
			target: this.testButton,
			type: "click",
			listener: this.handleTokenClick,
		});
	}

	setupFilterControls() {
		if (!this.filterControls || this.categoryButtons.length === 0) {
			return;
		}

		const availableCategories = this.categoryButtons
			.map((button) => button.dataset.category || "")
			.filter((value) => value.length > 0);

		let nextActive;
		if (this.activeCategories.size > 0) {
			nextActive = new Set();
			availableCategories.forEach((value) => {
				if (this.activeCategories.has(value)) {
					nextActive.add(value);
				}
			});
		} else {
			nextActive = new Set(availableCategories);
		}

		if (nextActive.size === 0 && availableCategories.length > 0) {
			nextActive = new Set(availableCategories);
		}

		this.activeCategories = nextActive;

		this.filterControls.addEventListener("click", this.handleFilterControlsClick);
		this.boundHandlers.push({
			target: this.filterControls,
			type: "click",
			listener: this.handleFilterControlsClick,
		});

		this.syncFilterButtons();
		this.applyCategoryFilters();
	}

	setupEventDetailsLoader() {
		if (!this.testEventsContainer) return;

		const loadDetails = async (panel) => {
			if (!panel) {
				return;
			}

			this.ensurePanelUrl(panel);

			if (
				panel.dataset.detailsLoaded === "true" ||
				panel.dataset.detailsLoading === "true" ||
				!panel.dataset.detailsUrl
			) {
				return;
			}

			const url = panel.dataset.detailsUrl;
			panel.dataset.detailsLoading = "true";
			const placeholder = panel.innerHTML.trim();

			try {
				const resp = await fetch(url, { headers: { "HX-Request": "true" } });
				if (!resp.ok) {
					throw new Error(`HTTP ${resp.status}`);
				}
				const html = await resp.text();
				if (!html.trim()) {
					panel.innerHTML = `<div class="text-muted small">No details available.</div>`;
					panel.dataset.detailsLoaded = "empty";
					panel.dataset.detailsLoading = "false";
					return;
				}
				panel.innerHTML = html;
				panel.dataset.detailsLoaded = "true";
				panel.dataset.detailsLoading = "false";
			} catch (error) {
				panel.dataset.detailsLoading = "false";
				panel.dataset.detailsLoaded = "error";
				panel.innerHTML = `<div class="text-danger small">Unable to load event details (${error?.message || error}).</div>`;
				// Restore placeholder so user can retry by toggling.
				if (placeholder) {
					panel.insertAdjacentHTML(
						"beforeend",
						`<div class="text-muted small mt-1">${placeholder}</div>`,
					);
				}
			}
		};

		const handleToggle = (event) => {
			const node = event.target;
			if (!(node instanceof HTMLDetailsElement && node.open)) {
				return;
			}
			const panel = node.querySelector(".event__details");
			loadDetails(panel);
		};

		const handleSummaryClick = (event) => {
			const summary = event.target instanceof Element ? event.target.closest("summary") : null;
			if (!summary) return;
			const details = summary.closest("details");
			if (!details) return;
			const panel = details.querySelector(".event__details");
			loadDetails(panel);
		};

		this.testEventsContainer.addEventListener("toggle", handleToggle);
		this.testEventsContainer.addEventListener("click", handleSummaryClick);
		this.boundHandlers.push({
			target: this.testEventsContainer,
			type: "toggle",
			listener: handleToggle,
		});
		this.boundHandlers.push({
			target: this.testEventsContainer,
			type: "click",
			listener: handleSummaryClick,
		});
	}

	ensurePanelUrl(panel) {
		if (!(panel instanceof Element)) {
			return null;
		}

		if (panel.dataset.detailsUrl) {
			return panel.dataset.detailsUrl;
		}

		const hxUrl = panel.getAttribute("hx-get");
		if (!hxUrl) {
			return null;
		}

		panel.dataset.detailsUrl = hxUrl;
		panel.removeAttribute("hx-get");
		panel.removeAttribute("hx-trigger");
		return hxUrl;
	}

	prepareEventDetailPanels(nodes) {
		const panels = [];
		nodes.forEach((node) => {
			if (!(node instanceof Element)) {
				return;
			}
			if (node.matches(".event__details")) {
				panels.push(node);
			}
			if (typeof node.querySelectorAll === "function") {
				panels.push(...node.querySelectorAll(".event__details"));
			}
		});

		panels.forEach((panel) => {
			this.ensurePanelUrl(panel);
		});
	}

	handleTokenClick() {
		if (this.tokenInput) {
			this.tokenInput.value = genToken();
		}
	}

	handleFilterControlsClick(event) {
		if (!this.filterControls) {
			return;
		}

		const target =
			event.target instanceof Element
				? event.target.closest('[data-filter="all"], [data-category]')
				: null;

		if (!(target && this.filterControls.contains(target))) {
			return;
		}

		if (target.matches('[data-filter="all"]')) {
			this.handleAllToggle(event, target);
			return;
		}

		if (target.matches("[data-category]")) {
			this.handleCategoryToggle(event, target);
		}
	}

	handleCategoryToggle(event, explicitButton = null) {
		if (event) {
			event.preventDefault();
		}

		const button =
			explicitButton ||
			(event?.target instanceof Element ? event.target.closest("[data-category]") : null);

		if (!(button instanceof HTMLElement && this.filterControls?.contains(button))) {
			return;
		}

		const value = button.dataset.category;
		if (!value) {
			return;
		}

		if (this.activeCategories.has(value)) {
			this.activeCategories.delete(value);
		} else {
			this.activeCategories.add(value);
		}

		this.syncFilterButtons();
		this.applyCategoryFilters();
		this.updateProgress();
	}

	handleAllToggle(event, explicitButton = null) {
		if (event) {
			event.preventDefault();
		}

		const button =
			explicitButton ||
			(event?.target instanceof Element ? event.target.closest('[data-filter="all"]') : null) ||
			this.allFilterButton;

		if (!(button instanceof HTMLElement && this.filterControls?.contains(button))) {
			return;
		}

		if (this.isAllSelected()) {
			this.activeCategories.clear();
		} else {
			this.activeCategories.clear();
			this.categoryButtons.forEach((button) => {
				const value = button.getAttribute("data-category");
				if (value) {
					this.activeCategories.add(value);
				}
			});
		}

		this.syncFilterButtons();
		this.applyCategoryFilters();
		this.updateProgress();
	}

	isAllSelected() {
		return (
			this.categoryButtons.length > 0 && this.activeCategories.size === this.categoryButtons.length
		);
	}

	getAllButtonState() {
		if (!this.allFilterButton) {
			return null;
		}

		if (this.isAllSelected()) {
			return "all";
		}

		if (this.activeCategories.size === 0) {
			return "cleared";
		}

		return "partial";
	}

	syncFilterButtons() {
		if (this.categoryButtons.length === 0) {
			return;
		}

		this.categoryButtons.forEach((button) => {
			const value = button.getAttribute("data-category");
			if (!value) {
				return;
			}
			const isActive = this.activeCategories.has(value);
			button.classList.toggle("active", isActive);
			button.setAttribute("aria-pressed", isActive ? "true" : "false");
		});

		if (this.allFilterButton) {
			const state = this.getAllButtonState();
			const shouldBeActive = state === "all";
			this.allFilterButton.classList.toggle("active", shouldBeActive);
			this.allFilterButton.setAttribute("aria-pressed", shouldBeActive ? "true" : "false");

			if (state) {
				this.allFilterButton.setAttribute("data-state", state);
			} else {
				this.allFilterButton.removeAttribute("data-state");
			}
		}
	}

	syncFormDisabledState() {
		if (!this.form) return;

		if (this.testPanel && !this.testPanel.hasAttribute("hidden")) {
			disableForm(this.form, true);
		} else {
			disableForm(this.form, false);
		}
	}

	invalidateEventCardCache() {
		this.eventCardCache = null;
	}

	getEventCards() {
		if (!this.testEventsContainer) {
			return [];
		}

		if (!this.eventCardCache) {
			this.eventCardCache = Array.from(this.testEventsContainer.querySelectorAll(".event-card"));
		}

		return this.eventCardCache;
	}

	applyCategoryFilters() {
		if (!this.testEventsContainer) {
			return;
		}

		const eventCards = this.getEventCards();
		const treatAsAllSelected = this.categoryButtons.length === 0 || this.isAllSelected();
		const knownCategories = new Set(
			this.categoryButtons
				.map((button) => button.dataset.category || "")
				.filter((value) => value.length > 0),
		);

		let visibleCount = 0;
		eventCards.forEach((card) => {
			const rawCategory = card.getAttribute("data-event-type") || "other";
			const isKnownCategory = knownCategories.has(rawCategory);
			const category = isKnownCategory ? rawCategory : "other";
			const shouldShow = treatAsAllSelected || this.activeCategories.has(category);
			if (shouldShow) {
				card.classList.remove("event-card--filtered-out");
				visibleCount += 1;
			} else {
				card.classList.add("event-card--filtered-out");
			}
		});

		this.displayedEventCount = visibleCount;
		this.updateFilterEmptyState(eventCards.length, visibleCount);
		this.updateFilterSummary(eventCards.length, visibleCount);
	}

	updateFilterEmptyState(totalCount, visibleCount) {
		if (!this.testEventsContainer) {
			return;
		}

		const shouldShowNotice = totalCount > 0 && visibleCount === 0;

		if (shouldShowNotice) {
			if (!this.filterEmptyNotice) {
				this.filterEmptyNotice = document.createElement("div");
				this.filterEmptyNotice.className = "filter-empty-notice text-muted text-center py-3";
				this.filterEmptyNotice.textContent = "No events match the selected filters.";
			}

			if (!this.filterEmptyNotice.isConnected) {
				this.testEventsContainer.insertAdjacentElement("afterbegin", this.filterEmptyNotice);
			}
		} else if (this.filterEmptyNotice?.isConnected) {
			this.filterEmptyNotice.remove();
		}
	}

	updateFilterSummary(totalCount, visibleCount) {
		if (!this.filterSummaryText) {
			return;
		}

		const totalTypes = this.categoryButtons.length;
		const activeTypes = this.activeCategories.size;

		if (totalTypes === 0) {
			this.filterSummaryText.textContent = "No event types";
			return;
		}

		const filterActive = activeTypes !== totalTypes;
		let summaryText;

		if (activeTypes === totalTypes) {
			summaryText = "All event types";
		} else if (activeTypes === 0) {
			summaryText = `0/${totalTypes} types`;
		} else {
			summaryText = `${activeTypes}/${totalTypes} types`;
		}

		if (filterActive) {
			if (visibleCount === 0 && totalCount > 0) {
				summaryText += " • no matches";
			} else if (visibleCount > 0) {
				const noun = visibleCount === 1 ? "event" : "events";
				summaryText += ` • ${visibleCount} ${noun}`;
			}
		}

		this.filterSummaryText.textContent = summaryText;
	}

	getDomEventCount() {
		if (!this.testEventsContainer) {
			return 0;
		}
		return this.getEventCards().length;
	}

	startPollingIfNeeded() {
		if (!this.testEventsContainer) return;
		if (!this.jobId) return;

		// Initial poll for existing content
		this.pollEvents();

		this.pollIntervalId = window.setInterval(this.pollEvents, POLL_INTERVALS.fast);
	}

	pruneOldEvents() {
		if (this.pruneScheduled) return 0;
		if (!this.testEventsContainer) return 0;

		const eventCards = this.getEventCards();
		const excessCount = eventCards.length - MAX_DOM_EVENTS;
		const hasNotice = this.testEventsContainer.querySelector(".pruning-notice");

		if (excessCount <= 0) {
			return 0;
		}

		this.pruneScheduled = true;
		scheduleFrame(() => {
			for (let i = 0; i < excessCount; i++) {
				eventCards[i]?.remove();
			}

			if (!hasNotice) {
				const notice = document.createElement("div");
				notice.className = "pruning-notice text-muted text-sm mb-2 p-2";
				notice.style.cssText =
					"background: #f8f9fa; border-left: 3px solid #6c757d; border-radius: 4px;";
				notice.innerHTML =
					"<strong>Note:</strong> Showing most recent " +
					MAX_DOM_EVENTS +
					" events. Older events were pruned for performance.";
				this.testEventsContainer.insertBefore(notice, this.testEventsContainer.firstChild);
			}

			if (this.testEventsContainer?.isConnected) {
				this.invalidateEventCardCache();
				this.applyCategoryFilters();
				this.updateProgress();
			}

			this.pruneScheduled = false;
		}, this.pendingFrameCancels);

		return excessCount;
	}

	updateProgress() {
		if (!(this.progressEl && this.progressText)) {
			return;
		}

		const renderedCount = this.getDomEventCount();
		const showing = renderedCount < this.totalEventCount ? ` (showing last ${renderedCount})` : "";
		const filterActive =
			this.categoryButtons.length > 0 && this.activeCategories.size !== this.categoryButtons.length;

		let filterMessage = "";
		if (filterActive) {
			const typeSummary = `${this.activeCategories.size}/${this.categoryButtons.length} types`;
			if (this.displayedEventCount === 0) {
				filterMessage = `; ${typeSummary} • no events match current filters`;
			} else {
				const noun = this.displayedEventCount === 1 ? "event" : "events";
				filterMessage = `; ${typeSummary} • ${this.displayedEventCount} ${noun} match current filters`;
			}
		}

		scheduleFrame(() => {
			this.progressEl.style.display = "block";
			if (this.isTerminal) {
				this.progressText.innerHTML = `<strong>[SUCCESS]</strong> Test complete - ${this.totalEventCount} events captured${filterMessage}`;
			} else {
				this.progressText.innerHTML = `<strong>[INFO]</strong> Test running... ${this.totalEventCount} events captured${showing}${filterMessage}`;
			}
		}, this.pendingFrameCancels);
	}

	getAdaptivePollInterval() {
		if (this.totalEventCount >= MAX_DOM_EVENTS) return POLL_INTERVALS.slow;
		if (this.totalEventCount >= 200) return POLL_INTERVALS.slow;
		if (this.totalEventCount >= 100) return POLL_INTERVALS.medium;
		return POLL_INTERVALS.fast;
	}

	showViewFullResultsButton() {
		if (!this.testEventsContainer) return;
		if (this.testEventsContainer.querySelector(".view-full-results-btn")) return;

		const btnContainer = document.createElement("div");
		btnContainer.className = "view-full-results-btn text-center mt-3 mb-3";
		btnContainer.innerHTML =
			'<a href="/jobs/' +
			this.jobId +
			'" class="btn btn-primary">' +
			'<i data-lucide="external-link" class="w-4 h-4"></i> View Full Results' +
			"</a>";
		this.testEventsContainer.appendChild(btnContainer);

		if (window.lucide && typeof window.lucide.createIcons === "function") {
			try {
				if (window.lucide.icons) {
					window.lucide.createIcons({ icons: window.lucide.icons });
				} else {
					window.lucide.createIcons();
				}
			} catch (error) {
				console.warn("Failed to initialize lucide icons:", error);
			}
		}
	}

	updatePollingSpeed() {
		const newInterval = this.getAdaptivePollInterval();
		const newSpeed =
			newInterval === POLL_INTERVALS.fast
				? "fast"
				: newInterval === POLL_INTERVALS.medium
					? "medium"
					: "slow";

		if (newSpeed === this.currentPollSpeed) {
			return;
		}

		this.currentPollSpeed = newSpeed;
		if (this.pollIntervalId) {
			clearInterval(this.pollIntervalId);
		}
		this.pollIntervalId = window.setInterval(this.pollEvents, newInterval);

		if (this.totalEventCount >= MAX_DOM_EVENTS) {
			this.showViewFullResultsButton();
		}
	}

	processPollingResponse(html) {
		if (!this.testEventsContainer) {
			return;
		}

		const tempDiv = document.createElement("div");
		tempDiv.innerHTML = html;

		const stateUpdate = tempDiv.querySelector(".polling-state-update");
		let nextSince = this.since;
		let newIsTerminal = false;

		if (stateUpdate) {
			nextSince = Number.parseInt(stateUpdate.getAttribute("data-next-since") || "0", 10);
			newIsTerminal = stateUpdate.getAttribute("data-terminal") === "true";
			stateUpdate.remove();
		}

		const scriptDefinitions = Array.from(tempDiv.querySelectorAll("script")).map((script) => {
			const definition = {
				type: script.getAttribute("type") || undefined,
				src: script.getAttribute("src") || undefined,
				text: script.textContent || "",
			};
			script.remove();
			return definition;
		});

		const newEventCards = tempDiv.querySelectorAll(".event-card");
		const newEventCount = newEventCards.length;

		const insertedNodes = [];
		const hasMarkup = tempDiv.innerHTML.trim().length > 0;
		if (hasMarkup) {
			const fragment = document.createDocumentFragment();
			while (tempDiv.firstChild) {
				const node = tempDiv.firstChild;
				insertedNodes.push(node);
				fragment.appendChild(node);
			}
			this.testEventsContainer.appendChild(fragment);
			this.invalidateEventCardCache();
		}

		if (window.htmx && typeof window.htmx.process === "function" && insertedNodes.length > 0) {
			insertedNodes.forEach((node) => {
				if (node.nodeType === Node.ELEMENT_NODE) {
					window.htmx.process(node);
				}
			});
		}

		if (insertedNodes.length > 0) {
			this.prepareEventDetailPanels(insertedNodes);
		}

		if (scriptDefinitions.length > 0) {
			executeScripts(scriptDefinitions);
		}

		if (!Number.isNaN(nextSince) && nextSince >= this.since) {
			this.since = nextSince;
		}

		this.totalEventCount += newEventCount;

		if (this.getDomEventCount() > MAX_DOM_EVENTS) {
			this.pruneOldEvents();
		}

		this.testEventsContainer.setAttribute("data-since", String(this.since));
		this.testEventsContainer.setAttribute("data-terminal", newIsTerminal ? "true" : "false");

		this.isTerminal = newIsTerminal;

		this.applyCategoryFilters();
		this.updateProgress();
		this.updatePollingSpeed();

		if (this.isTerminal) {
			if (!this.testEventsContainer.querySelector(".job-completed-msg")) {
				const completionMsg = document.createElement("div");
				completionMsg.className = "text-muted text-center mt-2 job-completed-msg";
				completionMsg.textContent = "Job completed";
				this.testEventsContainer.appendChild(completionMsg);
			}
			this.showViewFullResultsButton();
			this.cleanupPolling();
		}
	}

	pollEvents() {
		if (!this.testEventsContainer) return;
		if (!this.testEventsContainer.isConnected) {
			this.cleanupPolling();
			return;
		}

		if (this.testEventsContainer.getAttribute("data-terminal") === "true") {
			this.cleanupPolling();
			return;
		}

		if (this.isFetching) {
			return;
		}

		this.isFetching = true;
		this.abortController = new AbortController();
		const url = `/sources/test/${this.jobId}/events?since=${this.since}`;

		fetch(url, { signal: this.abortController.signal })
			.then((response) => {
				if (!response.ok) {
					throw new Error(`HTTP ${response.status} for ${url}`);
				}
				return response.text();
			})
			.then((html) => {
				this.processPollingResponse(html);
			})
			.catch((error) => {
				if (error.name === "AbortError") {
					return;
				}
				console.error("Polling error:", error);
				if (String(error.message).includes("HTTP 404")) {
					this.cleanupPolling();
				}
			})
			.finally(() => {
				this.isFetching = false;
			});
	}

	cleanupPolling() {
		if (this.pollIntervalId) {
			clearInterval(this.pollIntervalId);
			this.pollIntervalId = null;
		}

		if (this.abortController) {
			this.abortController.abort();
			this.abortController = null;
		}

		this.isFetching = false;

		if (this.form) {
			disableForm(this.form, false);
		}
	}

	destroy() {
		this.boundHandlers.forEach(({ target, type, listener }) => {
			target.removeEventListener(type, listener);
		});
		this.boundHandlers = [];
		this.cleanupPolling();
		this.pendingFrameCancels.forEach((cancel) => {
			try {
				cancel();
			} catch (_) {
				/* noop */
			}
		});
		this.pendingFrameCancels.clear();
		if (this.filterEmptyNotice?.isConnected) {
			this.filterEmptyNotice.remove();
		}
		this.filterEmptyNotice = null;
		this.pruneScheduled = false;
		this.invalidateEventCardCache();
	}
}

Lifecycle.register(
	"source-form",
	(element) => {
		element.__sourceForm = new SourceFormComponent(element);
	},
	(element) => {
		element.__sourceForm?.destroy();
		element.__sourceForm = undefined;
	},
);

export default null;
