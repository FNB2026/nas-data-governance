import { callOnce, callRead } from "./transport";
import type { BatchResult, GovernanceCapabilities, GovernanceQuery, PreviewRequest, ReviewPage, ReviewPreview } from "../lib/governance";

type Bridge = Record<string, unknown>;
function bridge(): Bridge {
  return (window as unknown as { go?: { wails?: { API?: Bridge } } }).go?.wails?.API ?? {};
}
function invoke<T>(name: string, ...args: unknown[]): Promise<T> {
  const receiver = bridge();
  const fn = receiver[name];
  if (typeof fn !== "function") return Promise.reject(new Error("UNAVAILABLE"));
  return (fn as (...input: unknown[]) => Promise<T>).apply(receiver, args);
}

// Resolve only explicit high-level use cases. Never fall back to unsafe legacy
// single writes or execution APIs when a new build lacks this contract.
export const governanceApi = {
  available: (): boolean => ["ListGovernanceGroups", "GetGovernanceCapabilities", "PreviewBatchDecisions", "CommitBatchDecisions", "GetBatchDecisionResult"].every((key) => typeof bridge()[key] === "function"),
  capabilities: (): Promise<GovernanceCapabilities> => callRead(() => invoke("GetGovernanceCapabilities")),
  list: (query: GovernanceQuery): Promise<ReviewPage> => callRead(() => invoke("ListGovernanceGroups", query)),
  preview: (request: PreviewRequest): Promise<ReviewPreview> => callOnce(() => invoke("PreviewBatchDecisions", request)),
  commit: (previewToken: string, idempotencyKey: string): Promise<BatchResult> => callOnce(() => invoke("CommitBatchDecisions", { preview_token: previewToken, idempotency_key: idempotencyKey })),
  result: (id: string): Promise<BatchResult> => callRead(() => invoke("GetBatchDecisionResult", id)),
};
