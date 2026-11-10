import { afterEach, describe, expect, it, vi } from 'vitest'
import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { FirebaseDistribution } from './FirebaseDistribution'

const account = {
  type: 'service_account',
  project_id: 'inspirimate-prod',
  private_key: '-----BEGIN PRIVATE KEY-----\nsecret-key-material\n-----END PRIVATE KEY-----\n',
  client_email: 'distro@inspirimate-prod.iam.gserviceaccount.com',
}

afterEach(cleanup)

describe('FirebaseDistribution', () => {
  it('shows the fields a Firebase distribution connection needs', () => {
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

  it('checks the service account and then drops the private key', () => {
    const toast = vi.fn()
    render(<FirebaseDistribution toast={toast} />)
    fireEvent.click(screen.getByTestId('firebase-add'))
    fireEvent.change(screen.getByTestId('firebase-name'), { target: { value: 'Inspirimate Android' } })
    fireEvent.change(screen.getByTestId('firebase-app-id'), { target: { value: '1:1234567890:android:abcdef' } })
    fireEvent.change(screen.getByTestId('firebase-service-account'), { target: { value: JSON.stringify(account) } })
    fireEvent.change(screen.getByTestId('firebase-group'), { target: { value: 'testers' } })
    fireEvent.click(screen.getByTestId('firebase-save'))

    const card = screen.getByTestId('firebase-preview')
    expect(card.textContent).toContain('Inspirimate Android')
    expect(card.textContent).toContain('1:1234567890:android:abcdef')
    expect(card.textContent).toContain('inspirimate-prod')
    expect(card.textContent).toContain('distro@inspirimate-prod.iam.gserviceaccount.com')
    expect(card.textContent).toContain('testers')
    expect(card.textContent).not.toContain('secret-key-material')
    expect(document.body.textContent).not.toContain('secret-key-material')
    expect(toast).toHaveBeenCalledWith('Preview only. This connection is not saved on the instance yet.')
  })
})
