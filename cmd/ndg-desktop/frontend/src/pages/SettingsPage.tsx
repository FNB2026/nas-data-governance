// Settings page: reorganized into five top-level groups (UI-P7-A).
//
//   常规  General   — recent projects, first-run guide, shortcuts
//   扫描  Scan      — scan defaults (full scan / worker count)
//   隐私  Privacy   — path masking, offline guarantees
//   安全  Security  — project mode, recovery lock, execution capability
//   关于  About     — product identity, version, external links (UI-P7-B)
//
// No new settings were introduced: every control below already existed
// and was only regrouped. About is informational and performs no online
// update check, no login, no telemetry and no implicit network access.

import type { ReactNode } from "react";
import { useProject } from "../state/ProjectContext";
import { maskPath } from "../state/settings";
import { PRODUCT, PRODUCT_LINKS } from "../app/productInfo";
import { openExternal } from "../lib/external";
import LoadingState from "../components/LoadingState";
import ErrorState from "../components/ErrorState";
import EmptyState from "../components/EmptyState";

const MODE_LABELS: Record<string, string> = {
  closed: "未打开",
  read_only: "只读",
  read_write: "读写",
};

const SHORTCUTS: Array<{ keys: string; action: string }> = [
  { keys: "⌘/Ctrl + 1–7", action: "切换左侧导航页面" },
  { keys: "⌘/Ctrl + O", action: "定位到项目数据库路径输入" },
  { keys: "⌘/Ctrl + R", action: "刷新当前项目数据" },
];

const SECTIONS = [
  { id: "general", label: "常规" },
  { id: "scan", label: "扫描" },
  { id: "privacy", label: "隐私" },
  { id: "security", label: "安全" },
  { id: "about", label: "关于" },
];

function SettingsSection({
  id,
  title,
  description,
  children,
}: {
  id: string;
  title: string;
  description?: string;
  children: ReactNode;
}) {
  return (
    <section
      className="card settings-section"
      id={`settings-${id}`}
      aria-labelledby={`settings-${id}-title`}
    >
      <div className="card-header-row">
        <h3 id={`settings-${id}-title`}>{title}</h3>
      </div>
      {description && <p className="muted settings-section-desc">{description}</p>}
      {children}
    </section>
  );
}

function OffBadge({ label }: { label: string }) {
  return (
    <span className="state-badge state-badge--cancelled" aria-label={`${label}：已关闭`}>
      已关闭
    </span>
  );
}

