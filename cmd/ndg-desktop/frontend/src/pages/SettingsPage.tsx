// Settings page: version info, AI/telemetry status, developer info.
// Settings persistence not yet implemented — values shown are defaults.

import { useProject } from "../state/ProjectContext";

export default function SettingsPage() {
  const { version, capabilities } = useProject();

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
            </tbody>
          </table>
        ) : (
          <p className="muted">加载中...</p>
        )}
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
        <p className="muted diag-read-only-hint">
          设置持久化、扫描参数默认值、隐私路径配置将在后续阶段接入。
        </p>
      </div>
    </div>
  );
}
