// GettingStartedRail: the lightweight post-create step list (UI-P7-C).
//
// Shown on the data-source page right after a project exists, so the
// first-run user can see the shape of the workflow without reading a
// tutorial. It is a guide, not a progress tracker: steps are marked
// complete only where the app has a real signal (a project is open, a
// scan job exists). No step is claimed complete on a guess.

import { useProject } from "../state/ProjectContext";
import { isRouteEnabled } from "../app/capability";
import type { AppRoute } from "../app/routes";

interface Step {
  index: number;
  label: string;
  hint: string;
  route: AppRoute;
}

const STEPS: Step[] = [
  { index: 1, label: "数据源", hint: "确认要治理的目录", route: "sources" },
  { index: 2, label: "扫描", hint: "建立文件指纹索引", route: "scan-jobs" },
  { index: 3, label: "查看重复结果", hint: "看每组文件所在语境", route: "duplicate-results" },
  { index: 4, label: "治理复核", hint: "确认保留与隔离", route: "governance-review" },
  { index: 5, label: "安全隔离", hint: "可恢复地执行处理", route: "execution-center" },
];

export interface GettingStartedRailProps {
  onNavigate: (route: AppRoute) => void;
}

export default function GettingStartedRail({ onNavigate }: GettingStartedRailProps) {
  const { jobs, scanProgress, capabilities, dismissGettingStarted } = useProject();

  // Only real, observable signals count as "done".
  const doneFlags: Record<number, boolean> = {
    1: capabilities.project_open,
    2: jobs.length > 0 || scanProgress !== null,
  };

  return (
    <section className="card card--full getting-started" aria-label="入门步骤">
      <div className="card-header-row">
        <h3>接下来</h3>
        <button
          type="button"
          className="link-button"
          onClick={dismissGettingStarted}
        >
          知道了
        </button>
      </div>
      <ol className="getting-started-list">
        {STEPS.map((step) => {
          const done = doneFlags[step.index] === true;
          const enabled = isRouteEnabled(step.route, capabilities);
          return (
            <li
              key={step.index}
              className={`getting-started-step ${done ? "getting-started-step--done" : ""}`}
            >
              <span className="getting-started-index" aria-hidden="true">
                {done ? "✓" : step.index}
              </span>
              <span className="getting-started-body">
                <span className="getting-started-label">{step.label}</span>
                <span className="getting-started-hint muted">{step.hint}</span>
              </span>
              <button
                type="button"
                className="btn-sm"
                disabled={!enabled}
                title={!enabled ? capabilities.disabled_reasons[step.route] : undefined}
                onClick={() => enabled && onNavigate(step.route)}
              >
                前往
              </button>
            </li>
          );
        })}
      </ol>
    </section>
  );
}
