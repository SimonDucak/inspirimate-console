import { afterEach, beforeAll, describe, expect, it, vi } from 'vitest'
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { AppView } from './AppView'
import { api } from './api'

vi.mock('./api', () => ({
  api: {
    getApp: vi.fn().mockResolvedValue({
      id: 'app',
      name: 'Timer',
      created_at: '',
      task_timeout_s: 1800,
    }),
    getAgents: vi.fn().mockResolvedValue([]),
    listModelProviders: vi.fn().mockResolvedValue([]),
    listTasks: vi.fn().mockResolvedValue([]),
    updateApp: vi.fn(),
  },
}))

beforeAll(() => {
  HTMLDialogElement.prototype.showModal = function showModal(this: HTMLDialogElement) {
    this.setAttribute('open', '')
  }
  HTMLDialogElement.prototype.close = function close(this: HTMLDialogElement) {
    this.removeAttribute('open')
  }
})

afterEach(cleanup)

describe('project agent time limit', () => {
  it('saves a new limit in seconds from the project menu', async () => {
    vi.mocked(api.updateApp).mockResolvedValue({
      id: 'app',
      name: 'Timer',
      description: '',
      tags: [],
      created_at: '',
      updated_at: '',
      task_timeout_s: 2700,
    })
    const noop = () => {}
    render(<AppView appId="app" onError={noop} toast={noop} goApps={noop} goSettings={noop} />)

    await screen.findByRole('button', { name: 'Project actions' })
    fireEvent.click(screen.getByRole('button', { name: 'Project actions' }))
    fireEvent.click(screen.getByRole('button', { name: 'Agent time limit · 30 min' }))

    const input = screen.getByTestId('task-timeout-minutes') as HTMLInputElement
    expect(input.value).toBe('30')
    fireEvent.change(input, { target: { value: '45' } })
    fireEvent.click(screen.getByRole('button', { name: 'Save time limit' }))

    await waitFor(() => expect(api.updateApp).toHaveBeenCalledWith('app', { task_timeout_s: 2700 }))
  })
})
