import { Events } from "../core/events.js";
import { on } from "../core/htmx-bridge.js";

const STORAGE_KEY = "merrymaker:theme";
const ThemeMode = Object.freeze({
	LIGHT: "light",
	DARK: "dark",
	SYSTEM: "system",
});
const VALID_THEME_VALUES = new Set(Object.values(ThemeMode));

let prefersDarkQuery = null;
let currentMode = ThemeMode.SYSTEM;
let initialized = false;

function readStoredPreference() {
	if (typeof window === "undefined") return null;

	try {
		const stored = window.localStorage.getItem(STORAGE_KEY);
		if (VALID_THEME_VALUES.has(stored)) {
			return stored;
		}
	} catch {
		// Ignore storage access issues (private mode, disabled storage, etc.)
	}

	return null;
}

function storePreference(mode) {
	if (typeof window === "undefined") return;

	try {
		if (mode === ThemeMode.SYSTEM) {
			window.localStorage.removeItem(STORAGE_KEY);
		} else {
			window.localStorage.setItem(STORAGE_KEY, mode);
		}
	} catch {
		// Ignore persistence failures
	}
}

function resolveEffectiveMode(mode) {
	if (mode === ThemeMode.SYSTEM) {
		return prefersDarkQuery?.matches ? ThemeMode.DARK : ThemeMode.LIGHT;
	}

	return mode;
}

function syncSelects(mode) {
	if (typeof document === "undefined") return;

	const targetValue = VALID_THEME_VALUES.has(mode) ? mode : ThemeMode.SYSTEM;

	document.querySelectorAll("[data-theme-select]").forEach((element) => {
		if (!(element instanceof HTMLSelectElement)) return;

		if (element.value !== targetValue) {
			element.value = targetValue;
		}
	});
}

function applyTheme(mode) {
	if (typeof document === "undefined") return;

	const root = document.documentElement;
	const attributeValue =
		mode === ThemeMode.DARK || mode === ThemeMode.LIGHT ? mode : ThemeMode.SYSTEM;

	root.setAttribute("data-theme", attributeValue);

	const resolvedMode = resolveEffectiveMode(mode);
	root.style.colorScheme = resolvedMode === ThemeMode.DARK ? "dark" : "light";

	currentMode = mode;
	syncSelects(mode);
}

function setTheme(mode) {
	if (!VALID_THEME_VALUES.has(mode)) {
		syncSelects(currentMode);
		return;
	}

	if (mode === currentMode) {
		syncSelects(currentMode);
		return;
	}

	storePreference(mode);
	applyTheme(mode);
}

function registerMediaQueryListener() {
	if (!prefersDarkQuery) return;

	const handleChange = () => {
		if (currentMode === ThemeMode.SYSTEM) {
			applyTheme(ThemeMode.SYSTEM);
		}
	};

	if (typeof prefersDarkQuery.addEventListener === "function") {
		prefersDarkQuery.addEventListener("change", handleChange);
	} else if (typeof prefersDarkQuery.addListener === "function") {
		prefersDarkQuery.addListener(handleChange);
	}
}

export function initThemePreference() {
	if (initialized || typeof document === "undefined") return;
	initialized = true;

	if (typeof window !== "undefined" && typeof window.matchMedia === "function") {
		prefersDarkQuery = window.matchMedia("(prefers-color-scheme: dark)");
	}

	const storedPreference = readStoredPreference();
	applyTheme(storedPreference ?? ThemeMode.SYSTEM);

	registerMediaQueryListener();

	Events.on("[data-theme-select]", "change", (_event, target) => {
		if (!(target instanceof HTMLSelectElement)) return;
		setTheme(target.value);
	});

	on("htmx:afterSwap", () => {
		syncSelects(currentMode);
	});
}
