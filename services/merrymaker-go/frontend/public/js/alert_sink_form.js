import { Lifecycle } from "./core/lifecycle.js";

function toPrettyJSON(value) {
	try {
		return JSON.stringify(value, null, 2);
	} catch (error) {
		console.error("alert-sink-form: failed to stringify preview result", error);
		return String(value);
	}
}

function debounce(fn, wait) {
	let timeoutId;
	const debounced = (...args) => {
		clearTimeout(timeoutId);
		timeoutId = setTimeout(() => {
			fn(...args);
		}, wait);
	};
	debounced.cancel = () => {
		clearTimeout(timeoutId);
		timeoutId = undefined;
	};
	return debounced;
}

function setBadge(element, text, status) {
	const badge = element.querySelector("#jmes-validity");
	if (!badge) return;

	badge.textContent = text;
	badge.classList.remove("is-ok", "is-error");
	if (status === "ok") badge.classList.add("is-ok");
	if (status === "error") badge.classList.add("is-error");
}

function updatePreview(element, exprEl, sampleEl, outEl) {
	if (!(exprEl && sampleEl && outEl)) {
		return;
	}

	const expr = exprEl.value || "@";
	const sampleTxt = sampleEl.value || "{}";

	let data;
	try {
		data = JSON.parse(sampleTxt);
	} catch (error) {
		setBadge(element, "Invalid JSON", "error");
		outEl.textContent = error?.message ?? "Invalid JSON";
		return;
	}

	if (!window.jmespath || typeof window.jmespath.search !== "function") {
		setBadge(element, "JMESPath not loaded", "error");
		outEl.textContent = "JMESPath library not available";
		return;
	}

	try {
		const result = window.jmespath.search(data, expr);
		setBadge(element, "Valid", "ok");
		outEl.textContent = toPrettyJSON(result);
	} catch (error) {
		setBadge(element, "Error", "error");
		outEl.textContent = error?.message ?? String(error);
	}
}

function initAlertSinkForm(element) {
	const exprEl = element.querySelector("#body");
	const sampleEl = element.querySelector("#sample_data");
	const outEl = element.querySelector("#jmes_result");

	if (!(exprEl && sampleEl && outEl)) {
		return null;
	}

	const debouncedPreview = debounce(() => updatePreview(element, exprEl, sampleEl, outEl), 250);
	exprEl.addEventListener("input", debouncedPreview);
	sampleEl.addEventListener("input", debouncedPreview);

	// Prime preview with current content
	updatePreview(element, exprEl, sampleEl, outEl);

	let repreviewTimer;
	let repreviewAttempts = 0;
	const maxRepreviewAttempts = 5;
	const scheduleRepreview = () => {
		if (window.jmespath?.search) {
			updatePreview(element, exprEl, sampleEl, outEl);
			return;
		}

		if (repreviewAttempts >= maxRepreviewAttempts) {
			return;
		}

		repreviewAttempts += 1;
		repreviewTimer = window.setTimeout(scheduleRepreview, 300);
	};

	scheduleRepreview();

	return {
		destroy() {
			exprEl.removeEventListener("input", debouncedPreview);
			sampleEl.removeEventListener("input", debouncedPreview);
			debouncedPreview.cancel?.();
			if (repreviewTimer) {
				clearTimeout(repreviewTimer);
				repreviewTimer = undefined;
			}
		},
	};
}

Lifecycle.register(
	"alert-sink-form",
	(element) => {
		element.__alertSinkForm = initAlertSinkForm(element);
	},
	(element) => {
		element.__alertSinkForm?.destroy?.();
		element.__alertSinkForm = undefined;
	},
);

export default null;
