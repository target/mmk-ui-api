/**
 * Form helper feature module
 *
 * Handles secrets toggles, replace checkbox interactions, and refresh config
 * controls used across forms.
 */
import { qs } from "../core/dom.js";
import { Events } from "../core/events.js";

let initialized = false;
const SECRET_TYPED_SHOW_LABEL = "Show Typed Value";
const SECRET_TYPED_HIDE_LABEL = "Hide Typed Value";
const SECRET_REVEAL_SHOW_LABEL = "Reveal Current Secret";
const SECRET_REVEAL_HIDE_LABEL = "Hide Revealed Secret";
const SECRET_REVEAL_DURATION_MS = 25_000;
const secretRevealTimers = new WeakMap();
const secretRevealControllers = new WeakMap();

function toggleTextarea(input, button) {
	const isHidden = input.classList.contains("secret-hidden");
	input.classList.toggle("secret-hidden", !isHidden);
	button.textContent = isHidden ? SECRET_TYPED_HIDE_LABEL : SECRET_TYPED_SHOW_LABEL;
	button.setAttribute("aria-pressed", isHidden ? "true" : "false");
}

function togglePasswordInput(input, button) {
	const isPwd = input.getAttribute("type") === "password";
	input.setAttribute("type", isPwd ? "text" : "password");
	button.setAttribute("aria-pressed", isPwd ? "true" : "false");
	button.textContent = isPwd ? SECRET_TYPED_HIDE_LABEL : SECRET_TYPED_SHOW_LABEL;
}

function handleToggleSecret(button) {
	const id = button.getAttribute("aria-controls");
	if (!id) return;

	const input = document.getElementById(id);
	if (!input) return;

	if (input.tagName.toLowerCase() === "textarea") {
		toggleTextarea(input, button);
	} else {
		togglePasswordInput(input, button);
	}
}

function handleToggleReplace(checkbox) {
	const form = checkbox.closest("form");
	const wrap = form ? qs("[data-secret-input]", form) : null;
	if (!wrap) return;

	const enable = checkbox.checked === true;
	wrap.hidden = !enable;
	const input = qs("[data-secret-input-target]", wrap);
	if (input) {
		input.disabled = !enable;
		if (!enable) {
			input.classList.add("secret-hidden");
		}
	}

	const toggleButton = qs('[data-action="toggle-secret"]', wrap);
	if (toggleButton) {
		toggleButton.textContent = SECRET_TYPED_SHOW_LABEL;
		toggleButton.setAttribute("aria-pressed", "false");
	}

	if (!enable) {
		resetSecretRevealState(wrap);
	}
}

function handleToggleRefreshConfig(checkbox) {
	const form = checkbox.closest("form");
	const wrap = form ? qs("[data-refresh-config]", form) : null;
	if (!wrap) return;

	const enable = checkbox.checked === true;
	wrap.hidden = !enable;
	wrap.setAttribute("aria-hidden", String(!enable));

	wrap.querySelectorAll("input, textarea").forEach((input) => {
		input.disabled = !enable;
		input.setAttribute("aria-disabled", String(!enable));
	});

	const valueInput = form ? qs("[data-value-required]", form) : null;
	const optionalHint = form ? qs("[data-optional-hint]", form) : null;
	const refreshHint = form ? qs("[data-refresh-hint]", form) : null;

	if (valueInput) {
		valueInput.required = !enable;
		if (optionalHint) optionalHint.hidden = !enable;
		if (refreshHint) refreshHint.hidden = !enable;
	}
}

function registerEventHandlers() {
	Events.on('[data-action="toggle-secret"]', "click", (_event, button) => {
		handleToggleSecret(button);
	});

	Events.on('[data-action="reveal-secret"]', "click", (_event, button) => {
		const revealUrl = button.dataset.secretRevealUrl;
		if (!revealUrl) return;
		handleRevealSecret(button, revealUrl);
	});

	Events.on('[data-action="toggle-replace"]', "change", (_event, checkbox) => {
		handleToggleReplace(checkbox);
	});

	Events.on('[data-action="toggle-refresh-config"]', "change", (_event, checkbox) => {
		handleToggleRefreshConfig(checkbox);
	});
}

export function initFormHelpers() {
	if (initialized) return;
	initialized = true;
	registerEventHandlers();
}

function getSecretElements(button) {
	const wrap = button.closest("[data-secret-input]");
	if (!wrap) {
		return {
			wrap: null,
			revealWrap: null,
			display: null,
			countdown: null,
			errorEl: null,
		};
	}

	const revealWrap = qs("[data-secret-reveal]", wrap);
	const display = revealWrap ? qs("[data-secret-display]", revealWrap) : null;
	const countdown = revealWrap ? qs("[data-secret-countdown]", revealWrap) : null;
	const errorEl = qs("[data-secret-error]", wrap);

	return { wrap, revealWrap, display, countdown, errorEl };
}

