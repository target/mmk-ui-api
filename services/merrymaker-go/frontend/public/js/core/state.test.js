import { beforeEach, describe, expect, it } from "bun:test";
import { State } from "./state.js";

describe("State Management", () => {
	let state;

	beforeEach(() => {
		state = new State({
			count: 0,
			name: "Test",
			items: [],
		});
	});

	describe("constructor", () => {
		it("should initialize with empty state", () => {
			const emptyState = new State();
			expect(emptyState.getAll()).toEqual({});
		});

		it("should initialize with provided state", () => {
			expect(state.get("count")).toBe(0);
			expect(state.get("name")).toBe("Test");
		});
	});

	describe("get()", () => {
		it("should get state value", () => {
			expect(state.get("count")).toBe(0);
			expect(state.get("name")).toBe("Test");
		});

		it("should return undefined for non-existent key", () => {
			expect(state.get("nonexistent")).toBeUndefined();
		});
	});

	describe("set()", () => {
		it("should set state value", () => {
			state.set("count", 42);
			expect(state.get("count")).toBe(42);
		});

		it("should update existing value", () => {
			state.set("name", "Updated");
			expect(state.get("name")).toBe("Updated");
		});

		it("should not notify if value unchanged", () => {
			let callCount = 0;
			state.subscribe("count", () => {
				callCount++;
			});

			state.set("count", 0); // Same value
			expect(callCount).toBe(0);
		});
	});

	describe("subscribe()", () => {
		it("should notify subscriber on change", () => {
			let newValue, oldValue;
			state.subscribe("count", (n, o) => {
				newValue = n;
				oldValue = o;
			});

			state.set("count", 5);
			expect(newValue).toBe(5);
			expect(oldValue).toBe(0);
		});

		it("should support multiple subscribers", () => {
			let count1 = 0,
				count2 = 0;
			state.subscribe("count", () => {
				count1++;
			});
			state.subscribe("count", () => {
				count2++;
			});

			state.set("count", 1);
			expect(count1).toBe(1);
			expect(count2).toBe(1);
		});

		it("should only notify subscribers for changed key", () => {
			let countCalls = 0,
				nameCalls = 0;
			state.subscribe("count", () => {
				countCalls++;
			});
			state.subscribe("name", () => {
				nameCalls++;
			});

			state.set("count", 1);
			expect(countCalls).toBe(1);
			expect(nameCalls).toBe(0);
		});

		it("should return unsubscribe function", () => {
			let callCount = 0;
			const unsubscribe = state.subscribe("count", () => {
				callCount++;
			});

			state.set("count", 1);
			expect(callCount).toBe(1);

			unsubscribe();
			state.set("count", 2);
			expect(callCount).toBe(1); // Not called again
		});

		it("should throw error if callback is not a function", () => {
			expect(() => {
				state.subscribe("count", "not a function");
			}).toThrow("Callback must be a function");
		});

		it("should handle errors in subscribers gracefully", () => {
			let goodCallbackCalled = false;

			state.subscribe("count", () => {
				throw new Error("Subscriber error");
			});

			state.subscribe("count", () => {
				goodCallbackCalled = true;
			});

			// Should not throw, but log error
			state.set("count", 1);
			expect(goodCallbackCalled).toBe(true);
		});
	});

	describe("getAll()", () => {
		it("should return all state", () => {
			const all = state.getAll();
			expect(all).toEqual({
				count: 0,
				name: "Test",
				items: [],
			});
		});

		it("should return copy of state", () => {
			const all = state.getAll();
			all.count = 999;
			expect(state.get("count")).toBe(0); // Original unchanged
		});
	});

	describe("reset()", () => {
		it("should reset state to new values", () => {
			state.set("count", 10);
			state.set("name", "Changed");

			state.reset({ count: 0, name: "Reset" });
			expect(state.get("count")).toBe(0);
			expect(state.get("name")).toBe("Reset");
		});

		it("should notify subscribers on reset", () => {
			let countCalls = 0,
				nameCalls = 0;
			state.subscribe("count", () => {
				countCalls++;
			});
			state.subscribe("name", () => {
				nameCalls++;
			});

			state.set("count", 10);
			state.set("name", "Changed");

			// Reset to different values
			state.reset({ count: 0, name: "Reset" });

			// Should be called once for set, once for reset
			expect(countCalls).toBe(2);
			expect(nameCalls).toBe(2);
		});

		it("should reset to empty state", () => {
			state.reset();
			expect(state.getAll()).toEqual({});
		});
	});

	describe("complex scenarios", () => {
		it("should handle object values", () => {
			state.set("user", { id: 1, name: "Alice" });
			const user = state.get("user");
			expect(user).toEqual({ id: 1, name: "Alice" });
		});

		it("should handle array values", () => {
			state.set("items", [1, 2, 3]);
			const items = state.get("items");
			expect(items).toEqual([1, 2, 3]);
		});

		it("should handle null and undefined", () => {
			state.set("nullable", null);
			state.set("undefinable", undefined);
			expect(state.get("nullable")).toBeNull();
			expect(state.get("undefinable")).toBeUndefined();
		});

		it("should support chaining subscriptions", () => {
			const log = [];

			state.subscribe("count", (newVal) => {
				log.push(`count: ${newVal}`);
				if (newVal === 5) {
					state.set("name", "Five");
				}
			});

			state.subscribe("name", (newVal) => {
				log.push(`name: ${newVal}`);
			});

			state.set("count", 5);
			expect(log).toEqual(["count: 5", "name: Five"]);
		});
	});
});
