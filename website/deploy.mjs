import {readFileSync} from "node:fs";
import {fileURLToPath} from "node:url";
import {dirname, join} from "node:path";
const token = process.env.NDG_CLOUDFLARE_API_TOKEN;
const account = process.env.NDG_CLOUDFLARE_ACCOUNT_ID;
if (!token || !account) {
  console.error("Missing NDG_CLOUDFLARE_API_TOKEN or NDG_CLOUDFLARE_ACCOUNT_ID");
  process.exit(2);
}
const root = dirname(fileURLToPath(import.meta.url));
const code = readFileSync(join(root, "dist/worker.mjs"), "utf8");
const form = new FormData();
form.append("metadata", new Blob([JSON.stringify({
  main_module: "worker.mjs",
  compatibility_date: "2026-11-19",
  observability: {enabled: true, head_sampling_rate: 1}
})], {type: "application/json"}));
form.append("worker.mjs", new Blob([code], {type: "application/javascript+module"}), "worker.mjs");
const response = await fetch(
  "https://api.cloudflare.com/client/v4/accounts/" + encodeURIComponent(account) + "/workers/scripts/ndg-official-site",
  {method: "PUT", headers: {Authorization: "Bearer " + token}, body: form}
);
const result = await response.json().catch(() => ({}));
if (!response.ok || result.success !== true) {
  console.error("Worker deployment failed", response.status, JSON.stringify(result.errors || []));
  process.exit(1);
}
console.log("Deployed Cloudflare Worker:", result.result?.id, result.result?.deployment_id);
