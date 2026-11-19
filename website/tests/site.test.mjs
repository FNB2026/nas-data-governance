import test from "node:test";
import assert from "node:assert/strict";
import worker from "../dist/worker.mjs";
const visit = (url, method = "GET") => worker.fetch(new Request(url, {method}));
test("canonical homepage keeps releases and security boundaries", async () => {
  const response = await visit("https://ndg.fnbapp.net/");
  const html = await response.text();
  assert.equal(response.status, 200);
  assert.match(html, /v0\.5\.0-beta\.11/);
  assert.match(html, /v0\.5\.0-docker-preview\.1/);
  assert.match(html, /永久清理/);
  assert.match(html, /style\.css/);
  assert.match(response.headers.get("Content-Security-Policy"), /style-src 'self'/);
});
test("light style and privacy are present", async () => {
  const cssResponse = await visit("https://ndg.fnbapp.net/style.css");
  assert.equal(cssResponse.status, 200);
  const css = await cssResponse.text();
  assert.match(css, /Light v2/);
  assert.match(css, /background:#fff/);
  assert.match(css, /#0071e3/);
  const privacy = await visit("https://ndg.fnbapp.net/privacy");
  assert.equal(privacy.status, 200);
  assert.match(await privacy.text(), /隐私说明/);
});
test("alias redirects with path and query", async () => {
  const redirect = await visit("https://ngd.fnbapp.net/privacy?q=1");
  assert.equal(redirect.status, 308);
  assert.equal(redirect.headers.get("Location"), "https://ndg.fnbapp.net/privacy?q=1");
});
test("health, headers, methods, and missing resources", async () => {
  for (const path of ["/healthz", "/robots.txt", "/sitemap.xml", "/favicon.svg"]) {
    assert.equal((await visit("https://ndg.fnbapp.net" + path)).status, 200);
  }
  assert.equal((await visit("https://ndg.fnbapp.net/", "HEAD")).status, 200);
  assert.equal((await visit("https://ndg.fnbapp.net/", "POST")).status, 405);
  assert.equal((await visit("https://ndg.fnbapp.net/404")).status, 404);
});
