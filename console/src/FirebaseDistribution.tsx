import { useState } from 'react'
import { Btn, Card, H, Input, Pill, c, mono } from './design/kit'
import { useT } from './i18n'

type Preview = {
  id: string
  name: string
  appId: string
  projectId: string
  clientEmail: string
  group: string
  emails: string[]
}

type Draft = {
  name: string
  appId: string
  serviceAccount: string
  group: string
  emails: string
}

const emptyDraft = (): Draft => ({
  name: '',
  appId: '',
  serviceAccount: '',
  group: '',
  emails: '',
})

const fieldStyle = { display: 'grid', gap: 5, fontSize: 12.5, color: c.muted } as const

const appIdPattern = /^1:\d+:android:[0-9a-fA-F]+$/

function emailList(value: string): string[] {
  return value
    .split(/[\s,]+/)
    .map((item) => item.trim())
    .filter(Boolean)
}

function validEmail(value: string): boolean {
  return /^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(value)
}

// readAccount checks the JSON shape and returns only the two labels we show.
// The private key is never copied into the preview.
function readAccount(raw: string): { projectId: string; clientEmail: string } | string {
  let parsed: unknown
  try {
    parsed = JSON.parse(raw)
  } catch {
    return 'Paste the whole service account JSON file.'
  }
  if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) {
    return 'Paste the whole service account JSON file.'
  }
  const account = parsed as { project_id?: unknown; client_email?: unknown; private_key?: unknown }
  if (typeof account.project_id !== 'string' || !account.project_id.trim()) {
    return 'The JSON needs a project_id.'
  }
  if (typeof account.client_email !== 'string' || !account.client_email.includes('@')) {
    return 'The JSON needs a client_email.'
  }
  if (typeof account.private_key !== 'string' || !account.private_key.includes('PRIVATE KEY')) {
    return 'The JSON needs a private_key. Download the key file again.'
  }
  return { projectId: account.project_id.trim(), clientEmail: account.client_email.trim() }
}

