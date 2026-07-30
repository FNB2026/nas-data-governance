import { beforeEach, describe, expect, it, vi } from "vitest";
import { loadSettings, saveSettings } from "./settings";

describe("settings persistence", () => {
  const values = new Map<string, string>();

  beforeEach(() => {
    values.clear();
    vi.stubGlobal("localStorage", {
      getItem: (key: string) => values.get(key) ?? null,
      setItem: (key: string, value: string) => values.set(key, value),
    });
  });

  it("persists privacy and scan defaults together", () => {
    const settings = {
      pathPrivacyMode: true,
      defaultFullScan: true,
      defaultWorkers: "8",
    };

    saveSettings(settings);
    expect(loadSettings()).toEqual(settings);
  });

  it("fills new scan defaults when loading legacy settings", () => {
    values.set("ndg-settings", JSON.stringify({ pathPrivacyMode: true }));

    expect(loadSettings()).toEqual({
      pathPrivacyMode: true,
      defaultFullScan: false,
      defaultWorkers: "",
    });
  });
});
