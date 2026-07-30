import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { api, ApiError } from "./client";
import { wails } from "../wailsjs/go/models";

type BackendMock = ReturnType<typeof vi.fn>;

const backend: Record<string, BackendMock> = {};

function binding(name: string): BackendMock {
  backend[name] ??= vi.fn();
  return backend[name];
}

beforeEach(() => {
  for (const mock of Object.values(backend)) mock.mockReset();
  vi.stubGlobal("window", {
    go: { wails: { API: new Proxy(backend, { get: (_target, key: string) => binding(key) }) } },
    runtime: {},
  });
});

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("API client mutation boundary", () => {
  const mutationCases: Array<[string, string, () => Promise<unknown>]> = [
    ["start scan", "StartScan", () => api.scan.start({ root: "/source" } as wails.StartScanRequest)],
    ["cancel scan", "CancelScan", () => api.scan.cancel("job-1")],
    ["save drafts", "SaveDraftPlans", () => api.governance.saveDrafts("storage-1")],
    ["approve plans", "ApprovePlans", () => api.governance.approve({ plan_ids: ["plan-1"] } as wails.ApprovePlansRequest)],
    ["save decision", "SaveGroupDecision", () => api.governance.saveDecision({ group_id: "g1", decision_type: "KEEP_ALL" } as wails.SaveDecisionRequest)],
    ["execute plans", "ExecutePlans", () => api.execution.executePlans({ plan_ids: ["plan-1"] } as wails.ExecutePlansRequest)],
    ["create restore", "CreateRestorePlan", () => api.execution.createRestorePlan("item-1")],
    ["execute restore", "ExecuteRestore", () => api.execution.executeRestore({ plan_id: "restore-1" } as wails.ExecuteRestoreRequest)],
    ["create purge", "CreatePurgePlans", () => api.execution.createPurgePlans()],
    ["execute purge", "ExecutePurge", () => api.execution.executePurge({ plan_id: "purge-1" } as wails.ExecutePurgeRequest)],
    ["recover source", "RecoverSourcePlans", () => api.recovery.recoverSource()],
    ["recover restores", "RecoverRestores", () => api.recovery.recoverRestores({ quarantine_root: "/q" } as wails.RecoverRestoresRequest)],
    ["recover purges", "RecoverPurges", () => api.recovery.recoverPurges("/q")],
  ];

  it.each(mutationCases)("does not retry %s after an ambiguous network failure", async (_label, method, invoke) => {
    binding(method).mockRejectedValue(new Error("connection reset"));

    await expect(invoke()).rejects.toBeInstanceOf(ApiError);
    expect(binding(method)).toHaveBeenCalledTimes(1);
  });

  it("forwards the complete quarantine execution request unchanged", async () => {
    const request = {
      plan_ids: ["plan-1", "plan-2"],
      quarantine_root: "/quarantine",
      source_roots: ["/source-a", "/source-b"],
      dry_run: true,
      retention_hours: 720,
    } as wails.ExecutePlansRequest;
    binding("ExecutePlans").mockResolvedValue({ results: [], executed: 0, skipped: 0, failed: 0 });

    await api.execution.executePlans(request);

    expect(binding("ExecutePlans")).toHaveBeenCalledWith(request);
  });
});
