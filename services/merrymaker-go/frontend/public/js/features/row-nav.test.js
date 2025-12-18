import { afterEach, beforeEach, describe, expect, it } from "bun:test";
import { initRowNav } from "./row-nav.js";

describe("row-nav feature", () => {
	let container;
	let triggerCalls;
	let originalDescriptor;

	beforeEach(() => {
		triggerCalls = [];

		originalDescriptor = Object.getOwnPropertyDescriptor(window, "htmx");

		Object.defineProperty(window, "htmx", {
			configurable: true,
			writable: true,
			value: {
				trigger: (target, eventName) => {
					triggerCalls.push({ target, eventName });
				},
			},
		});

		container = document.createElement("div");
		container.innerHTML = `
			<table>
				<tbody>
					<tr class="row-link"
						hx-get="/rows/1"
						hx-trigger="row-nav"
						role="link"
						tabindex="0">
						<td>Row 1</td>
						<td>
							<button type="button" data-stop-row-nav>Delete</button>
						</td>
					</tr>
					<tr class="row-link"
						data-row-nav-target="[data-row-nav-trigger]"
						role="link"
						tabindex="0">
						<td>
							<a data-row-nav-trigger hx-get="/rows/2" hx-trigger="row-nav">Go</a>
						</td>
						<td>
							<button type="button" data-stop-row-nav>Archive</button>
						</td>
					</tr>
				</tbody>
			</table>
		`;

		document.body.appendChild(container);
		initRowNav();
	});

	afterEach(() => {
		if (originalDescriptor) {
			Object.defineProperty(window, "htmx", originalDescriptor);
		} else {
			window.htmx = undefined;
		}

		if (container?.parentNode) {
			container.remove();
		}
	});

	it("triggers row-nav on row click", () => {
		const row = container.querySelector(".row-link");
		row?.dispatchEvent(new MouseEvent("click", { bubbles: true }));

		const match = triggerCalls.find((call) => call.target === row && call.eventName === "row-nav");
		expect(match).toBeTruthy();
	});

	it("does not trigger when clicking a stop control", () => {
		const button = container.querySelector("[data-stop-row-nav]");
		button?.dispatchEvent(new MouseEvent("click", { bubbles: true }));

		expect(triggerCalls.length).toBe(0);
	});

	it("triggers on Enter key", () => {
		const row = container.querySelector(".row-link");
		const event = new KeyboardEvent("keydown", { key: "Enter", bubbles: true, cancelable: true });
		row?.dispatchEvent(event);

		expect(event.defaultPrevented).toBe(true);
		const match = triggerCalls.find((call) => call.target === row && call.eventName === "row-nav");
		expect(match).toBeTruthy();
	});

	it("uses custom target when data-row-nav-target present", () => {
		const row = container.querySelector("[data-row-nav-target]");
		const target = row?.querySelector("[data-row-nav-trigger]");

		row?.dispatchEvent(new MouseEvent("click", { bubbles: true }));

		const match = triggerCalls.find(
			(call) => call.target === target && call.eventName === "row-nav",
		);
		expect(match).toBeTruthy();
	});
});
