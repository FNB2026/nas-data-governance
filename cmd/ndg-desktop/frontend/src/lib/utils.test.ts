// @vitest-environment jsdom

import { afterEach, describe, expect, it, vi } from "vitest";
import {
  TERMINAL_STATES,
  formatBytes,
  shortHash,
  formatDateTime,
  stateLabel,
  stageLabel,
  eventLabel,
  stateBadgeClass,
  friendlyError,
  errorText,
  copyToClipboard,
  hasWailsRuntime,
} from "./utils";

// ---- formatBytes ----

describe("formatBytes", () => {
  it("returns 0 B for zero or negative", () => {
    expect(formatBytes(0)).toBe("0 B");
    expect(formatBytes(-1)).toBe("0 B");
  });

  it("formats bytes without decimal", () => {
    expect(formatBytes(512)).toBe("512.0 B");
  });

  it("formats kilobytes", () => {
    expect(formatBytes(1024)).toBe("1.0 KB");
    expect(formatBytes(1536)).toBe("1.5 KB");
  });

  it("formats megabytes", () => {
    expect(formatBytes(1048576)).toBe("1.0 MB");
  });

  it("formats gigabytes", () => {
    expect(formatBytes(1073741824)).toBe("1.0 GB");
  });

  it("caps at exabytes", () => {
    // Larger than EB range should still show EB, not overflow
    const huge = Math.pow(1024, 6) * 1024; // 1 ZB
    expect(formatBytes(huge)).toMatch(/EB$/);
  });
});

// ---- shortHash ----

describe("shortHash", () => {
  it("returns short hashes unchanged", () => {
    expect(shortHash("abc123")).toBe("abc123");
  });

  it("returns exactly 12-char hashes unchanged", () => {
    expect(shortHash("abcdef123456")).toBe("abcdef123456");
  });

  it("truncates hashes longer than 12 chars with ellipsis", () => {
    const hash = "abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890";
    const result = shortHash(hash);
    expect(result).toBe("abcdef12…7890");
    expect(result.length).toBeLessThan(hash.length);
  });
});

// ---- formatDateTime ----

describe("formatDateTime", () => {
  it("returns em-dash for empty string", () => {
    expect(formatDateTime("")).toBe("—");
  });

  it("parses valid ISO string", () => {
    const result = formatDateTime("2026-07-30T10:30:00Z");
    expect(result).not.toBe("—");
    expect(result).not.toBe("2026-07-30T10:30:00Z");
    // Should contain year and time digits
    expect(result).toMatch(/2026/);
  });

  it("returns original string for invalid date", () => {
    expect(formatDateTime("not-a-date")).toBe("not-a-date");
  });
});

// ---- Label functions ----

describe("stateLabel", () => {
  it("returns Chinese label for known state", () => {
    expect(stateLabel("COMPLETED")).toBe("已完成");
    expect(stateLabel("RUNNING")).toBe("运行中");
    expect(stateLabel("PAUSED_NETWORK")).toBe("网络中断，已暂停");
  });

  it("returns raw value for unknown state", () => {
    expect(stateLabel("UNKNOWN")).toBe("UNKNOWN");
  });
});

describe("stageLabel", () => {
  it("returns Chinese label for known stage", () => {
    expect(stageLabel("DISCOVERING")).toBe("发现文件");
    expect(stageLabel("FULL_HASHING")).toBe("完整哈希");
    expect(stageLabel("FINALIZING")).toBe("保存索引与收尾");
  });

  it("returns raw value for unknown stage", () => {
    expect(stageLabel("CUSTOM_STAGE")).toBe("CUSTOM_STAGE");
  });
});

describe("eventLabel", () => {
  it("returns Chinese label for known event", () => {
    expect(eventLabel("job:created")).toBe("创建");
    expect(eventLabel("job:completed")).toBe("完成");
  });

  it("returns raw value for unknown event", () => {
    expect(eventLabel("custom:event")).toBe("custom:event");
  });
});

// ---- stateBadgeClass ----

describe("stateBadgeClass", () => {
  it("generates kebab-case badge class", () => {
    expect(stateBadgeClass("COMPLETED")).toBe("state-badge state-badge--completed");
  });

  it("handles states with underscores", () => {
    expect(stateBadgeClass("CANCEL_REQUESTED")).toBe("state-badge state-badge--cancel-requested");
  });
});

// ---- TERMINAL_STATES ----

