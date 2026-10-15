import { useCallback, useEffect, useRef, useState } from "react";
import DisabledNotice from "../components/DisabledNotice";
import LoadingState from "../components/LoadingState";
import { HttpAdapter, WebRequestError, type Snapshot, type Source } from "./http";

const defaultAPI = new HttpAdapter();
const states: Record<string, string> = { IDLE: "尚未扫描", QUEUED: "等待扫描", RUNNING: "扫描中", CANCEL_REQUESTED: "正在停止", COMPLETED: "扫描完成", CANCELLED: "扫描已停止", FAILED: "扫描失败", PAUSED_AUTHORIZATION: "授权已失效", PAUSED_NETWORK: "连接中断" };
const isRunning = (s?: Snapshot) => !!s && ["RUNNING", "QUEUED", "CANCEL_REQUESTED"].includes(s.state);
const message = (e: unknown) => e instanceof WebRequestError ? e.message : "请求未完成，请检查服务状态后重试。";

export function WebApp({ api = defaultAPI }: { api?: HttpAdapter }) {
  const [password, setPassword] = useState("");
  const [sources, setSources] = useState<Source[]>([]);
  const [alias, setAlias] = useState("");
  const [result, setResult] = useState<{ alias: string; value: Snapshot }>();
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  const [busy, setBusy] = useState(false);
  const [checkingSession, setCheckingSession] = useState(true);
  const [signedIn, setSignedIn] = useState(false);
  const [revision, setRevision] = useState(0);
  const actionPending = useRef(false);
  const status = signedIn && result?.alias === alias ? result.value : undefined;
  const forgetSession = useCallback(() => {
    api.reset(); setSignedIn(false); setSources([]); setAlias(""); setResult(undefined); setPassword(""); setNotice("");
  }, [api]);
  useEffect(() => {
    let disposed = false;
    void (async () => {
      try {
        await api.session(); const list = await api.sources();
        if (!disposed) { setSources(list); setAlias(list[0]?.alias ?? ""); setSignedIn(true); }
      } catch (e) {
        if (!disposed && !(e instanceof WebRequestError && e.status === 401)) setError(message(e));
      } finally { if (!disposed) setCheckingSession(false); }
    })();
    return () => { disposed = true; };
  }, [api]);
  useEffect(() => {
    if (!signedIn || !alias) return;
    let disposed = false, pending = false;
    let timer: ReturnType<typeof setTimeout> | undefined;
    const controller = new AbortController();
    const refresh = async () => {
      if (disposed || pending || document.hidden) return;
      clearTimeout(timer); pending = true;
      try {
        const next = await api.status(alias, controller.signal);
        if (!disposed) {
          setResult({ alias, value: next });
          if (isRunning(next)) timer = setTimeout(() => void refresh(), 2000);
        }
      } catch (e) {
        if (!disposed) { setResult(undefined); if (e instanceof WebRequestError && e.status === 401) forgetSession(); setError(message(e)); }
      } finally { pending = false; }
    };
    const visible = () => { if (!document.hidden) void refresh(); else clearTimeout(timer); };
    void refresh(); document.addEventListener("visibilitychange", visible);
    return () => { disposed = true; clearTimeout(timer); controller.abort(); document.removeEventListener("visibilitychange", visible); };
  }, [api, signedIn, alias, revision, forgetSession]);
  async function action(fn: () => Promise<unknown>) {
    if (actionPending.current) return;
    actionPending.current = true; setBusy(true); setError(""); setNotice("");
    try { await fn(); }
    catch (e) { if (e instanceof WebRequestError && e.status === 401) forgetSession(); setError(message(e)); }
    finally { actionPending.current = false; setBusy(false); }
  }
  return <main className="page web-page">
    <header><h1>NDG · 只读浏览</h1><p className="muted">只读 Web 开发候选 · Linux 本地来源</p></header>
    <DisabledNotice reason="当前版本仅扫描和浏览，隔离、恢复与永久删除尚未开放。" hint="内容相同不代表可以删除。这里只展示最多 200 组聚合结果，不显示文件名、路径和完整目录语境。" />
    {checkingSession ? <LoadingState label="正在检查登录状态…" /> : !signedIn ?
      <form className="web-card web-login" onSubmit={e => {
        e.preventDefault(); const credential = password; setPassword("");
        void action(async () => { await api.login(credential); const list = await api.sources(); setSources(list); setAlias(list[0]?.alias ?? ""); setSignedIn(true); });
      }}>
        <h2>登录 NDG</h2><p className="muted">使用安装时设置的 NDG 登录凭据。</p>
        <label htmlFor="ndg-credential">登录凭据</label>
        <input id="ndg-credential" type="password" autoComplete="current-password" required disabled={busy} value={password} onChange={e => setPassword(e.target.value)} />
        <button type="submit" disabled={busy || !password}>登录</button>
      </form> : <>
        <section className="web-card" aria-labelledby="source-heading">
          <h2 id="source-heading">扫描来源</h2><label htmlFor="source">已授权来源</label>
          <select id="source" value={alias} disabled={busy || !sources.length} onChange={e => { setResult(undefined); setNotice(""); setError(""); setAlias(e.target.value); }}>
            {sources.map(s => <option key={s.alias} value={s.alias}>{s.alias}</option>)}
          </select>
          {!sources.length && <p>尚未配置来源，请在安装配置中授权人工测试目录。</p>}
          <div className="web-actions">
            <button disabled={busy || !alias || isRunning(status)} onClick={() => void action(async () => { await api.scan(alias, false); setNotice("扫描请求已接受，请等待实际扫描结果。"); setRevision(v => v + 1); })}>开始扫描</button>
            <button disabled={busy || !alias || isRunning(status)} onClick={() => void action(async () => { await api.scan(alias, true); setNotice("续扫请求已接受，请等待实际扫描结果。"); setRevision(v => v + 1); })}>从断点继续</button>
            <button disabled={busy || !alias || !isRunning(status) || status?.state === "CANCEL_REQUESTED"} onClick={() => void action(async () => { await api.cancel(alias); setNotice("停止请求已接受，请等待扫描实际停止。"); setRevision(v => v + 1); })}>停止扫描</button>
            <button disabled={busy || !alias} onClick={() => { setError(""); setRevision(v => v + 1); }}>刷新状态</button>
            <button disabled={busy} onClick={() => void action(async () => { await api.logout(); forgetSession(); })}>退出登录</button>
          </div>
          <p className="muted">后台页面暂停轮询；扫描停止后不持续刷新会话。请求失败不会自动重复提交。</p>
        </section>
        <section className="web-card" aria-labelledby="result-heading">
          <h2 id="result-heading">扫描结果</h2>
          {!status ? <p role="status">{error ? "暂时无法读取所选来源状态，请刷新重试。" : "正在读取所选来源的状态…"}</p> : <>
            <p role="status">{states[status.state] ?? "未知扫描状态"}</p>
            <dl className="web-summary">{[["发现文件", status.discovered], ["已处理", status.processed], ["失败", status.failed], ["有效记录", status.active], ["文件缺失", status.missing], ["不可访问", status.unavailable], ["完整校验", status.full]].map(([label, value]) => <div key={label}><dt>{label}</dt><dd>{value}</dd></div>)}</dl>
            <h3>重复组 · {status.groups.length} 组</h3>
            {status.groups.length ? <div className="web-table"><table><caption>聚合结果，上限 200 组；分组编号仅用于本次结果展示。</caption><thead><tr><th scope="col">分组</th><th scope="col">副本数</th><th scope="col">大小（字节）</th></tr></thead><tbody>{status.groups.map((g, i) => <tr key={g.id}><th scope="row">组 {i + 1}</th><td>{g.copies}</td><td>{g.bytes.toLocaleString("zh-CN")}</td></tr>)}</tbody></table></div> : <p>当前没有可展示的重复组。扫描尚未完成时，这不代表来源中没有重复文件。</p>}
          </>}
        </section>
      </>}
    {busy && <LoadingState label="正在等待服务响应…" />}
    {notice && <p role="status">{notice}</p>}
    {error && <p className="web-error" role="alert">{error}</p>}
  </main>;
}
