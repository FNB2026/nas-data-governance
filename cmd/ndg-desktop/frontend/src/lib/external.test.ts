import { afterEach, describe, expect, it, vi } from "vitest";
import { normalizeExternalUrl, openExternal } from "./external";
import { PRODUCT_LINKS, PRODUCT } from "../app/productInfo";

const globalWithWindow = globalThis as unknown as { window?: unknown };
const originalWindow = globalWithWindow.window;

afterEach(() => {
  if (originalWindow === undefined) {
    delete globalWithWindow.window;
  } else {
    globalWithWindow.window = originalWindow;
  }
  vi.restoreAllMocks();
});

describe("normalizeExternalUrl", () => {
  it("percent-encodes non-ASCII path segments", () => {
    const normalized = normalizeExternalUrl(PRODUCT_LINKS[1].url);
    expect(normalized).not.toMatch(/[^\x20-\x7E]/);
    expect(normalized.startsWith("https://github.com/")).toBe(true);
  });

  it("keeps the anchor while encoding its non-ASCII segment", () => {
    const normalized = normalizeExternalUrl(PRODUCT_LINKS[3].url);
    expect(normalized).toContain("#");
    expect(normalized).not.toMatch(/[^\x20-\x7E]/);
  });

  it("leaves an ASCII URL unchanged", () => {
    expect(normalizeExternalUrl(PRODUCT.repoUrl)).toBe(PRODUCT.repoUrl);
  });
});

describe("openExternal", () => {
  it("returns false for an empty URL", () => {
    expect(openExternal("")).toBe(false);
  });

  it("uses the native browser opener inside the Wails runtime", () => {
    const browserOpenURL = vi.fn();
    globalWithWindow.window = { go: {}, runtime: { BrowserOpenURL: browserOpenURL } };

    expect(openExternal(PRODUCT.repoUrl)).toBe(true);
    expect(browserOpenURL).toHaveBeenCalledWith(PRODUCT.repoUrl);
  });

  it("falls back to window.open outside the Wails runtime", () => {
    const open = vi.fn();
    globalWithWindow.window = { open };

    expect(openExternal(PRODUCT.repoUrl)).toBe(true);
    expect(open).toHaveBeenCalledWith(PRODUCT.repoUrl, "_blank", "noopener,noreferrer");
  });

  it("reports failure when no opener is available", () => {
    delete globalWithWindow.window;
    expect(openExternal(PRODUCT.repoUrl)).toBe(false);
  });
});

describe("product identity links", () => {
  it("pins every link to the project repository and keeps them external", () => {
    expect(PRODUCT_LINKS.length).toBeGreaterThanOrEqual(5);

    const ids = PRODUCT_LINKS.map((link) => link.id);
    expect(ids).toEqual(["github", "guide", "license", "privacy", "issues"]);

    for (const link of PRODUCT_LINKS) {
      expect(link.url.startsWith("https://github.com/FNB2026/nas-data-governance")).toBe(true);
      expect(link.external).toBe(true);
    }
  });

  it("does not introduce an auto-update or account endpoint", () => {
    for (const link of PRODUCT_LINKS) {
      expect(link.url).not.toMatch(/releases\/latest|update|auth|login|telemetry/i);
    }
  });
});