describe("TERMINAL_STATES", () => {
  it("includes completed, failed, and cancelled", () => {
    expect(TERMINAL_STATES.has("COMPLETED")).toBe(true);
    expect(TERMINAL_STATES.has("FAILED")).toBe(true);
    expect(TERMINAL_STATES.has("CANCELLED")).toBe(true);
    expect(TERMINAL_STATES.has("PAUSED_NETWORK")).toBe(true);
  });

  it("does not include running or queued", () => {
    expect(TERMINAL_STATES.has("RUNNING")).toBe(false);
    expect(TERMINAL_STATES.has("QUEUED")).toBe(false);
  });
});

// ---- errorText ----

describe("errorText", () => {
  it("extracts message from Error", () => {
    expect(errorText(new Error("boom"))).toBe("boom");
  });

  it("stringifies non-Error values", () => {
    expect(errorText(42)).toBe("42");
    expect(errorText("plain string")).toBe("plain string");
  });
});

// ---- friendlyError ----

describe("friendlyError", () => {
  it("maps database schema errors", () => {
    const msg = friendlyError(new Error("no such table: files"));
    expect(msg).toContain("数据库结构不兼容");
    expect(msg).toContain("读写模式");
  });

  it("maps database locked errors", () => {
    const msg = friendlyError(new Error("database is locked"));
    expect(msg).toContain("数据库被占用");
  });

  it("maps read-only write errors", () => {
    const msg = friendlyError(new Error("attempt to write in read-only mode"));
    expect(msg).toContain("只读模式");
  });

  it("maps symlink errors", () => {
    const msg = friendlyError(new Error("symbolic link detected"));
    expect(msg).toContain("符号链接");
  });

  it("maps permission denied errors", () => {
    const msg = friendlyError(new Error("permission denied"));
    expect(msg).toContain("权限不足");
  });

  it("maps path not found errors", () => {
    const msg = friendlyError(new Error("no such file or directory"));
    expect(msg).toContain("路径不存在");
  });

  it("maps purge confirmation errors", () => {
    const msg = friendlyError(new Error("purge confirmation rejected"));
    expect(msg).toContain("确认语句不匹配");
  });

  it("maps dry-run required errors", () => {
    const msg = friendlyError(new Error("dry-run is required before purge"));
    expect(msg).toContain("试运行");
  });

  it("maps stale plan errors", () => {
    const msg = friendlyError(new Error("plan is stale"));
    expect(msg).toContain("过期");
  });

  it("maps recovery lock errors", () => {
    const msg = friendlyError(new Error("recovery lock active"));
    expect(msg).toContain("恢复锁");
  });

  it("maps connection errors", () => {
    const msg = friendlyError(new Error("connection refused"));
    expect(msg).toContain("连接超时");
  });

  it("returns original message for unrecognized errors", () => {
    const msg = friendlyError(new Error("something unexpected"));
    expect(msg).toBe("something unexpected");
  });
});

// ---- copyToClipboard ----

describe("copyToClipboard", () => {
  const originalClipboard = navigator.clipboard;

  afterEach(() => {
    // Restore original clipboard
    Object.defineProperty(navigator, "clipboard", {
      value: originalClipboard,
      configurable: true,
      writable: true,
    });
  });

  it("succeeds when navigator.clipboard.writeText resolves", async () => {
    const writeText = vi.fn().mockResolvedValue(undefined);
    Object.defineProperty(navigator, "clipboard", {
      value: { writeText },
      configurable: true,
    });

    const ok = await copyToClipboard("test-value");
    expect(ok).toBe(true);
    expect(writeText).toHaveBeenCalledWith("test-value");
  });

  it("falls back to execCommand when clipboard API is unavailable", async () => {
    Object.defineProperty(navigator, "clipboard", {
      value: undefined,
      configurable: true,
    });

    // jsdom doesn't define execCommand; assign a mock directly
    const execCommand = vi.fn().mockReturnValue(true);
    document.execCommand = execCommand;

    const ok = await copyToClipboard("fallback-test");
    expect(ok).toBe(true);
    expect(execCommand).toHaveBeenCalledWith("copy");
  });

  it("returns false when all methods fail", async () => {
    Object.defineProperty(navigator, "clipboard", {
      value: undefined,
      configurable: true,
    });

    document.execCommand = vi.fn(() => {
      throw new Error("not allowed");
    });

    const ok = await copyToClipboard("will-fail");
    expect(ok).toBe(false);
  });
});

// ---- hasWailsRuntime ----

describe("hasWailsRuntime", () => {
  it("returns false when window.go is missing", () => {
    expect(hasWailsRuntime()).toBe(false);
  });
});
