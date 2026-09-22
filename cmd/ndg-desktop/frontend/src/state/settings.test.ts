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

  it("persists privacy, scan defaults and onboarding state together", () => {
    const settings = {
      pathPrivacyMode: true,
      defaultFullScan: true,
      defaultWorkers: "8",
      onboardingDone: true,
      gettingStartedDone: false,
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
      onboardingDone: false,
      gettingStartedDone: false,
    });
  });

  it("defaults onboarding state to not-seen for a first launch", () => {
    expect(loadSettings()).toEqual({
      pathPrivacyMode: false,
      defaultFullScan: false,
      defaultWorkers: "",
      onboardingDone: false,
      gettingStartedDone: false,
    });
  });

  it("treats a non-boolean onboarding flag as not seen", () => {
    values.set(
      "ndg-settings",
      JSON.stringify({ onboardingDone: "yes", gettingStartedDone: 1 }),
    );

    const loaded = loadSettings();
    expect(loaded.onboardingDone).toBe(false);
    expect(loaded.gettingStartedDone).toBe(false);
  });
});
