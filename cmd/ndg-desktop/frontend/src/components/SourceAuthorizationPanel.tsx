export interface SourceAuthorizationPanelProps {
  busy: boolean;
  isReadWrite: boolean;
  onAuthorize: () => void;
  error?: string | null;
  notice?: string | null;
}

export default function SourceAuthorizationPanel({
  busy,
  isReadWrite,
  onAuthorize,
  error,
  notice,
}: SourceAuthorizationPanelProps) {
  return (
    <section className="card card--full" aria-labelledby="source-authorization-title">
      <h2 id="source-authorization-title">源目录授权</h2>
      <p className="muted">
        只会为你在系统目录选择器中选中的目录保存只读授权。重新授权不会自动扫描、Resume 或解除恢复锁；请先核对 SMB 服务器与共享身份。
        新位置会登记为独立存储，不会自动迁移旧检查点。
      </p>
      <div className="button-row">
        <button
          className="btn-sm secondary"
          type="button"
          disabled={busy || !isReadWrite}
          onClick={onAuthorize}
        >
          {busy ? "正在登记授权…" : "选择并授权扫描目录"}
        </button>
        {!isReadWrite && <span className="muted">请以读写模式打开项目后登记授权</span>}
      </div>
      {error && <p className="error" role="alert">{error}</p>}
      {notice && <p className="muted" role="status">{notice}</p>}
    </section>
  );
}