export default function SettingsPage() {
  const {
    version,
    error,
    capabilities,
    pathPrivacyMode,
    togglePathPrivacy,
    defaultFullScan,
    defaultWorkers,
    setDefaultFullScan,
    setDefaultWorkers,
    recentProjects,
    displayPath,
    restartOnboarding,
  } = useProject();

  const modeLabel = MODE_LABELS[capabilities.project_mode] || capabilities.project_mode;

  return (
    <div className="page page--settings">
      <div className="page-header">
        <h2>设置</h2>
        <p className="muted">应用配置、隐私与安全状态</p>
      </div>

      <nav className="settings-toc" aria-label="设置分组">
        {SECTIONS.map((section) => (
          <a key={section.id} className="settings-toc-item" href={`#settings-${section.id}`}>
            {section.label}
          </a>
        ))}
      </nav>

      {/* ---- 常规 ---- */}
      <SettingsSection id="general" title="常规">
        <h4 className="settings-subhead">最近项目</h4>
        {recentProjects.length > 0 ? (
          <ul className="settings-recent-list">
            {recentProjects.map((entry) => (
              <li key={entry.path}>
                <span className="settings-recent-name">{entry.name}</span>
                <span className="settings-recent-path mono muted" title={displayPath(entry.path)}>
                  {displayPath(entry.path)}
                </span>
              </li>
            ))}
          </ul>
        ) : (
          <EmptyState title="暂无最近项目" hint="创建或打开一个项目后会出现在这里。" />
        )}

        <h4 className="settings-subhead">首次启动引导</h4>
        <div className="settings-action-row">
          <button type="button" className="btn-sm" onClick={restartOnboarding}>
            重新显示首次启动引导
          </button>
          <p className="muted settings-toggle-hint">
            只重新展示引导说明，不改变任何项目、扫描或执行行为。
          </p>
        </div>

        <h4 className="settings-subhead">键盘快捷键</h4>
        <table className="data-table">
          <tbody>
            {SHORTCUTS.map((item) => (
              <tr key={item.keys}>
                <td className="mono">{item.keys}</td>
                <td>{item.action}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </SettingsSection>

      {/* ---- 扫描 ---- */}
      <SettingsSection
        id="scan"
        title="扫描"
        description="以下默认值作用于新建扫描，不改变已有任务。"
      >
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
      </SettingsSection>

      {/* ---- 隐私 ---- */}
      <SettingsSection
        id="privacy"
        title="隐私"
        description="本地优先：除你主动点击的外部链接外，NDG 不进行任何联网操作。"
      >
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

        <table className="data-table settings-guarantee-table">
          <tbody>
            <tr>
              <td>路径脱敏</td>
              <td>{pathPrivacyMode ? "已开启" : "未开启"}</td>
            </tr>
            <tr>
              <td>外部 AI</td>
              <td><OffBadge label="外部 AI" /></td>
            </tr>
            <tr>
              <td>遥测</td>
              <td><OffBadge label="遥测" /></td>
            </tr>
            <tr>
              <td>云上传</td>
              <td><OffBadge label="云上传" /></td>
            </tr>
          </tbody>
        </table>
        <p className="muted diag-read-only-hint">
          NDG 不会在未经明确同意的情况下进行任何联网操作。
        </p>
      </SettingsSection>

      {/* ---- 安全 ---- */}
      <SettingsSection
        id="security"
        title="安全"
        description="当前项目的能力边界。只读或恢复锁生效时，写操作会在入口处被阻止。"
      >
        <table className="data-table">
          <tbody>
            <tr>
              <td>当前项目模式</td>
              <td>{modeLabel}</td>
            </tr>
            <tr>
              <td>可执行扫描</td>
              <td>{capabilities.can_scan ? "可用" : "受限"}</td>
            </tr>
            <tr>
              <td>可执行治理（隔离）</td>
              <td>{capabilities.can_execute_quarantine ? "可用" : "受限"}</td>
            </tr>
            <tr>
              <td>可执行永久清理</td>
              <td>{capabilities.can_execute_purge ? "可用" : "受限"}</td>
            </tr>
            <tr>
              <td>Recovery Lock</td>
              <td>
                {capabilities.recovery_lock_active ? "激活" : "无"}
              </td>
            </tr>
          </tbody>
        </table>
        <p className="muted diag-read-only-hint">
          本地优先 / 无隐式联网：所有判断与执行都在本机完成。
        </p>
      </SettingsSection>

      {/* ---- 关于 ---- */}
      <SettingsSection id="about" title="关于">
        <div className="about-identity">
          <span className="about-mark" aria-hidden="true">N</span>
          <div className="about-identity-text">
            <p className="about-name">{PRODUCT.name}</p>
            <p className="about-tagline muted">{PRODUCT.tagline}</p>
          </div>
        </div>

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
        ) : error ? (
          <ErrorState message={error} />
        ) : (
          <LoadingState label="正在加载版本信息…" />
        )}

        <h4 className="settings-subhead">链接</h4>
        <ul className="about-links">
          {PRODUCT_LINKS.map((link) => (
            <li key={link.id}>
              <button
                type="button"
                className="about-link"
                title={link.url}
                onClick={() => openExternal(link.url)}
              >
                <span className="about-link-label">{link.label}</span>
                <span className="about-link-hint muted">{link.hint}</span>
              </button>
            </li>
          ))}
        </ul>
        <p className="muted diag-read-only-hint">
          Beta 阶段不提供在线更新检测、账号登录或自动联网；以上链接仅在你点击时通过系统浏览器打开。
        </p>
      </SettingsSection>
    </div>
  );
}
