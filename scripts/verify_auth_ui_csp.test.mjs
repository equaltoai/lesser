import assert from "node:assert/strict";
import fs from "node:fs";
import test from "node:test";

const source = fs.readFileSync(new URL("./verify_auth_ui_csp.sh", import.meta.url), "utf8");
const literal = source.match(/if \(!\/(.+)\/([a-z]*)\.test\(match\[1\]\)\) \{/);
if (literal === null) {
  throw new Error("could not find inline-script src detection regex");
}
const hasExternalSource = new RegExp(literal[1], literal[2]);

function inlineScriptContents(html) {
  const scripts = [];
  for (const match of html.matchAll(/<script\b([^>]*)>([\s\S]*?)<\/script>/gi)) {
    if (!hasExternalSource.test(match[1])) {
      scripts.push(match[2]);
    }
  }
  return scripts;
}

test("data-src remains inline while src is external", () => {
  const html = [
    '<script data-src="/metadata.js">data-src-inline()</script>',
    '<script src="/external.js">external()</script>',
    '<script type="module">ordinary-inline()</script>',
  ].join("");

  assert.deepEqual(inlineScriptContents(html), ["data-src-inline()", "ordinary-inline()"]);
});
