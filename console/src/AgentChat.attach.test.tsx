import { afterEach, describe, expect, it, vi } from 'vitest'
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { AgentChat } from './AgentChat'
import { api } from './api'

vi.mock('./api', () => ({
  api: {
    getAgents: vi.fn().mockResolvedValue([]),
    listModelProviders: vi.fn().mockResolvedValue([]),
    listTasks: vi.fn().mockResolvedValue([]),
    putWorkspaceFile: vi.fn().mockResolvedValue(undefined),
    submitTask: vi.fn().mockResolvedValue({ id: 'task-1' }),
    taskEventsURL: vi.fn().mockReturnValue('/events'),
  },
}))

afterEach(cleanup)

describe('chat file attachments', () => {
  it('saves up to two files into the project and names them in the prompt', async () => {
    vi.stubGlobal(
      'EventSource',
      class {
        addEventListener() {}
        close() {}
      },
    )
    const sandbox = {
      id: 'sb',
      status: 'running',
      created_at: '',
      ports: [],
    }
    const noop = () => {}
    render(
      <AgentChat sb={sandbox} onError={noop} toast={noop} refresh={noop} goSettings={noop} />,
    )
    fireEvent.change(screen.getByTestId('task-file'), {
      target: {
        files: [
          new File(['name,kcal\napple,52\n'], 'nutrients.csv', { type: 'text/csv' }),
          new File(['other'], 'notes.txt', { type: 'text/plain' }),
          new File(['too many'], 'third.csv', { type: 'text/csv' }),
        ],
      },
    })
    expect(screen.getByText('You can attach up to 2 files.')).toBeTruthy()
    expect(screen.getByText('nutrients.csv')).toBeTruthy()
    expect(screen.getByText('notes.txt')).toBeTruthy()
    expect(screen.queryByText('third.csv')).toBeNull()

    fireEvent.change(screen.getByTestId('task-prompt'), {
      target: { value: 'Seed the nutrients table from this file.' },
    })
    await waitFor(() => expect((screen.getByTestId('task-send') as HTMLButtonElement).disabled).toBe(false))
    screen.getByTestId('task-prompt').closest('form')!.requestSubmit()

    await waitFor(() => expect(api.submitTask).toHaveBeenCalled())
    expect(api.putWorkspaceFile).toHaveBeenCalledWith('sb', 'uploads/nutrients.csv', 'name,kcal\napple,52\n')
    expect(api.putWorkspaceFile).toHaveBeenCalledWith('sb', 'uploads/notes.txt', 'other')
    const prompt = vi.mocked(api.submitTask).mock.calls[0][1]
    expect(prompt).toContain('uploads/nutrients.csv')
    expect(prompt).toContain('uploads/notes.txt')
    expect(prompt).toContain('Seed the nutrients table from this file.')
    expect(screen.getByText('Seed the nutrients table from this file.')).toBeTruthy()
  })

  it('uploads a PNG as the file itself and refuses an unsupported video', async () => {
    vi.clearAllMocks()
    vi.stubGlobal(
      'EventSource',
      class {
        addEventListener() {}
        close() {}
      },
    )
    const sandbox = {
      id: 'sb',
      status: 'running',
      created_at: '',
      ports: [],
    }
    const noop = () => {}
    const { unmount } = render(
      <AgentChat sb={sandbox} onError={noop} toast={noop} refresh={noop} goSettings={noop} />,
    )
    const logo = new File([new Uint8Array([0xff, 0xd8, 0xff, 0x00])], 'logo.png', { type: 'image/png' })
    fireEvent.change(screen.getByTestId('task-file'), { target: { files: [logo] } })
    fireEvent.change(screen.getByTestId('task-prompt'), { target: { value: 'Put this logo in the header.' } })
    await waitFor(() => expect((screen.getByTestId('task-send') as HTMLButtonElement).disabled).toBe(false))
    screen.getByTestId('task-prompt').closest('form')!.requestSubmit()

    await waitFor(() => expect(api.submitTask).toHaveBeenCalled())
    expect(api.putWorkspaceFile).toHaveBeenCalledWith('sb', 'uploads/logo.png', logo)
    const prompt = vi.mocked(api.submitTask).mock.calls.at(-1)?.[1] as string
    expect(prompt).toContain('uploads/logo.png')
    expect(prompt).toContain('copy them into the site')
    expect(prompt).toContain('Put this logo in the header.')

    unmount()
    cleanup()
    render(<AgentChat sb={sandbox} onError={noop} toast={noop} refresh={noop} goSettings={noop} />)
    fireEvent.change(screen.getByTestId('task-file'), {
      target: { files: [new File(['webm'], 'clip.webm', { type: 'video/webm' })] },
    })
    expect(screen.getByText('clip.webm is not a supported file. Attach a text file, or a JPG, PNG, or MP4.')).toBeTruthy()
    expect(screen.queryByText('clip.webm')).toBeNull()
  })
})