function clearSecretTimer(revealWrap) {
	const timerId = secretRevealTimers.get(revealWrap);
	if (timerId !== undefined) {
		window.clearTimeout(timerId);
		secretRevealTimers.delete(revealWrap);
	}
}

function clearSecretController(revealWrap) {
	if (!revealWrap) return;
	const controller = secretRevealControllers.get(revealWrap);
	if (controller) {
		controller.abort();
	}
	secretRevealControllers.delete(revealWrap);
}

function resetSecretRevealState(wrap) {
	if (!wrap) return;
	const revealWrap = qs("[data-secret-reveal]", wrap);
	if (revealWrap) {
		clearSecretTimer(revealWrap);
		clearSecretController(revealWrap);

		const display = qs("[data-secret-display]", revealWrap);
		if (display) {
			display.value = "";
			display.blur();
		}

		revealWrap.hidden = true;

		const countdown = qs("[data-secret-countdown]", revealWrap);
		if (countdown) {
			countdown.hidden = true;
			countdown.textContent = "";
		}
	}

	const errorEl = qs("[data-secret-error]", wrap);
	if (errorEl) {
		errorEl.hidden = true;
		errorEl.textContent = "";
	}

	const revealButton = qs('[data-action="reveal-secret"]', wrap);
	if (revealButton) {
		revealButton.textContent = SECRET_REVEAL_SHOW_LABEL;
		revealButton.setAttribute("aria-pressed", "false");
		revealButton.disabled = false;
		delete revealButton.dataset.secretLoading;
		revealButton.removeAttribute("aria-busy");
	}
}

function hideSecretReveal(button, elements) {
	const { revealWrap, display, countdown } = elements;
	if (!revealWrap) return;

	clearSecretTimer(revealWrap);
	clearSecretController(revealWrap);

	if (display) {
		display.value = "";
		display.blur();
	}

	revealWrap.hidden = true;
	button.textContent = SECRET_REVEAL_SHOW_LABEL;
	button.setAttribute("aria-pressed", "false");

	if (countdown) {
		countdown.hidden = true;
		countdown.textContent = "";
	}
}

async function fetchSecretValue(url, signal) {
	const response = await fetch(url, {
		credentials: "same-origin",
		headers: { Accept: "application/json" },
		signal,
	});

	if (!response.ok) {
		throw new Error(`Secret fetch failed with status ${response.status}`);
	}

	const data = await response.json();
	const value = data?.value;

	if (typeof value !== "string") {
		throw new Error("Secret response missing value");
	}

	return value;
}

function showSecretError(errorEl, message) {
	if (!errorEl) return;
	errorEl.textContent = message;
	errorEl.hidden = false;
}

function clearSecretError(errorEl) {
	if (!errorEl) return;
	errorEl.textContent = "";
	errorEl.hidden = true;
}

function scheduleAutoHide(button, elements) {
	const { revealWrap, countdown } = elements;
	if (!revealWrap) return;

	clearSecretTimer(revealWrap);

	if (countdown) {
		const seconds = Math.round(SECRET_REVEAL_DURATION_MS / 1000);
		countdown.hidden = false;
		countdown.textContent = `Secret hides automatically in ${seconds} second${seconds === 1 ? "" : "s"}.`;
	}

	const timerId = window.setTimeout(() => {
		hideSecretReveal(button, elements);
	}, SECRET_REVEAL_DURATION_MS);

	secretRevealTimers.set(revealWrap, timerId);
}

async function handleRevealSecret(button, url) {
	if (button.dataset.secretLoading === "true") return;
	const elements = getSecretElements(button);
	const { revealWrap, display, errorEl, wrap } = elements;

	if (!wrap) {
		return;
	}
	if (!(revealWrap && display)) {
		return;
	}

	const isActive = button.getAttribute("aria-pressed") === "true";
	if (isActive) {
		hideSecretReveal(button, elements);
		return;
	}

	clearSecretController(revealWrap);
	clearSecretError(errorEl);
	button.disabled = true;
	button.dataset.secretLoading = "true";
	button.setAttribute("aria-busy", "true");

	const controller = new AbortController();
	secretRevealControllers.set(revealWrap, controller);
	try {
		const secret = await fetchSecretValue(url, controller.signal);

		// Wrap may have been hidden or removed while awaiting response
		if (!wrap.isConnected || wrap.hidden) {
			resetSecretRevealState(wrap);
			return;
		}

		display.value = secret;
		revealWrap.hidden = false;
		display.focus({ preventScroll: true });

		button.textContent = SECRET_REVEAL_HIDE_LABEL;
		button.setAttribute("aria-pressed", "true");

		scheduleAutoHide(button, elements);
	} catch (error) {
		if (error?.name !== "AbortError") {
			console.error("Failed to reveal secret", error);
			hideSecretReveal(button, elements);
			showSecretError(errorEl, "Unable to load secret. Try again.");
		}
	} finally {
		secretRevealControllers.delete(revealWrap);
		button.disabled = false;
		delete button.dataset.secretLoading;
		button.removeAttribute("aria-busy");
	}
}
