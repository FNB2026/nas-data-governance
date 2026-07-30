// ErrorBoundary: catches React render errors and shows a recovery UI
// instead of a white screen. Does NOT catch errors in event handlers,
// async code, or SSR — those are handled by page-level try/catch.

import { Component, type ErrorInfo, type ReactNode } from "react";

interface ErrorBoundaryProps {
  children: ReactNode;
}

interface ErrorBoundaryState {
  hasError: boolean;
  error: Error | null;
}

export default class ErrorBoundary extends Component<ErrorBoundaryProps, ErrorBoundaryState> {
  constructor(props: ErrorBoundaryProps) {
    super(props);
    this.state = { hasError: false, error: null };
  }

  static getDerivedStateFromError(error: Error): ErrorBoundaryState {
    return { hasError: true, error };
  }

  componentDidCatch(error: Error, errorInfo: ErrorInfo): void {
    // Log to console for development debugging. Per AGENTS rule 6,
    // we do not send error details to any external service.
    console.error("[ErrorBoundary]", error, errorInfo.componentStack);
  }

  handleReload = (): void => {
    this.setState({ hasError: false, error: null });
    // Force a full reload to clear any stale state
    window.location.reload();
  };

  handleDismiss = (): void => {
    this.setState({ hasError: false, error: null });
  };

  render(): ReactNode {
    if (this.state.hasError) {
      const errorName = this.state.error?.name || "UnknownError";
      const errorMessage = this.state.error?.message || "发生未知错误";

      return (
        <div className="error-boundary">
          <div className="error-boundary-card">
            <div className="error-boundary-icon">⚠</div>
            <h2>页面渲染出错</h2>
            <p className="error-boundary-message">
              应用遇到渲染错误，但数据没有丢失。您可以重新加载页面或返回继续操作。
            </p>
            <details className="error-boundary-details">
              <summary>技术详情（{errorName}）</summary>
              <pre className="error-boundary-stack">{errorMessage}</pre>
            </details>
            <div className="error-boundary-actions">
              <button className="btn-sm" onClick={this.handleReload}>
                重新加载
              </button>
              <button className="btn-sm secondary" onClick={this.handleDismiss}>
                尝试恢复
              </button>
            </div>
          </div>
        </div>
      );
    }

    return this.props.children;
  }
}
