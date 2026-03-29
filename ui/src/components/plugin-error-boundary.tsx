import { Component } from 'react'
import type { ErrorInfo, ReactNode } from 'react'

interface Props {
  pluginName: string
  children: ReactNode
}

interface State {
  hasError: boolean
  error: Error | null
}

export class PluginErrorBoundary extends Component<Props, State> {
  state: State = { hasError: false, error: null }

  static getDerivedStateFromError(error: Error): State {
    return { hasError: true, error }
  }

  componentDidCatch(error: Error, info: ErrorInfo) {
    console.error(`Plugin "${this.props.pluginName}" crashed:`, error, info)
  }

  render() {
    if (this.state.hasError) {
      return (
        <div className="rounded-lg border border-destructive/50 bg-destructive/10 p-6 text-center">
          <h3 className="text-lg font-semibold text-destructive">
            Plugin Error: {this.props.pluginName}
          </h3>
          <p className="mt-2 text-sm text-muted-foreground">
            {this.state.error?.message || 'An unexpected error occurred'}
          </p>
          <button
            className="mt-4 rounded-md bg-primary px-4 py-2 text-sm text-primary-foreground hover:bg-primary/90"
            onClick={() => this.setState({ hasError: false, error: null })}
          >
            Retry
          </button>
        </div>
      )
    }

    return this.props.children
  }
}
