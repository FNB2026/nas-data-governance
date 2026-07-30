import { friendlyError } from "../lib/utils";
import { retryWithBackoff } from "../lib/retry";

/** Error exposed by the API layer with a user-facing message. */
export class ApiError extends Error {
  readonly raw: unknown;

  constructor(raw: unknown) {
    super(friendlyError(raw));
    this.name = "ApiError";
    this.raw = raw;
  }
}

/** Runs an idempotent read operation with network-error retries. */
export async function callRead<T>(fn: () => Promise<T>, retries = 3): Promise<T> {
  try {
    return await retryWithBackoff(fn, { maxRetries: retries });
  } catch (error) {
    throw new ApiError(error);
  }
}

/**
 * Runs an operation exactly once. Use for every mutation and for polling
 * operations whose caller owns its retry schedule.
 */
export async function callOnce<T>(fn: () => Promise<T>): Promise<T> {
  try {
    return await fn();
  } catch (error) {
    throw new ApiError(error);
  }
}
