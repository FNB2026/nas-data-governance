import test from "node:test";
import assert from "node:assert/strict";
import {createHash} from "node:crypto";
import worker from "../dist/worker.mjs";
const visit = (url, method = "GET") => worker.fetch(new Request(url, {method}));
const digest = value => createHash("sha256").update(value).digest("hex").slice(0, 10);
test("canonical homepage keeps releases and security boundaries", async () => {
  const response = await visit("https://ndg.fnbapp.net/");
  const html = await response.text();
  assert.equal(response.status, 200);
  assert.match(html, /v0\.5\.0-beta\.11/);
  assert.match(html, /v0\.5\.0-docker-preview\.1/);
  assert.match(html, /永久清理/);
  assert.match(html, /style\.css/);
  assert.equal((html.match(/class="mark" aria-hidden="true">NDG<\/span>/g) || []).length, 2);
  assert.equal((html.match(/class="brand-text"/g) || []).length, 2);
  assert.match(response.headers.get("Content-Security-Policy"), /style-src 'self'/);
});
test("light style and privacy are present", async () => {
  const cssResponse = await visit("https://ndg.fnbapp.net/style.css");
  assert.equal(cssResponse.status, 200);
  const css = await cssResponse.text();
  assert.match(css, /Light v2/);
  assert.match(css, /background:#fff/);
  assert.match(css, /#0071e3/);
  assert.match(css, /Official NDG wordmark/);
  assert.match(css, /\.brand \.brand-sub\{display:block/);
  const favicon = await (await visit("https://ndg.fnbapp.net/favicon.svg")).text();
  assert.match(favicon, /NDG<\/text>/);
  assert.match(favicon, /fill="#0071e3"/);
  const privacy = await visit("https://ndg.fnbapp.net/privacy");
  assert.equal(privacy.status, 200);
  assert.match(await privacy.text(), /隐私说明/);
});
test("alias redirects with path and query", async () => {
  const redirect = await visit("https://ngd.fnbapp.net/privacy?q=1");
  assert.equal(redirect.status, 308);
  assert.equal(redirect.headers.get("Location"), "https://ndg.fnbapp.net/privacy?q=1");
});
test("static assets carry content hashes and only hashed URLs are immutable", async () => {
  const html = await (await visit("https://ndg.fnbapp.net/")).text();
  const cssHref = html.match(/href="\/style\.css\?v=([0-9a-f]{10})"/);
  const iconHref = html.match(/href="\/favicon\.svg\?v=([0-9a-f]{10})"/);
  assert.ok(cssHref, "homepage must reference a hashed stylesheet");
  assert.ok(iconHref, "homepage must reference a hashed favicon");
  const css = await (await visit("https://ndg.fnbapp.net/style.css")).text();
  const icon = await (await visit("https://ndg.fnbapp.net/favicon.svg")).text();
  assert.equal(cssHref[1], digest(css));
  assert.equal(iconHref[1], digest(icon));
  const hashed = await visit("https://ndg.fnbapp.net/style.css?v=" + cssHref[1]);
  assert.equal(hashed.status, 200);
  assert.equal(hashed.headers.get("Cache-Control"), "public, max-age=31536000, immutable");
  const bare = await visit("https://ndg.fnbapp.net/style.css");
  assert.equal(bare.headers.get("Cache-Control"), "public, max-age=300, must-revalidate");
  const privacy = await (await visit("https://ndg.fnbapp.net/privacy")).text();
  assert.match(privacy, /href="\/style\.css\?v=[0-9a-f]{10}"/);
});
test("health, headers, methods, and missing resources", async () => {
  for (const path of ["/healthz", "/robots.txt", "/sitemap.xml", "/favicon.svg"]) {
    assert.equal((await visit("https://ndg.fnbapp.net" + path)).status, 200);
  }
  assert.equal((await visit("https://ndg.fnbapp.net/", "HEAD")).status, 200);
  assert.equal((await visit("https://ndg.fnbapp.net/", "POST")).status, 405);
  assert.equal((await visit("https://ndg.fnbapp.net/404")).status, 404);
});
