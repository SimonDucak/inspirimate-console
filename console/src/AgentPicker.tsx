import type { Agent, ModelProvider } from './api'

export type AgentChoice = { agent: string; provider: string; model: string }
export const defaultChoice: AgentChoice = { agent: 'opencode', provider: '', model: '' }

export function preferredModel(provider: ModelProvider) {
  if (provider.default_model && provider.models.some((model) => model.id === provider.default_model)) {
    return provider.default_model
  }
  return provider.models[0]?.id || ''
}

// withPreferred fills an untouched OpenCode choice with the connection and model
// saved in Settings. An explicit agent, connection, or model is left as chosen.
export function withPreferred(choice: AgentChoice, providers: ModelProvider[]): AgentChoice {
  if (choice.agent !== 'opencode' || choice.provider || choice.model) return choice
  const saved = providers.find((provider) => provider.default)
  if (!saved) return choice
  return { agent: 'opencode', provider: saved.id, model: preferredModel(saved) }
}
export type BuildIntent = AgentChoice & { prompt: string }

export function AgentPicker({
  value,
  onChange,
  providers,
  agents,
  disabled = false,
}: {
  value: AgentChoice
  onChange: (value: AgentChoice) => void
  providers: ModelProvider[]
  agents: Agent[]
  disabled?: boolean
}) {
  const selected = providers.find((p) => p.id === value.provider)
  const modelValue = selected
    ? selected.models.some((model) => model.id === value.model)
      ? value.model
      : preferredModel(selected)
    : value.model
  return (
    <div className="agent-picker">
      <label>
        Coding agent
        <select
          disabled={disabled}
          data-testid="task-agent"
          value={value.agent}
          onChange={(e) =>
            onChange({
              agent: e.target.value,
              model: '',
              provider: e.target.value === 'claude-code' ? '' : value.provider,
            })
          }
        >
          <option value="opencode">OpenCode</option>
          <option value="pi">Pi</option>
          <option value="claude-code">Claude Code</option>
        </select>
      </label>
      {value.agent !== 'claude-code' && (
        <label>
          Connection
          <select
            disabled={disabled}
            aria-label="Model connection"
            data-testid="task-provider"
            value={value.provider}
            onChange={(e) => {
              const next = providers.find((provider) => provider.id === e.target.value)
              onChange({ ...value, provider: e.target.value, model: next ? preferredModel(next) : '' })
            }}
          >
            <option value="">{value.agent === 'pi' ? 'Select a connection…' : 'Built-in provider'}</option>
            {providers.map((p) => (
              <option value={p.id} key={p.id}>
                {p.name}
              </option>
            ))}
          </select>
        </label>
      )}
      <label className="full-width">
        Model
        <select
          disabled={disabled}
          data-testid="task-model"
          value={modelValue}
          onChange={(e) => onChange({ ...value, model: e.target.value })}
        >
          {selected ? (
            selected.models.map((model) => (
              <option key={model.id} value={model.id}>
                {model.id}
                {model.id === preferredModel(selected) ? ' (default)' : ''}
              </option>
            ))
          ) : (
            <option value="">Default model</option>
          )}
          {!value.provider &&
            value.agent === 'opencode' &&
            ['glm-5', 'kimi-k2.6', 'deepseek-v4-pro', 'MiniMax-M3.1-Flash', 'MiniMax-M3'].map((m) => (
              <option value={`opencode/${m}`} key={m}>
                {m}
              </option>
            ))}
          {value.agent === 'claude-code' &&
            ['sonnet', 'opus', 'haiku'].map((m) => <option key={m}>{m}</option>)}
        </select>
      </label>
      <p className="field-help full-width">
        {value.agent === 'pi'
          ? 'Pi uses your model connection.'
          : value.provider
            ? 'Your model endpoint. Your API key stays on the control plane.'
            : value.agent === 'opencode' &&
                !agents.some((a) => a.id === 'opencode' && a.status === 'connected')
              ? 'OpenCode can start with its built-in free tier.'
              : 'Uses the account connected in Settings.'}
      </p>
    </div>
  )
}

export function choiceBlocked(choice: AgentChoice, providers: ModelProvider[], agents: Agent[]) {
  if (choice.provider) return !providers.some((p) => p.id === choice.provider)
  if (choice.agent === 'pi') return true
  return (
    choice.agent === 'claude-code' && !agents.some((a) => a.id === 'claude-code' && a.status === 'connected')
  )
}
