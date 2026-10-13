import { callOnce, callRead } from "../api/transport";
export interface Source { alias: string }
export interface Snapshot { state: string; stage: string; discovered: number; processed: number; failed: number; checkpoint: string; scanned: number; active: number; missing: number; unavailable: number; quick: number; full: number; groups: Array<{ id: string; copies: number; bytes: number }> }
// Cookies are HttpOnly. CSRF is held only in memory, never localStorage or URLs.
export class HttpAdapter {
  private csrf = "";
  private async request<T>(path: string, body?: unknown): Promise<T> {
    const response = await fetch(path, { method: body === undefined ? "GET" : "POST", credentials: "same-origin", headers: body === undefined ? {} : { "Content-Type": "application/json", "X-CSRF-Token": this.csrf }, body: body === undefined ? undefined : JSON.stringify(body) });
    if (!response.ok) throw new Error("Request rejected. Verify your session and the approved source.");
    return response.json() as Promise<T>;
  }
  async login(password: string): Promise<void> { const result = await callOnce(() => this.request<{ csrf: string }>("/api/login", { password })); this.csrf = result.csrf; }
  async logout(): Promise<void> { await callOnce(() => this.request("/api/logout", {})); this.csrf = ""; }
  sources(): Promise<Source[]> { return callRead(() => this.request<Source[]>("/api/sources")); }
  status(alias: string): Promise<Snapshot> { return callOnce(() => this.request<Snapshot>("/api/status", { alias })); }
  scan(alias: string, resume: boolean): Promise<unknown> { return callOnce(() => this.request("/api/scan", { alias, resume })); }
  cancel(alias: string): Promise<unknown> { return callOnce(() => this.request("/api/cancel", { alias })); }
}
