/**
 * Screenshot modal feature module
 *
 * Wires up thumbnail triggers and exposes the modal helper globally for areas
 * that still access it via `window.showScreenshotModal`.
 */

import { showScreenshotModal } from "../components/screenshot-modal.js";
import { Events } from "../core/events.js";

let initialized = false;
let observer;

function revealScreenshotFallback(img) {
	if (!(img instanceof HTMLImageElement)) return;
	if (!img.matches("img[data-onerror-fallback]")) return;
	img.hidden = true;
	const fallback = img.nextElementSibling;
	if (fallback instanceof HTMLElement) {
		fallback.hidden = false;
	}
}

function handleExistingFailures(root = document) {
	const images = root.querySelectorAll("img[data-onerror-fallback]");
	for (const img of images) {
		if (!(img instanceof HTMLImageElement)) continue;
		if (img.complete && img.naturalWidth === 0) {
			revealScreenshotFallback(img);
		}
	}
}

function observeNewImages() {
	if (observer || !("MutationObserver" in window) || !document.body) return;
	observer = new MutationObserver((mutations) => {
		for (const mutation of mutations) {
			for (const node of mutation.addedNodes) {
				if (!(node instanceof HTMLElement)) continue;
				if (node.matches?.("img[data-onerror-fallback]")) {
					if (node.complete && node.naturalWidth === 0) {
						revealScreenshotFallback(node);
					}
				}
				handleExistingFailures(node);
			}
		}
	});
	observer.observe(document.body, { childList: true, subtree: true });
}

function registerEventHandlers() {
	window.showScreenshotModal = showScreenshotModal;

	document.addEventListener(
		"error",
		(event) => {
			const target = event.target;
			revealScreenshotFallback(target);
		},
		true,
	);

	if (document.readyState === "loading") {
		document.addEventListener(
			"DOMContentLoaded",
			() => {
				handleExistingFailures();
				observeNewImages();
			},
			{ once: true },
		);
	} else {
		handleExistingFailures();
		observeNewImages();
	}

	Events.on(".screenshot-thumbnail", "click", (_event, img) => {
		const src = img.dataset.src || img.src;
		const caption = img.dataset.caption || "";
		showScreenshotModal(src, caption);
	});
}

export function initScreenshotFeature() {
	if (initialized) return;
	initialized = true;
	registerEventHandlers();
}
