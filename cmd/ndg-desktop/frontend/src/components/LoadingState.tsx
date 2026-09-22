// LoadingState: standard in-progress indicator (spinner + label).

export interface LoadingStateProps {
  label?: string;
}

export default function LoadingState({ label = "加载中…" }: LoadingStateProps) {
  return (
    <div className="state-loading" role="status" aria-busy="true">
      <span className="state-spinner" aria-hidden="true" />
      <span>{label}</span>
    </div>
  );
}