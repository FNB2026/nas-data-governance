// Settings page: version info, scan defaults, privacy, developer info.

import { useProject } from "../state/ProjectContext";
import { maskPath } from "../state/settings";

export default function SettingsPage() {
  const {
    version,
    capabilities,
    pathPrivacyMode,
    togglePathPrivacy,
    defaultFullScan,
    defaultWorkers,
    setDefaultFullScan,
    setDefaultWorkers,
  } = useProject();

  return (
    <div className="page page--settings">
      <div className="page-header">
        <h2>设置</h2>
        <p className="muted">应用配置与开发者信息</p>
      </div>

      <div className="card">
        <h3>版本信息</h3>
        {version ? (
          <table className="data-table">
            <tbody>
              <tr>
                <td>版本</td>
                <td className="mono">{version.version}</td>
              </tr>
              <tr>
                <td>提交</td>
                <td className="mono">{version.commit}</td>
              </tr>
              <tr>
                <td>构建时间</td>
                <td className="mono">{version.build_time}</td>
              </tr>
              <tr>
                <td>发布通道</td>
                <td className="mono">{version.channel || "—"}</td>
              </tr>
            </tbody>
          </table>
        ) : (
          <p className="muted">加载中...</p>
        )}
      </div>

      <div className="card">
        <h3>隐私与路径脱敏</h3>
        <div className="settings-toggle-row">
          <label className="mode-toggle">
            <input
              type="checkbox"
              checked={pathPrivacyMode}
              onChange={togglePathPrivacy}
            />
            路径脱敏模式
          </label>
          <p className="muted settings-toggle-hint">
            开启后，界面中的路径和文件名将被部分遮蔽（如 <code>…/[hidden]/r***t.pdf</code>），
            适合截图分享或演示场景。
          </p>
          <div className="settings-preview">
            <span className="muted">预览：</span>
            <code className="settings-preview-path">
              {pathPrivacyMode
                ? maskPath("/data/archive/Documents/Work/Projects/report.pdf")
                : "/data/archive/Documents/Work/Projects/report.pdf"}
            </code>
          </div>
        </div>
      </div>

      <div className="card">
        <h3>隐私与联网</h3>
        <table className="data-table">
          <tbody>
            <tr>
              <td>外部 AI</td>
              <td><span className="state-badge state-badge--cancelled">已关闭</span></td>
            </tr>
            <tr>
              <td>遥测</td>
              <td><span className="state-badge state-badge--cancelled">已关闭</span></td>
            </tr>
            <tr>
              <td>云上传</td>
              <td><span className="state-badge state-badge--cancelled">已关闭</span></td>
            </tr>
          </tbody>
        </table>
        <p className="muted diag-read-only-hint">
          NDG 不会在未经明确同意的情况下进行任何联网操作。
        </p>
      </div>

      <div className="card">
        <h3>扫描参数默认值</h3>
        <div className="settings-toggle-row">
          <label className="mode-toggle">
            <input
              type="checkbox"
              checked={defaultFullScan}
              onChange={(e) => setDefaultFullScan(e.target.checked)}
            />
            默认完整扫描（完整哈希校验）
          </label>
          <p className="muted settings-toggle-hint">
            开启后，新建扫描将默认使用完整哈希（SHA-256）而非快速哈希。
            完整哈希更精确但速度较慢，适用于最终归档前的全量校验。
          </p>
          <div className="settings-scan-workers">
            <label className="mode-toggle">
              默认并发工作线程数
              <input
                type="number"
                min={1}
                max={64}
                value={defaultWorkers}
                onChange={(e) => setDefaultWorkers(e.target.value)}
                placeholder="自动"
                className="settings-workers-input"
              />
            </label>
            <p className="muted settings-toggle-hint">
              留空则由后端自动检测最优并发数（通常等于 CPU 核心数）。
              手动指定可控制资源占用，范围 1–64。
            </p>
          </div>
        </div>
      </div>

      <div className="card">
        <h3>能力状态</h3>
        <table className="data-table">
          <tbody>
            <tr>
              <td>项目模式</td>
              <td>{capabilities.project_mode}</td>
            </tr>
            <tr>
              <td>可扫描</td>
              <td>{capabilities.can_scan ? "是" : "否"}</td>
            </tr>
            <tr>
              <td>可编辑复核</td>
              <td>{capabilities.can_edit_reviews ? "是" : "否"}</td>
            </tr>
            <tr>
              <td>可执行隔离</td>
              <td>{capabilities.can_execute_quarantine ? "是" : "待接线"}</td>
            </tr>
            <tr>
              <td>恢复锁</td>
              <td>{capabilities.recovery_lock_active ? "激活" : "无"}</td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>
  );
}
