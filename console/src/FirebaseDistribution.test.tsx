import { afterEach, describe, expect, it, vi } from 'vitest'
import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { FirebaseDistribution } from './FirebaseDistribution'
import { api } from './api'

vi.mock('./api', () => ({
  api: {
    listFirebaseDistributions: vi.fn(),
    createFirebaseDistribution: vi.fn(),
    updateFirebaseDistribution: vi.fn(),
    deleteFirebaseDistribution: vi.fn(),
  },
}))

const account = {
  type: 'service_account',
  project_id: 'inspirimate-prod',
  private_key: '-----BEGIN PRIVATE KEY-----\nsecret-key-material\n-----END PRIVATE KEY-----\n',
  client_email: 'distro@inspirimate-prod.iam.gserviceaccount.com',
}

afterEach(() => {
  cleanup()
  vi.clearAllMocks()
})

describe('FirebaseDistribution', () => {
  it('shows the fields a Firebase distribution connection needs', async () => {
    vi.mocked(api.listFirebaseDistributions).mockResolvedValue([])
    render(<FirebaseDistribution toast={() => {}} />)
    fireEvent.click(screen.getByTestId('firebase-add'))
    expect(screen.getByTestId('firebase-needs').textContent).toMatch(/app id/i)
    expect(screen.getByTestId('firebase-needs').textContent).toMatch(/Service account/)
    expect(screen.getByTestId('firebase-needs').textContent).toMatch(/tester/i)
    expect(screen.getByTestId('firebase-name')).toBeTruthy()
    expect(screen.getByTestId('firebase-app-id')).toBeTruthy()
    expect(screen.getByTestId('firebase-service-account')).toBeTruthy()
    expect(screen.getByTestId('firebase-group')).toBeTruthy()
    expect(screen.getByTestId('firebase-emails')).toBeTruthy()
  })

  it('sends the service account to the server and does not show the private key', async () => {
    const saved = {
      id: 'fd1',
      name: 'Inspirimate Android',
      app_id: '1:1234567890:android:abcdef',
      project_id: 'inspirimate-prod',
      client_email: account.client_email,
      group_alias: 'testers',
      tester_emails: [] as string[],
      secret_set: true,
      created_at: '2026-01-01T00:00:00Z',
    }
    let rows: typeof saved[] = []
    vi.mocked(api.listFirebaseDistributions).mockImplementation(async () => rows)
    vi.mocked(api.createFirebaseDistribution).mockImplementation(async () => {
      rows = [saved]
      return saved
    })
    const toast = vi.fn()
    render(<FirebaseDistribution toast={toast} />)
    fireEvent.click(screen.getByTestId('firebase-add'))
    fireEvent.change(screen.getByTestId('firebase-name'), { target: { value: 'Inspirimate Android' } })
    fireEvent.change(screen.getByTestId('firebase-app-id'), { target: { value: '1:1234567890:android:abcdef' } })
    fireEvent.change(screen.getByTestId('firebase-service-account'), { target: { value: JSON.stringify(account) } })
    fireEvent.change(screen.getByTestId('firebase-group'), { target: { value: 'testers' } })
    fireEvent.click(screen.getByTestId('firebase-save'))

    const card = await screen.findByTestId('firebase-preview')
    expect(card.textContent).toContain('Inspirimate Android')
    expect(card.textContent).toContain('1:1234567890:android:abcdef')
    expect(card.textContent).toContain(account.client_email)
    expect(card.textContent).not.toContain('secret-key-material')
    expect(api.createFirebaseDistribution).toHaveBeenCalled()
    const sent = vi.mocked(api.createFirebaseDistribution).mock.calls[0][0].service_account
    expect(sent).toContain('secret-key-material')
    expect(toast).toHaveBeenCalled()
  })
})
