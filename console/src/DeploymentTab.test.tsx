import { afterEach, describe, expect, it, vi } from 'vitest'
import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { DeploymentTab } from './DeploymentTab'
import { api, Sandbox } from './api'

vi.mock('./api', () => ({
  api: {
    listApkBuilds: vi.fn(),
    startApkBuild: vi.fn(),
    apkDownloadUrl: (id: string) => `/v1/sandboxes/${id}/apk-builds/latest`,
  },
}))

const sb = { id: 'sb1', status: 'running' } as Sandbox

afterEach(() => {
  cleanup()
  vi.clearAllMocks()
})

describe('DeploymentTab', () => {
  it('offers a download only for the latest succeeded APK', async () => {
    vi.mocked(api.listApkBuilds).mockResolvedValue([
      { id: 'old', status: 'succeeded', started_at: '2020-01-01T00:00:00Z', version: '0.9.0' },
      { id: 'bad', status: 'failed', started_at: '2020-01-03T00:00:00Z', log: 'gradle failed' },
      { id: 'new', status: 'succeeded', started_at: '2020-01-02T00:00:00Z', version: '1.0.0' },
    ])
    render(<DeploymentTab sb={sb} onError={() => {}} toast={() => {}} />)

    expect(await screen.findByText('1.0.0')).toBeTruthy()
    expect(screen.getByText('0.9.0')).toBeTruthy()
    expect(screen.queryByTestId('apk-download')).toBeNull()
    expect(screen.queryByText('gradle failed')).toBeNull()

    fireEvent.click(screen.getByTestId('pipeline-new'))
    const link = await screen.findByTestId('apk-download')
    expect(link.getAttribute('href')).toBe('/v1/sandboxes/sb1/apk-builds/latest')
    expect(screen.getAllByTestId('apk-download').length).toBe(1)
    expect(screen.getByText('Build APK')).toBeTruthy()
    expect(screen.getByText('Send to Firebase App Distribution')).toBeTruthy()
    expect(screen.getByText('Notify users')).toBeTruthy()
    expect(screen.getAllByText('Not connected yet').length).toBeGreaterThan(0)

    fireEvent.click(screen.getByTestId('pipeline-bad'))
    expect(await screen.findByText('gradle failed')).toBeTruthy()
    expect(screen.queryByTestId('apk-download')).toBeNull()
    expect(screen.getAllByText('Skipped because the APK build failed.').length).toBe(2)
    expect((screen.getByRole('tab', { name: /iOS/ }) as HTMLButtonElement).disabled).toBe(true)
    expect((screen.getByRole('tab', { name: /Web/ }) as HTMLButtonElement).disabled).toBe(true)
  })

  it('shows the tester install link after Firebase accepts the APK', async () => {
    vi.mocked(api.listApkBuilds).mockResolvedValue([
      {
        id: 'sent',
        status: 'succeeded',
        started_at: '2020-01-02T00:00:00Z',
        version: '1.0.0',
        firebase_status: 'succeeded',
        testing_uri: 'https://appdistribution.firebase.google.com/testerapps/example',
        firebase_log: 'Firebase emailed the invited testers.',
      },
    ])
    render(<DeploymentTab sb={sb} onError={() => {}} toast={() => {}} />)
    expect(await screen.findByText('Sent to testers')).toBeTruthy()
    fireEvent.click(screen.getByTestId('pipeline-sent'))
    const link = await screen.findByTestId('firebase-testing-link')
    expect(link.getAttribute('href')).toBe('https://appdistribution.firebase.google.com/testerapps/example')
    expect(screen.getByText('Firebase emailed the invited testers.')).toBeTruthy()
  })
})