export function FirebaseDistribution({ toast }: { toast: (message: string) => void }) {
  const t = useT()
  const [previews, setPreviews] = useState<Preview[]>([])
  const [editing, setEditing] = useState(false)
  const [form, setForm] = useState<Draft>(emptyDraft)
  const [error, setError] = useState('')

  const save = () => {
    const name = form.name.trim()
    const appId = form.appId.trim()
    const group = form.group.trim()
    const emails = emailList(form.emails)
    if (!name) {
      setError(t('Give the connection a name.'))
      return
    }
    if (!appIdPattern.test(appId)) {
      setError(t('The Android app id looks like 1:1234567890:android:abcdef.'))
      return
    }
    const account = readAccount(form.serviceAccount.trim())
    if (typeof account === 'string') {
      setError(t(account))
      return
    }
    if (!group && emails.length === 0) {
      setError(t('Add a tester group or at least one tester email.'))
      return
    }
    if (emails.some((email) => !validEmail(email))) {
      setError(t('One of the tester emails is not a valid address.'))
      return
    }
    setPreviews((rows) => [
      {
        id: `${Date.now()}`,
        name,
        appId,
        projectId: account.projectId,
        clientEmail: account.clientEmail,
        group,
        emails,
      },
      ...rows,
    ])
    setForm(emptyDraft())
    setEditing(false)
    setError('')
    toast(t('Preview only. This connection is not saved on the instance yet.'))
  }

  return (
    <Card style={{ padding: 16, marginBottom: 12 }} data-testid="settings-firebase-distribution">
      <div style={{ display: 'flex', alignItems: 'center', gap: 12 }}>
        <H>{t('Firebase distribution')}</H>
        {!editing && (
          <Btn sm style={{ marginLeft: 'auto' }} data-testid="firebase-add" onClick={() => setEditing(true)}>
            {t('Add connection')}
          </Btn>
        )}
      </div>
      <p style={{ color: c.muted, fontSize: 12.5, lineHeight: 1.6 }}>
        {t(
          'One connection sends this project’s APK to one Firebase Android app. Create the app in Firebase with the same package name as the APK, then bring the app id, a service account key, and the testers.',
        )}
      </p>
      <ul className="firebase-needs" data-testid="firebase-needs">
        <li>{t('Firebase Android app id, from Project settings → Your apps.')}</li>
        <li>
          {t(
            'Service account JSON key. The account needs the role Firebase App Distribution Admin, and the Firebase App Distribution API must be on.',
          )}
        </li>
        <li>{t('A tester group alias, tester emails, or both. Those Google accounts receive the install link.')}</li>
      </ul>
      {error && (
        <p role="alert" style={{ color: c.bad, fontSize: 12.5 }}>
          {error}
        </p>
      )}
      {previews.length === 0 && !editing && (
        <div style={{ padding: 16, background: c.panel2, borderRadius: 8, color: c.muted, fontSize: 12.5 }}>
          {t('No Firebase distribution connection yet. This screen is a preview and does not save anything.')}
        </div>
      )}
      {previews.map((row) => (
        <div key={row.id} style={{ borderTop: `1px solid ${c.border}`, padding: '14px 0' }} data-testid="firebase-preview">
          <div style={{ display: 'flex', alignItems: 'center', gap: 8, flexWrap: 'wrap' }}>
            <b style={{ fontSize: 13 }}>{row.name}</b>
            <Pill tone="warn">{t('Preview')}</Pill>
            <span style={{ color: c.muted2, fontSize: 11.5 }}>{t('Service account checked, key not kept')}</span>
          </div>
          <div style={{ ...mono, color: c.muted, fontSize: 11.5, margin: '7px 0', overflowWrap: 'anywhere' }}>
            {row.appId}
          </div>
          <div style={{ color: c.muted, fontSize: 12 }}>
            {row.projectId} · {row.clientEmail}
          </div>
          <div style={{ color: c.muted, fontSize: 12, marginTop: 4 }}>
            {row.group ? `${t('Group')} ${row.group}` : null}
            {row.group && row.emails.length > 0 ? ' · ' : null}
            {row.emails.length > 0 ? row.emails.join(', ') : null}
          </div>
        </div>
      ))}
      {editing && (
        <form
          data-testid="firebase-form"
          onSubmit={(event) => {
            event.preventDefault()
            save()
          }}
          style={{ display: 'grid', gap: 14, marginTop: 16, paddingTop: 14, borderTop: `1px solid ${c.border}` }}
        >
          <H size={13}>{t('New connection')}</H>
          <label style={fieldStyle}>
            {t('Connection name')}
            <Input
              required
              maxLength={100}
              value={form.name}
              placeholder="Inspirimate Android"
              data-testid="firebase-name"
              onChange={(event) => setForm({ ...form, name: event.target.value })}
            />
            <span style={{ fontSize: 11.5 }}>{t('The name you will choose on the project.')}</span>
          </label>
          <label style={fieldStyle}>
            {t('Firebase Android app id')}
            <Input
              required
              value={form.appId}
              placeholder="1:1234567890:android:abcdef"
              spellCheck={false}
              data-testid="firebase-app-id"
              onChange={(event) => setForm({ ...form, appId: event.target.value })}
            />
            <span style={{ fontSize: 11.5 }}>
              {t('Firebase console → Project settings → Your apps → App ID. The package name there must match the APK.')}
            </span>
          </label>
          <label style={fieldStyle}>
            {t('Service account JSON')}
            <textarea
              required
              value={form.serviceAccount}
              spellCheck={false}
              data-testid="firebase-service-account"
              placeholder={'{\n  "type": "service_account",\n  "project_id": "…",\n  "private_key": "…",\n  "client_email": "…"\n}'}
              onChange={(event) => setForm({ ...form, serviceAccount: event.target.value })}
              style={{
                minHeight: 140,
                resize: 'vertical',
                fontFamily: 'ui-monospace, monospace',
                fontSize: 12,
                color: c.fg,
                background: c.panel2,
                border: `1px solid ${c.border}`,
                borderRadius: 8,
                padding: 10,
              }}
            />
            <span style={{ fontSize: 11.5 }}>
              {t(
                'Google Cloud → IAM → Service accounts → Keys → Add key → JSON. Role: Firebase App Distribution Admin. This preview checks the file and then discards the private key.',
              )}
            </span>
          </label>
          <label style={fieldStyle}>
            {t('Tester group')}
            <Input
              value={form.group}
              placeholder="testers"
              spellCheck={false}
              data-testid="firebase-group"
              onChange={(event) => setForm({ ...form, group: event.target.value })}
            />
            <span style={{ fontSize: 11.5 }}>{t('The group alias from Firebase App Distribution. Optional when you list emails.')}</span>
          </label>
          <label style={fieldStyle}>
            {t('Tester emails')}
            <textarea
              value={form.emails}
              spellCheck={false}
              data-testid="firebase-emails"
              placeholder={'ada@example.com\ngrace@example.com'}
              onChange={(event) => setForm({ ...form, emails: event.target.value })}
              style={{
                minHeight: 72,
                resize: 'vertical',
                fontFamily: 'ui-monospace, monospace',
                fontSize: 12,
                color: c.fg,
                background: c.panel2,
                border: `1px solid ${c.border}`,
                borderRadius: 8,
                padding: 10,
              }}
            />
            <span style={{ fontSize: 11.5 }}>{t('One Google account per line. Optional when you set a tester group.')}</span>
          </label>
          <div style={{ display: 'flex', gap: 8 }}>
            <Btn type="submit" data-testid="firebase-save">
              {t('Check connection')}
            </Btn>
            <Btn
              type="button"
              variant="ghost"
              onClick={() => {
                setEditing(false)
                setForm(emptyDraft())
                setError('')
              }}
            >
              {t('Cancel')}
            </Btn>
          </div>
        </form>
      )}
    </Card>
  )
}
