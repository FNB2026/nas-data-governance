// Product identity: single source of truth for the About section.
//
// Privacy boundary (Beta): the About section is informational only. It
// performs no online update check, no account login, no telemetry and no
// implicit network access. Every outbound link below is opened only when
// the user explicitly clicks it, through the OS browser.

const REPO_URL = "https://github.com/FNB2026/nas-data-governance";

/** Path inside the repository; the full blob URL is derived below. */
const GUIDE_PATH = "docs/user-guide/NDG-用户指南.md";

export interface ProductLink {
  /** Stable identifier used by tests and analytics-free keys. */
  id: string;
  label: string;
  /** Short description shown next to the link. */
  hint: string;
  url: string;
  /** True for links that leave the app and must be user-triggered. */
  external: boolean;
}

export const PRODUCT = {
  /** Canonical product identity shown on the About page. */
  name: "NDG — NAS Data Governance",
  /** Localized application name (matches wails.json productName). */
  displayName: "NDG 数据治理工作台",
  tagline: "本地优先的 NAS 数据治理工作台",
  repoUrl: REPO_URL,
} as const;

export const PRODUCT_LINKS: ProductLink[] = [
  {
    id: "github",
    label: "GitHub",
    hint: "源代码仓库",
    url: REPO_URL,
    external: true,
  },
  {
    id: "guide",
    label: "用户指南",
    hint: "安装、扫描、复核与恢复",
    url: `${REPO_URL}/blob/main/${GUIDE_PATH}`,
    external: true,
  },
  {
    id: "license",
    label: "License",
    hint: "Apache License 2.0",
    url: `${REPO_URL}/blob/main/LICENSE`,
    external: true,
  },
  {
    id: "privacy",
    label: "隐私说明",
    hint: "数据不上传与本地优先声明",
    url: `${REPO_URL}/blob/main/${GUIDE_PATH}#9-隐私安全与数据不上传声明`,
    external: true,
  },
  {
    id: "issues",
    label: "问题反馈",
    hint: "使用合成数据提交 Issue",
    url: `${REPO_URL}/issues/new/choose`,
    external: true,
  },
];
