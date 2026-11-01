import { afterEach, beforeAll, describe, expect, it, vi } from 'vitest'
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { AgentChat } from './AgentChat'
import { api, TaskSummary } from './api'

vi.mock('./api', () => ({
  api: {
    getAgents: vi.fn().mockResolvedValue([]),
    listModelProviders: vi.fn().mockResolvedValue([]),
    listTasks: vi.fn().mockResolvedValue([]),
    taskEventsURL: vi.fn().mockReturnValue('/events'),
  },
}))

afterEach(cleanup)

beforeAll(() => {
  HTMLDialogElement.prototype.showModal = function showModal(this: HTMLDialogElement) {
    this.setAttribute('open', '')
  }
  HTMLDialogElement.prototype.close = function close(this: HTMLDialogElement) {
    this.removeAttribute('open')
  }
})

const sandbox = { id: 'sb', status: 'running', created_at: '', ports: [] }
const noop = () => {}

function task(partial: Partial<TaskSummary> & Pick<TaskSummary, 'id' | 'status'>): TaskSummary {
  return { can_revert: false, ...partial }
}

describe('finished task with an agent issue', () => {
  it('shows finished and keeps the issue behind a button', async () => {
    const issue = 'agent error: claude exited: exit status 1'
    vi.mocked(api.listTasks).mockResolvedValue([
      task({
        id: 'failed-turn',
        prompt: 'Add a save button',
        status: 'failed',
        agent_message: issue,
        error_message: issue,
      }),
      task({
        id: 'ok-turn',
        prompt: 'Rename the title',
        status: 'succeeded',
        agent_message: 'Renamed the title.',
      }),
    ])

    render(<AgentChat sb={sandbox} onError={noop} toast={noop} refresh={noop} goSettings={noop} />)

    await waitFor(() => expect(screen.getByText('finished')).toBeTruthy())
    expect(screen.queryByText('failed')).toBeNull()
    expect(screen.getByText(/may still be implemented correctly/)).toBeTruthy()
    expect(screen.queryByText(issue)).toBeNull()
    expect(screen.getByText('The agent finished this turn.')).toBeTruthy()
    expect(screen.getByText('succeeded')).toBeTruthy()
    expect(screen.getByText('Renamed the title.')).toBeTruthy()

    fireEvent.click(screen.getByRole('button', { name: 'View issue' }))
    expect(screen.getByText(issue)).toBeTruthy()
  })
})
