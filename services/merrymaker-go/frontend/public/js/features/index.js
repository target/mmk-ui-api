/**
 * Central feature registry for the client shell.
 *
 * Each entry exposes metadata that allows the app bootstrapper to initialise
 * features in a predictable order while enabling selective boot in tests.
 */

import { initFormHelpers } from "./forms.js";
import { initFragmentHelpers } from "./htmx-fragments.js";
import { initHtmxHistory } from "./htmx-history.js";
import { initIcons } from "./icons.js";
import { initNavigation } from "./navigation.js";
import { initNetworkDetails } from "./network.js";
import { initRowNav } from "./row-nav.js";
import { initScreenshotFeature } from "./screenshot.js";
import { initSidebar } from "./sidebar.js";
import { initThemePreference } from "./theme.js";
import { initUserMenu } from "./user-menu.js";

const registry = [
	{ id: "theme", init: initThemePreference },
	{ id: "sidebar", init: initSidebar },
	{ id: "forms", init: initFormHelpers },
	{ id: "user-menu", init: initUserMenu },
	{ id: "network", init: initNetworkDetails },
	{ id: "screenshot", init: initScreenshotFeature },
	{ id: "row-nav", init: initRowNav },
	{ id: "navigation", init: initNavigation },
	{ id: "icons", init: initIcons },
	{ id: "htmx-history", init: initHtmxHistory },
	{ id: "htmx-fragments", init: initFragmentHelpers },
].map((feature) => Object.freeze(feature));

export const featureRegistry = Object.freeze(registry);

const knownIds = new Set();

for (const feature of featureRegistry) {
	const { id } = feature;
	if (knownIds.has(id)) {
		console.warn(
			`[features] Duplicate feature id "${id}" detected. Only the first will be booted.`,
		);
		continue;
	}
	knownIds.add(id);
}

/**
 * Convert iterable to Set when defined, otherwise return null.
 * Helps keep the bootFeatures branching low while avoiding accidental mutations.
 */
function toSetOrNull(iterable) {
	if (!iterable) return null;
	if (typeof iterable[Symbol.iterator] !== "function") return null;
	return new Set(iterable);
}

function warnForUnknownIds(ids, label) {
	if (!ids) return;

	const unknownIds = [];
	for (const id of ids) {
		if (!knownIds.has(id)) {
			unknownIds.push(id);
		}
	}

	if (unknownIds.length) {
		console.warn(`[features] Unknown feature id(s) in ${label}: ${unknownIds.join(", ")}`);
	}
}

function shouldBootFeature(id, includeSet, excludeSet, booted) {
	if (booted.has(id)) return false;
	if (includeSet && !includeSet.has(id)) return false;
	if (excludeSet?.has(id)) return false;
	return true;
}

/**
 * Boots the feature registry.
 *
 * @param {Object} options
 * @param {Iterable<string>} [options.include] - Boot only the given feature ids.
 * @param {Iterable<string>} [options.exclude] - Skip the given feature ids.
 * @returns {Set<string>} Set of booted feature ids.
 */
export function bootFeatures(options = {}) {
	const { include, exclude } = options;
	const includeSet = toSetOrNull(include);
	const excludeSet = toSetOrNull(exclude);
	const booted = new Set();

	warnForUnknownIds(includeSet, "include");
	warnForUnknownIds(excludeSet, "exclude");

	for (const feature of featureRegistry) {
		const { id, init } = feature;
		if (!shouldBootFeature(id, includeSet, excludeSet, booted)) continue;
		booted.add(id);

		if (typeof init !== "function") {
			console.warn(`[features] Feature "${id}" is missing an init() hook.`);
			continue;
		}

		try {
			init();
		} catch (error) {
			console.error(`[features] Failed to boot "${id}":`, error);
		}
	}

	return booted;
}
