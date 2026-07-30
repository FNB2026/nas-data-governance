// @vitest-environment jsdom

import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import CopyButton from "./CopyButton";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

describe("CopyButton", () => {
  it("renders the copy icon by default", () => {
    render(<CopyButton text="abc" />);
    const btn = screen.getByRole("button");
    expect(btn).toHaveTextContent("⧉");
  });

  it("copies text and shows checkmark on click", async () => {
    const writeText = vi.fn().mockResolvedValue(undefined);
    Object.defineProperty(navigator, "clipboard", {
      value: { writeText },
      configurable: true,
    });

    render(<CopyButton text="hash-value" label="复制哈希" />);
    const btn = screen.getByRole("button");
    fireEvent.click(btn);

    // Wait for the async handler to complete
    await vi.waitFor(() => {
      expect(btn).toHaveTextContent("✓");
    });
    expect(writeText).toHaveBeenCalledWith("hash-value");
  });

  it("updates aria-label to indicate success after copy", async () => {
    const writeText = vi.fn().mockResolvedValue(undefined);
    Object.defineProperty(navigator, "clipboard", {
      value: { writeText },
      configurable: true,
    });

    render(<CopyButton text="test" label="复制" />);
    const btn = screen.getByRole("button");

    // Before click: generic label
    expect(btn).toHaveAttribute("aria-label", "复制");

    fireEvent.click(btn);

    await vi.waitFor(() => {
      expect(btn).toHaveAttribute("aria-label", "复制成功");
    });
  });

  it("does not show checkmark when copy fails", async () => {
    const writeText = vi.fn().mockRejectedValue(new Error("denied"));
    Object.defineProperty(navigator, "clipboard", {
      value: { writeText },
      configurable: true,
    });

    render(<CopyButton text="test" />);
    const btn = screen.getByRole("button");
    fireEvent.click(btn);

    // Wait for the rejected promise to settle
    await vi.waitFor(() => {
      expect(writeText).toHaveBeenCalled();
    });

    // Should still show the copy icon, not the checkmark
    expect(btn).toHaveTextContent("⧉");
  });

  it("applies custom className", () => {
    render(<CopyButton text="x" className="custom-cls" />);
    const btn = screen.getByRole("button");
    expect(btn.className).toContain("custom-cls");
  });
});
