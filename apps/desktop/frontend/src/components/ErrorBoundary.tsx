import { Component, type ReactNode } from "react";
import { Mark } from "@/components/Brand";

type State = { error: Error | null };

/** Last line of defence: show the error instead of a black window. */
export class ErrorBoundary extends Component<{ children: ReactNode }, State> {
  state: State = { error: null };
  static getDerivedStateFromError(error: Error): State { return { error }; }
  componentDidCatch(error: Error) { console.error("[rolle] render error", error); }
  render() {
    if (!this.state.error) return this.props.children;
    return (
      <main className="flex h-full flex-col items-center justify-center gap-4 bg-background p-8 text-foreground">
        <Mark className="size-12" />
        <h1 className="text-lg font-medium">Something broke in the interface</h1>
        <pre className="max-w-xl overflow-auto rounded-md bg-card p-3 font-mono text-xs text-muted-foreground">{this.state.error.message}</pre>
        <button className="text-sm underline" onClick={() => this.setState({ error: null })}>Try again</button>
      </main>
    );
  }
}
