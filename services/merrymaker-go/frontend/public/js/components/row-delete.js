/**
 * Row Delete Component
 *
 * Applies row removal animations and cleanup logic to delete triggers marked
 * with data-component="row-delete" and data-row-delete.
 */

import { Lifecycle } from "../core/lifecycle.js";

function findRow(element) {
	try {
		return element?.closest?.("tr") ?? null;
	} catch (error) {
		console.warn("row-delete: failed to find parent row", error);
		return null;
	}
}

function setRowRemoving(element, enabled) {
	const row = findRow(element);
	if (!row) return;
	row.classList.toggle("row-removing", enabled);
}

Lifecycle.register(
	"row-delete",
	(element) => {
		if (!element.matches?.("[data-row-delete]")) {
			console.warn("row-delete: component applied to element without [data-row-delete] attribute");
		}

		const handlers = [];
		const addHandler = (type, listener) => {
			element.addEventListener(type, listener);
			handlers.push({ type, listener });
		};

		addHandler("htmx:beforeRequest", () => {
			setRowRemoving(element, true);
		});

		addHandler("htmx:responseError", () => {
			setRowRemoving(element, false);
		});

		addHandler("htmx:sendError", () => {
			setRowRemoving(element, false);
		});

		addHandler("htmx:timeout", () => {
			setRowRemoving(element, false);
		});

		addHandler("htmx:afterRequest", (event) => {
			const status = event?.detail?.xhr?.status;
			if (status === 204) {
				setRowRemoving(element, false);
			}
		});

		addHandler("htmx:beforeSwap", (event) => {
			const detail = event?.detail;
			if (!detail) {
				return;
			}

			if (detail.isError) {
				setRowRemoving(element, false);
				return;
			}

			const status = detail?.xhr?.status;
			if (typeof status === "number" && status >= 400) {
				setRowRemoving(element, false);
			}
		});

		element.__rowDeleteHandlers = handlers;
	},
	(element) => {
		const handlers = element.__rowDeleteHandlers || [];
		handlers.forEach(({ type, listener }) => {
			element.removeEventListener(type, listener);
		});
		setRowRemoving(element, false);
		element.__rowDeleteHandlers = undefined;
	},
);

export default null;
