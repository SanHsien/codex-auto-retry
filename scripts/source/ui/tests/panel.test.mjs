import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";
import vm from "node:vm";
import ts from "typescript";

// Only the DOM operations used by the panel are needed; no browser or server runs.
class Element extends EventTarget {
  constructor(tagName = "div") {
    super();
    this.tagName = tagName;
    this.children = [];
    this.dataset = {};
    this.attributes = {};
    this.style = {};
    this.classList = { add() {}, toggle() {} };
    this.value = "";
    this.textContent = "";
    this.open = false;
  }
  append(...children) { this.children.push(...children); }
  replaceChildren(...children) { this.children = children; }
  setAttribute(name, value) { this.attributes[name] = value; }
  getAttribute(name) { return this.attributes[name] ?? null; }
  querySelectorAll(selector) {
    const descendants = this.children.flatMap((child) => [child, ...child.querySelectorAll("*")]);
    if (selector === "*") return descendants;
    if (selector === "details[data-thread-id]") {
      return descendants.filter((child) => child.tagName === "details" && child.dataset.threadId);
    }
    if (selector === "details") return descendants.filter((child) => child.tagName === "details");
    return [];
  }
}

function panel() {
  const html = readFileSync(new URL("../panel.html", import.meta.url), "utf8");
  const elements = new Map([...html.matchAll(/id="([^"]+)"/g)].map(([, id]) => [id, new Element()]));
  const strategies = ["fixed", "exponential", "linear"].map((value) => Object.assign(new Element("input"), { value }));
  let app;
  const intervals = [];
  class App {
    constructor() { app = this; }
    connect() { return new Promise(() => {}); }
    async callServerTool() { return { structuredContent: next }; }
  }
  const context = vm.createContext({
    exports: {},
    require(name) {
      if (name === "@modelcontextprotocol/ext-apps") return { App };
      if (name === "lucide") return { createIcons() {} };
      if (name === "./panel.css") return {};
      throw new Error(`Unexpected import: ${name}`);
    },
    document: {
      documentElement: new Element("html"),
      getElementById: (id) => elements.get(id),
      createElement: (tag) => new Element(tag),
      createTextNode: (text) => Object.assign(new Element("#text"), { textContent: text }),
      querySelectorAll: (selector) => selector === 'input[name="delay-strategy"]' ? strategies : [],
    },
    window: {
      location: { search: "" },
      setInterval: (callback, delay) => intervals.push({ callback, delay }),
      setTimeout() {},
      clearTimeout() {},
    },
    URLSearchParams,
  });
  const source = readFileSync(new URL("../src/panel.ts", import.meta.url), "utf8");
  const { outputText } = ts.transpileModule(source, {
    compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2022 },
  });
  vm.runInContext(outputText, context);
  let next = vm.runInContext("previewSnapshot()", context);
  // All preview IDs share a prefix, so these tests also reject shortened ID keys.
  next.retries = next.retries.slice(0, 2);
  app.ontoolresult({ structuredContent: next });
  return {
    details: () => elements.get("queue-list").querySelectorAll("details"),
    async refresh(retries = next.retries) {
      next = { ...next, retries, last_scan_at: new Date().toISOString() };
      // Exercise the actual status polling callback and response rendering path.
      intervals.find(({ delay }) => delay > 1000).callback();
      await new Promise(setImmediate);
    },
    retries: next.retries,
  };
}

test("status refresh preserves only the expanded task using its full thread ID", async () => {
  const ui = panel();
  const oldDetails = ui.details();
  oldDetails[0].open = true;
  // A refresh may arrive before the browser dispatches the queued toggle event.
  await ui.refresh([...ui.retries].reverse());
  const details = ui.details();
  assert.notEqual(details[1], oldDetails[0], "the queue must actually be redrawn");
  assert.equal(details[1].open, true, "the expanded task must survive a refresh and reorder");
  assert.equal(details[0].open, false, "another task with the same ID prefix must stay collapsed");
});

test("status refresh respects a task collapsed after an earlier refresh", async () => {
  const ui = panel();
  ui.details()[0].open = true;
  ui.details()[0].dispatchEvent(new Event("toggle"));
  await ui.refresh();
  ui.details()[0].open = false;
  ui.details()[0].dispatchEvent(new Event("toggle"));
  await ui.refresh();
  assert.equal(ui.details()[0].open, false);
});

for (const emptyQueue of [false, true]) {
  test(`removed task loses its expanded state (${emptyQueue ? "empty queue" : "other task remains"})`, async () => {
    const ui = panel();
    ui.details()[0].open = true;
    ui.details()[0].dispatchEvent(new Event("toggle"));
    await ui.refresh();
    await ui.refresh(emptyQueue ? [] : [ui.retries[1]]);
    await ui.refresh(ui.retries);
    assert.equal(ui.details()[0].open, false, "a returning task must start collapsed");
    assert.equal(ui.details()[1].open, false);
  });
}
