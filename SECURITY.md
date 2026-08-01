# Security Policy

## Scope

NDG 数据治理工作台 scans and may eventually modify large local file collections. Security reports are especially important when they involve:

- source-root boundary bypasses;
- symbolic-link, mount-point, path traversal, or race-condition issues;
- unintended deletion, overwrite, quarantine escape, or incomplete rollback;
- leakage of real paths, filenames, file contents, credentials, or private reports;
- unsafe execution-plan approval or stale-check bypasses;
- dependency or workflow supply-chain risks.

## Reporting

Do not open a public issue containing real NAS paths, filenames, database contents, logs, credentials, or sample files.

Use GitHub Security Advisories and the repository's **Report a vulnerability** entry when available. Otherwise, contact the repository owner through a private channel and provide only sanitized reproduction details.

A useful report should include:

1. the affected command or package;
2. the expected safety boundary;
3. a minimal reproduction using temporary or synthetic files;
4. the observed impact;
5. the relevant commit or version;
6. any proposed mitigation.

## Handling Sensitive Evidence

- Replace source paths and filenames with synthetic placeholders.
- Do not upload a production SQLite database, JSONL index, audit log, or private diagnostic report.
- Reproduce filesystem issues inside an isolated temporary directory whenever possible.
- Do not test destructive behavior against real NAS data.

## Supported Versions

Security fixes are applied to the current `main` branch and the latest tagged release. Older snapshots may not receive backports.
