import {readFileSync, mkdirSync, writeFileSync} from "node:fs";
import {createHash} from "node:crypto";
import {fileURLToPath} from "node:url";
import {dirname, join} from "node:path";
const root = dirname(fileURLToPath(import.meta.url));
const get = name => readFileSync(join(root, name), "utf8");
const version = value => createHash("sha256").update(value).digest("hex").slice(0, 10);
const css = get("style.css");
const icon = get("favicon.svg");
const cssVersion = version(css);
const iconVersion = version(icon);
// HTML only lives 300s in caches while versioned assets live a year, so every
// HTML reference has to carry the content hash; otherwise a CSS deploy leaves
// visitors on new markup and old styles for up to a day.
const reference = html => html
  .replace('href="/style.css"', `href="/style.css?v=${cssVersion}"`)
  .replace('href="/favicon.svg"', `href="/favicon.svg?v=${iconVersion}"`);
const assets = {
  "/": {body: reference(get("index.html")), type: "text/html; charset=utf-8"},
  "/privacy": {body: reference(get("privacy.html")), type: "text/html; charset=utf-8"},
  "/style.css": {body: css, type: "text/css; charset=utf-8"},
  "/favicon.svg": {body: icon, type: "image/svg+xml; charset=utf-8"}
};
const source = "const ASSETS=" + JSON.stringify(assets) + ";\n" + get("worker-template.mjs");
mkdirSync(join(root, "dist"), {recursive: true});
writeFileSync(join(root, "dist/worker.mjs"), source);
console.log("Built website/dist/worker.mjs", Buffer.byteLength(source), "bytes", "css:" + cssVersion, "icon:" + iconVersion);
