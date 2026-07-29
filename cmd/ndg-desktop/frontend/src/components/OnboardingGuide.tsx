// Onboarding guide: shown on the Sources page when no project is open.
// Provides step-by-step instructions for first-time users.

export default function OnboardingGuide() {
  return (
    <div className="onboarding-guide">
      <h3>快速开始</h3>
      <ol className="onboarding-steps">
        <li>
          <strong>选择数据库路径</strong>
          <p className="muted">输入一个 .db 文件路径。新项目会在读写打开时自动创建。</p>
        </li>
        <li>
          <strong>勾选「读写模式」</strong>
          <p className="muted">读写模式允许扫描和治理操作。仅查看已有数据可不勾选。</p>
        </li>
        <li>
          <strong>点击「读写打开」</strong>
          <p className="muted">打开后选择「扫描任务」，输入要扫描的目录路径。</p>
        </li>
        <li>
          <strong>开始扫描</strong>
          <p className="muted">扫描完成后，在重复结果页面查看重复文件和目录语境。</p>
        </li>
        <li>
          <strong>治理复核与执行</strong>
          <p className="muted">在治理复核页面审批计划，然后在执行中心安全执行操作。</p>
        </li>
      </ol>
      <div className="onboarding-shortcuts">
        <p className="muted">
          <span className="kbd">⌘1</span>～<span className="kbd">⌘7</span> 切换到可用页面 ·{" "}
          <span className="kbd">⌘O</span> 打开项目 ·{" "}
          <span className="kbd">⌘R</span> 刷新数据
        </p>
      </div>
    </div>
  );
}
