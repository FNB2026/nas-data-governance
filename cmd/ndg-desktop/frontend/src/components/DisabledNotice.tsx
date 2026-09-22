// DisabledNotice: panel-level explanation for mode-limited areas
// (read-only / recovery lock / disconnected data source), with an
// optional action. Pure presentation — reason text is supplied by caller.

export interface DisabledNoticeProps {
  reason: string;
  hint?: string;
  actionLabel?: string;
  onAction?: () => void;
}

export default function DisabledNotice({
  reason,
  hint,
  actionLabel,
  onAction,
}: DisabledNoticeProps) {
  return (
    <div className="disabled-notice" role="status">
      <span className="disabled-notice-icon" aria-hidden="true">
        ⚠
      </span>
      <div className="disabled-notice-body">
        <p className="disabled-notice-reason">{reason}</p>
        {hint && <p className="muted">{hint}</p>}
      </div>
      {actionLabel && onAction && (
        <button type="button" className="header-status-action" onClick={onAction}>
          {actionLabel}
        </button>
      )}
    </div>
  );
}