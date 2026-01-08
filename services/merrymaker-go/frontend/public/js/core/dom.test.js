import { beforeEach, describe, expect, it } from "bun:test";
import { clearElement, h, htmlToElement, htmlToFragment, qs, qsa } from "./dom.js";

describe("DOM Utilities", () => {
	describe("h() - Element Creation", () => {
		it("should create a simple element", () => {
			const div = h("div");
			expect(div.tagName).toBe("DIV");
		});

		it("should set className", () => {
			const div = h("div", { className: "container box" });
			expect(div.className).toBe("container box");
		});

		it("should set text content", () => {
			const p = h("p", { text: "Hello World" });
			expect(p.textContent).toBe("Hello World");
		});

		it("should set data attributes", () => {
			const div = h("div", { "data-component": "test", "data-value": "123" });
			expect(div.getAttribute("data-component")).toBe("test");
			expect(div.getAttribute("data-value")).toBe("123");
		});

		it("should handle boolean attributes", () => {
			const input = h("input", { disabled: true, required: true });
			expect(input.hasAttribute("disabled")).toBe(true);
			expect(input.hasAttribute("required")).toBe(true);
		});

		it("should handle event handlers", () => {
			let clicked = false;
			const button = h("button", {
				onClick: () => {
					clicked = true;
				},
			});
			button.click();
			expect(clicked).toBe(true);
		});

		it("should handle style object", () => {
			const div = h("div", {
				style: { color: "red", fontSize: "16px" },
			});
			expect(div.style.color).toBe("red");
			expect(div.style.fontSize).toBe("16px");
		});

		it("should append string children", () => {
			const div = h("div", {}, "Hello");
			expect(div.textContent).toBe("Hello");
		});

		it("should append element children", () => {
			const child = h("span", { text: "Child" });
			const parent = h("div", {}, child);
			expect(parent.children.length).toBe(1);
			expect(parent.children[0].textContent).toBe("Child");
		});

		it("should append array of children", () => {
			const div = h("div", {}, [
				h("span", { text: "A" }),
				h("span", { text: "B" }),
				h("span", { text: "C" }),
			]);
			expect(div.children.length).toBe(3);
			expect(div.textContent).toBe("ABC");
		});

		it("should handle nested children arrays", () => {
			const div = h("div", {}, [h("p", {}, [h("span", { text: "Nested" })])]);
			expect(div.querySelector("span")?.textContent).toBe("Nested");
		});
	});

	describe("qs() - Query Selector", () => {
		beforeEach(() => {
			document.body.innerHTML = `
        <div id="container">
          <p class="text">Paragraph</p>
          <span class="text">Span</span>
        </div>
      `;
		});

		it("should find element by id", () => {
			const el = qs("#container");
			expect(el).not.toBeNull();
			expect(el?.id).toBe("container");
		});

		it("should find element by class", () => {
			const el = qs(".text");
			expect(el).not.toBeNull();
			expect(el?.tagName).toBe("P");
		});

		it("should search within root element", () => {
			const container = qs("#container");
			const span = qs("span", container);
			expect(span).not.toBeNull();
			expect(span?.tagName).toBe("SPAN");
		});

		it("should return null if not found", () => {
			const el = qs("#nonexistent");
			expect(el).toBeNull();
		});
	});

	describe("qsa() - Query Selector All", () => {
		beforeEach(() => {
			document.body.innerHTML = `
        <div id="container">
          <p class="text">P1</p>
          <p class="text">P2</p>
          <span class="text">S1</span>
        </div>
      `;
		});

		it("should find all matching elements", () => {
			const elements = qsa(".text");
			expect(elements.length).toBe(3);
		});

		it("should return array", () => {
			const elements = qsa(".text");
			expect(Array.isArray(elements)).toBe(true);
		});

		it("should search within root element", () => {
			const container = qs("#container");
			const paragraphs = qsa("p", container);
			expect(paragraphs.length).toBe(2);
		});

		it("should return empty array if not found", () => {
			const elements = qsa(".nonexistent");
			expect(elements.length).toBe(0);
		});
	});

	describe("htmlToFragment()", () => {
		it("should create document fragment from HTML", () => {
			const fragment = htmlToFragment("<div>A</div><div>B</div>");
			expect(fragment.children.length).toBe(2);
		});

		it("should handle complex HTML", () => {
			const fragment = htmlToFragment(`
        <div class="container">
          <h1>Title</h1>
          <p>Content</p>
        </div>
      `);
			expect(fragment.querySelector("h1")?.textContent).toBe("Title");
		});
	});

	describe("htmlToElement()", () => {
		it("should create element from HTML", () => {
			const div = htmlToElement('<div class="box">Content</div>');
			expect(div?.tagName).toBe("DIV");
			expect(div?.className).toBe("box");
			expect(div?.textContent).toBe("Content");
		});

		it("should return first element only", () => {
			const div = htmlToElement("<div>A</div><div>B</div>");
			expect(div?.textContent).toBe("A");
		});
	});

	describe("clearElement()", () => {
		it("should remove all children", () => {
			const div = h("div", {}, [h("span", { text: "A" }), h("span", { text: "B" })]);
			expect(div.children.length).toBe(2);

			clearElement(div);
			expect(div.children.length).toBe(0);
		});

		it("should handle empty element", () => {
			const div = h("div");
			clearElement(div);
			expect(div.children.length).toBe(0);
		});
	});
});
