import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { AppView } from './AppView'
import { api } from './api'

vi.mock('./api', () => ({ api: {
  getApp: vi.fn().mockResolvedValue({ id: 'app', name: 'Model picker', created_at: '' }),
  appManifest: vi.fn().mockResolvedValue({ present: false }),
  runtimeInspect: vi.fn().mockResolvedValue({ suggestions: [] }),
  getAgents: vi.fn().mockResolvedValue([]),
  listModelProviders: vi.fn().mockResolvedValue([]),
} }))

beforeEach(() => {
  vi.mocked(api.listModelProviders).mockReset()
  vi.mocked(api.listModelProviders).mockResolvedValue([])
})
afterEach(cleanup)

describe('OpenCode model picker', () => {
  it('offers Flash before M3, preserves other choices, and leaves the saved default in control', async () => {
    const noop = () => {}
    render(<AppView appId="app" onError={noop} toast={noop} goApps={noop} goSettings={noop} />)
    const picker = await screen.findByTestId('task-model') as HTMLSelectElement
    expect(picker.value).toBe('')
    expect(Array.from(picker.options).map((o) => o.value)).toEqual([
      '', 'opencode/glm-5', 'opencode/kimi-k2.6', 'opencode/deepseek-v4-pro',
      'opencode/MiniMax-M3.1-Flash', 'opencode/MiniMax-M3',
    ])
    fireEvent.change(picker, { target: { value: 'opencode/MiniMax-M3.1-Flash' } })
    expect(picker.value).toBe('opencode/MiniMax-M3.1-Flash')
    fireEvent.change(screen.getByTestId('task-agent'), { target: { value: 'claude-code' } })
    expect(Array.from(picker.options).map((o) => o.value)).toEqual(['', 'sonnet', 'opus', 'haiku'])
  })

  it('opens with the default model connection and model', async () => {
    vi.mocked(api.listModelProviders).mockResolvedValue([{
      id: 'openrouter', name: 'OpenRouter', protocol: 'openai-completions',
      base_url: 'https://openrouter.ai/api/v1', has_api_key: true, default: true,
      default_model: 'openai/gpt-4.1',
      models: [
        { id: 'openai/gpt-4.1-mini', context_window: 128000, max_tokens: 4096 },
        { id: 'openai/gpt-4.1', context_window: 128000, max_tokens: 4096 },
      ],
    }])
    const noop = () => {}
    render(<AppView appId="app" onError={noop} toast={noop} goApps={noop} goSettings={noop} />)
    await waitFor(() => expect((screen.getByTestId('task-provider') as HTMLSelectElement).value).toBe('openrouter'))
    expect((screen.getByTestId('task-model') as HTMLSelectElement).value).toBe('openai/gpt-4.1')
  })
})
