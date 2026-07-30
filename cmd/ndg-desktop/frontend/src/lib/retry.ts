// Retry utilities for network drive disconnection tolerance.
// Provides exponential backoff retry for async operations and polling.

/**
 * Default retry configuration for network-related failures.
 */
export const RETRY_CONFIG = {
  maxRetries: 5,
  baseDelayMs: 1000,
  maxDelayMs: 30000,
  jitterMs: 500,
} as const;

/**
 * Computes the delay for a given retry attempt using exponential backoff.
 * delay = min(baseDelay * 2^attempt, maxDelay) + random jitter
 */
export function computeBackoffDelay(
  attempt: number,
  baseDelayMs: number = RETRY_CONFIG.baseDelayMs,
  maxDelayMs: number = RETRY_CONFIG.maxDelayMs,
  jitterMs: number = RETRY_CONFIG.jitterMs,
): number {
  const exponential = Math.min(baseDelayMs * Math.pow(2, attempt), maxDelayMs);
  const jitter = Math.random() * jitterMs;
  return exponential + jitter;
}

/**
 * Checks if an error is likely caused by a network disconnection
 * (as opposed to a logic error or permission issue).
 */
export function isNetworkError(error: unknown): boolean {
  const msg = (error instanceof Error ? error.message : String(error)).toLowerCase();
  return (
    msg.includes("connection refused") ||
    msg.includes("connection reset") ||
    msg.includes("timeout") ||
    msg.includes("timed out") ||
    msg.includes("network is unreachable") ||
    msg.includes("host is down") ||
    msg.includes("no route to host") ||
    msg.includes("broken pipe") ||
    msg.includes("eof") ||
    msg.includes("disconnected") ||
    msg.includes("transport connection")
  );
}

/**
 * Retries an async operation with exponential backoff.
 * Only retries on network errors; non-network errors are thrown immediately.
 *
 * @param fn The async function to retry
 * @param config Retry configuration
 * @returns The result of the function, or throws after max retries
 */
export async function retryWithBackoff<T>(
  fn: () => Promise<T>,
  config: {
    maxRetries?: number;
    baseDelayMs?: number;
    maxDelayMs?: number;
    onRetry?: (attempt: number, delayMs: number, error: unknown) => void;
  } = {},
): Promise<T> {
  const maxRetries = config.maxRetries ?? RETRY_CONFIG.maxRetries;
  const baseDelayMs = config.baseDelayMs ?? RETRY_CONFIG.baseDelayMs;
  const maxDelayMs = config.maxDelayMs ?? RETRY_CONFIG.maxDelayMs;

  let lastError: unknown;

  for (let attempt = 0; attempt <= maxRetries; attempt++) {
    try {
      return await fn();
    } catch (error) {
      lastError = error;

      // Don't retry on non-network errors
      if (!isNetworkError(error)) {
        throw error;
      }

      // Don't retry on the last attempt
      if (attempt >= maxRetries) {
        throw error;
      }

      const delayMs = computeBackoffDelay(attempt, baseDelayMs, maxDelayMs);
      config.onRetry?.(attempt + 1, delayMs, error);

      await new Promise<void>((resolve) => setTimeout(resolve, delayMs));
    }
  }

  throw lastError;
}
