import { Component } from 'react';
import type { ErrorInfo, ReactNode } from 'react';

interface ErrorBoundaryProps {
  children: ReactNode;
}

interface ErrorBoundaryState {
  error: Error | null;
}

// ErrorBoundary keeps a single render error from blanking the whole IDE. It
// shows a recoverable message and a reload action instead of an empty screen.
export class ErrorBoundary extends Component<ErrorBoundaryProps, ErrorBoundaryState> {
  state: ErrorBoundaryState = { error: null };

  static getDerivedStateFromError(error: Error): ErrorBoundaryState {
    return { error };
  }

  componentDidCatch(error: Error, info: ErrorInfo): void {
    // eslint-disable-next-line no-console
    console.error('UI render error', error, info.componentStack);
  }

  private reset = () => {
    this.setState({ error: null });
  };

  render(): ReactNode {
    if (this.state.error) {
      return (
        <div className="error-boundary" role="alert">
          <h1>Что-то пошло не так</h1>
          <p>Интерфейс упал при отрисовке. Попробуйте сбросить экран или перезагрузить страницу.</p>
          <pre className="error-boundary__detail">{this.state.error.message}</pre>
          <div className="error-boundary__actions">
            <button type="button" onClick={this.reset}>
              Сбросить экран
            </button>
            <button type="button" onClick={() => window.location.reload()}>
              Перезагрузить
            </button>
          </div>
        </div>
      );
    }
    return this.props.children;
  }
}
