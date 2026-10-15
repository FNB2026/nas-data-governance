export interface Source { alias: string }
export interface Snapshot { state: string; stage: string; discovered: number; processed: number; failed: number; checkpoint: string; scanned: number; active: number; missing: number; unavailable: number; quick: number; full: number; groups: Array<{ id: string; copies: number; bytes: number }> }

// No response body, URL, credential or raw transport error crosses this boundary.
export class WebRequestError extends Error {
  constructor(readonly status: number) {
    super(status === 401 ? "登录凭据不正确或会话已失效，请重新登录。"
      : status === 403 ? "请求未获授权，请重新登录后再试。"
      : status === 409 ? "当前来源或扫描状态不允许此操作，请刷新状态后检查。"
      : status === 429 ? "请求过于频繁，请稍后重试。"
      : status >= 500 || status === 0 ? "暂时无法连接服务，请检查服务状态后重试。"
      : "请求未完成，请检查所选来源后重试。");
    this.name = "WebRequestError";
  }
}
// HttpOnly cookies; CSRF only in memory. Relative URLs support a fixed CGI prefix.
// All requests are bounded and made once; mutations are never replayed.
export class HttpAdapter {
  private csrf = "";
  reset(): void { this.csrf = ""; }
  private async request<T>(path: string, body?: unknown, signal?: AbortSignal): Promise<T> {
    const requestCSRF = this.csrf;
    const controller = new AbortController();
    const abort = () => controller.abort();
    signal?.addEventListener("abort", abort, { once: true });
    if (signal?.aborted) controller.abort();
    const timer = setTimeout(abort, 15000);
    try {
      const response = await fetch(path, {
        method: body === undefined ? "GET" : "POST", credentials: "same-origin",
        cache: "no-store", signal: controller.signal,
        headers: body === undefined ? {} : { "Content-Type": "application/json", "X-CSRF-Token": this.csrf },
        body: body === undefined ? undefined : JSON.stringify(body),
      });
      if (!response.ok) {
        if (response.status === 401 && this.csrf === requestCSRF) this.reset();
        throw new WebRequestError(response.status);
      }
      return await response.json() as T;
    } catch (error) {
      throw error instanceof WebRequestError ? error : new WebRequestError(0);
    } finally { clearTimeout(timer); signal?.removeEventListener("abort", abort); }
  }
  async session(): Promise<void> { const r = await this.request<{ csrf: string }>("api/session"); this.csrf = r.csrf; }
  async login(password: string): Promise<void> { this.reset(); const r = await this.request<{ csrf: string }>("api/login", { password }); this.csrf = r.csrf; }
  async logout(): Promise<void> { await this.request("api/logout", {}); this.reset(); }
  sources(): Promise<Source[]> { return this.request<Source[]>("api/sources"); }
  status(alias: string, signal?: AbortSignal): Promise<Snapshot> { return this.request<Snapshot>("api/status", { alias }, signal); }
  scan(alias: string, resume: boolean): Promise<unknown> { return this.request("api/scan", { alias, resume }); }
  cancel(alias: string): Promise<unknown> { return this.request("api/cancel", { alias }); }
}
