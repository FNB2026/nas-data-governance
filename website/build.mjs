import {readFileSync, mkdirSync, writeFileSync} from "node:fs";
import {fileURLToPath} from "node:url";
import {dirname, join} from "node:path";
const root = dirname(fileURLToPath(import.meta.url));
const get = name => readFileSync(join(root, name), "utf8");
const assets = {
  "/": {body: get("index.html"), type: "text/html; charset=utf-8"},
  "/mac-app-privacy": {body: get("mac-app-privacy.html"), type: "text/html; charset=utf-8"},
  "/privacy": {body: get("privacy.html"), type: "text/html; charset=utf-8"},
  "/style.css": {body: get("style.css"), type: "text/css; charset=utf-8"},
  "/favicon.svg": {body: get("favicon.svg"), type: "image/svg+xml; charset=utf-8"}
};
const source = "const ASSETS=" + JSON.stringify(assets) + ";\n" + get("worker-template.mjs");
mkdirSync(join(root, "dist"), {recursive: true});
writeFileSync(join(root, "dist/worker.mjs"), source);
console.log("Built website/dist/worker.mjs", Buffer.byteLength(source), "bytes");
