import { Component, type ReactNode } from "react";

type State = { error: Error | null };

export class ErrorBoundary extends Component<{ children: ReactNode }, State> {
  state: State = { error: null };

  static getDerivedStateFromError(error: Error): State {
    return { error };
  }

  componentDidCatch(error: Error, info: { componentStack?: string | null }) {
    console.error("[ErrorBoundary]", error, info);
  }

  render() {
    if (this.state.error) {
      return (
        <div
          style={{
            position: "fixed",
            inset: 0,
            display: "grid",
            placeItems: "center",
            background: "#0f1219",
            color: "#fff",
            fontFamily: "ui-monospace, Menlo, monospace",
            fontSize: 12,
            padding: 24,
          }}
        >
          <div style={{ maxWidth: 720 }}>
            <div style={{ color: "#ff6b6b", fontSize: 14, marginBottom: 12 }}>
              NovaNAS console failed to start
            </div>
            <div style={{ marginBottom: 8 }}>{this.state.error.message}</div>
            <pre
              style={{
                whiteSpace: "pre-wrap",
                wordBreak: "break-all",
                fontSize: 10,
                opacity: 0.6,
              }}
            >
              {this.state.error.stack}
            </pre>
          </div>
        </div>
      );
    }
    return this.props.children;
  }
}
