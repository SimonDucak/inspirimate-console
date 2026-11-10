import { afterEach, describe, expect, it, vi } from 'vitest'
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { DeployTab } from './DeployTab'
import { api, Sandbox } from './api'

vi.mock('./api', () => ({
  api: {
    getWorkspaceFile: vi.fn(),
    putWorkspaceFile: vi.fn(),
  },
}))

vi.mock('./CodeEditor', () => ({
  CodeEditor: ({
    value,
    onChange,
  }: {
    value: string
    onChange: (value: string) => void
  }) => <textarea aria-label="deploy.yaml" value={value} onChange={(e) => onChange(e.target.value)} />,
}))

const sb = { id: 'sb1', status: 'running' } as Sandbox

afterEach(() => {
  cleanup()
  vi.clearAllMocks()
})

describe('DeployTab', () => {
  it('shows a Build stage and a Deploy stage from deploy.yaml', async () => {
    vi.mocked(api.getWorkspaceFile).mockResolvedValue(`version: 1
target: pi
build:
  command: pnpm build
  output: dist
steps:
  - copy: dist
    to: /var/www/app
`)
    const noop = () => {}
    render(<DeployTab sb={sb} onError={noop} toast={noop} />)
    expect((await screen.findByTestId('deploy-stage-build')).textContent).toContain('pnpm build')
    expect(screen.getByTestId('deploy-stage-deploy').textContent).toContain('copy dist')
    expect(screen.getByTestId('deploy-pipeline').querySelectorAll('.deploy-stage')).toHaveLength(2)
  })

  it('drops the Build stage when the build command is cleared', async () => {
    vi.mocked(api.getWorkspaceFile).mockResolvedValue('build:\n  command: pnpm build\nsteps:\n  - run: python3 main.py\n')
    const noop = () => {}
    render(<DeployTab sb={sb} onError={noop} toast={noop} />)
    expect(await screen.findByTestId('deploy-stage-build')).toBeTruthy()
    fireEvent.change(screen.getByLabelText('deploy.yaml'), {
      target: { value: 'steps:\n  - run: python3 main.py\n' },
    })
    expect(screen.queryByTestId('deploy-stage-build')).toBeNull()
    expect(screen.getByTestId('deploy-stage-deploy').textContent).toContain('python3 main.py')
  })

  it('saves the editor contents as deploy.yaml', async () => {
    vi.mocked(api.getWorkspaceFile).mockResolvedValue(null)
    vi.mocked(api.putWorkspaceFile).mockResolvedValue(undefined)
    const toast = vi.fn()
    render(<DeployTab sb={sb} onError={() => {}} toast={toast} />)
    expect((await screen.findByLabelText('deploy.yaml') as HTMLTextAreaElement).value).toBe('')
    fireEvent.click(screen.getByTestId('deploy-starter'))
    fireEvent.click(screen.getByTestId('deploy-save'))
    await waitFor(() => expect(api.putWorkspaceFile).toHaveBeenCalledWith('sb1', 'deploy.yaml', expect.stringContaining('python3 main.py')))
    expect(toast).toHaveBeenCalledWith('Saved deploy.yaml')
  })
})
