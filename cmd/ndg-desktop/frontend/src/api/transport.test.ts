import { afterEach, describe, expect, it, vi } from "vitest";
import { ApiError, callOnce, callRead } from "./transport";

describe("API transport retry policy", () => {
  afterEach(() => {
    vi.useRealTimers();
    vi.restoreAllMocks();
  });

  it("never retries a mutating operation after a network error", async () => {
    const operation = vi.fn().mockRejectedValue(new Error("connection reset"));

    await expect(callOnce(operation)).rejects.toBeInstanceOf(ApiError);
    expect(operation).toHaveBeenCalledTimes(1);
  });

  it("retries an idempotent read after transient network errors", async () => {
    vi.useFakeTimers();
    vi.spyOn(Math, "random").mockReturnValue(0);
    const operation = vi.fn()
      .mockRejectedValueOnce(new Error("network is unreachable"))
      .mockRejectedValueOnce(new Error("connection reset"))
      .mockResolvedValue("ready");

    const result = callRead(operation, 2);
    const assertion = expect(result).resolves.toBe("ready");
    await vi.runAllTimersAsync();
    await assertion;
    expect(operation).toHaveBeenCalledTimes(3);
  });

  it("does not retry a non-network read failure", async () => {
    const operation = vi.fn().mockRejectedValue(new Error("permission denied"));

    await expect(callRead(operation)).rejects.toBeInstanceOf(ApiError);
    expect(operation).toHaveBeenCalledTimes(1);
  });
});
