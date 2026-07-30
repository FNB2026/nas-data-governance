import { readdirSync, readFileSync } from "node:fs";
import { dirname, join, relative } from "node:path";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";

const srcRoot = dirname(dirname(fileURLToPath(import.meta.url)));

function sourceFiles(directory: string): string[] {
  return readdirSync(directory, { withFileTypes: true }).flatMap((entry) => {
    const path = join(directory, entry.name);
    if (entry.isDirectory()) {
      return entry.name === "wailsjs" ? [] : sourceFiles(path);
    }
    return /\.tsx?$/.test(entry.name) ? [path] : [];
  });
}

describe("frontend API architecture", () => {
  it("keeps raw Wails API imports inside the centralized client", () => {
    const offenders = sourceFiles(srcRoot)
      .filter((path) => !path.endsWith("api/client.ts") && !path.includes(".test."))
      .filter((path) => /wailsjs\/go\/wails\/API/.test(readFileSync(path, "utf8")))
      .map((path) => relative(srcRoot, path));

    expect(offenders).toEqual([]);
  });
});
