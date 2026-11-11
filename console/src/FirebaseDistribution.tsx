import { useEffect, useState } from 'react'
import { api, FirebaseDistribution as FirebaseConnection } from './api'
import { Btn, Card, H, Input, c, mono } from './design/kit'
import { useT } from './i18n'

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

function readAccount(raw: string): true | string {
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
  return true
}

export function FirebaseDistribution({ toast }: { toast: (message: string) => void }) {
  const t = useT()
  const [connections, setConnections] = useState<FirebaseConnection[]>([])
  const [loading, setLoading] = useState(true)
  const [editing, setEditing] = useState<string | null>(null)
  const [form, setForm] = useState<Draft>(emptyDraft)
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)

  const load = () => {
    setLoading(true)
    api
      .listFirebaseDistributions()
      .then((rows) => {
        setConnections(rows)
        setError('')
      })
      .catch((err: Error) => setError(err.message || t('Could not load Firebase distributions')))
      .finally(() => setLoading(false))
  }

  useEffect(load, [])

  const edit = (connection?: FirebaseConnection) => {
    setError('')
    setEditing(connection?.id || '')
    setForm(
      connection
        ? {
            name: connection.name,
            appId: connection.app_id,
            serviceAccount: '',
            group: connection.group_alias || '',
            emails: (connection.tester_emails || []).join('\n'),
          }
        : emptyDraft(),
    )
  }

  const save = async () => {
    const name = form.name.trim()
    const appId = form.appId.trim()
    const group = form.group.trim()
    const emails = emailList(form.emails)
    const existing = connections.find((item) => item.id === editing)
    const account = form.serviceAccount.trim()
    if (!name) {
      setError(t('Give the connection a name.'))
      return
    }
    if (!appIdPattern.test(appId)) {
      setError(t('The Android app id looks like 1:1234567890:android:abcdef.'))
      return
    }
    if (!existing && !account) {
      setError(t('Paste the whole service account JSON file.'))
      return
    }
    if (account) {
      const checked = readAccount(account)
      if (checked !== true) {
        setError(t(checked))
        return
      }
    }
    if (!group && emails.length === 0) {
      setError(t('Add a tester group or at least one tester email.'))
      return
    }
    if (emails.some((email) => !validEmail(email))) {
      setError(t('One of the tester emails is not a valid address.'))
      return
    }
    setBusy(true)
    setError('')
    const body = {
      name,
      app_id: appId,
      group_alias: group,
      tester_emails: emails,
      ...(account ? { service_account: account } : {}),
    }
    try {
      if (existing) await api.updateFirebaseDistribution(existing.id, body)
      else await api.createFirebaseDistribution({ ...body, service_account: account })
      setForm(emptyDraft())
      setEditing(null)
      toast(t('Firebase distribution saved'))
      load()
    } catch (err) {
      setError(err instanceof Error ? err.message : t('Could not save the connection'))
    } finally {
      setBusy(false)
    }
  }

  const remove = async (id: string) => {
    if (busy) return
    setBusy(true)
    setError('')
    try {
      await api.deleteFirebaseDistribution(id)
      if (editing === id) setEditing(null)
      toast(t('Firebase distribution removed'))
      load()
    } catch (err) {
      setError(err instanceof Error ? err.message : t('Could not remove the connection'))
    } finally {
      setBusy(false)
    }
  }

  return (
    <Card style={{ padding: 16, marginBottom: 12 }} data-testid="settings-firebase-distribution">
      <div style={{ display: 'flex', alignItems: 'center', gap: 12 }}>
        <H>{t('Firebase distribution')}</H>
        {editing === null && (
          <Btn sm style={{ marginLeft: 'auto' }} data-testid="firebase-add" onClick={() => edit()}>
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
      {loading && connections.length === 0 && editing === null && (
        <div style={{ padding: 16, background: c.panel2, borderRadius: 8, color: c.muted, fontSize: 12.5 }}>
          {t('Loading…')}
        </div>
      )}
      {!loading && connections.length === 0 && editing === null && (
        <div style={{ padding: 16, background: c.panel2, borderRadius: 8, color: c.muted, fontSize: 12.5 }}>
          {t('No Firebase distribution connections yet. Add one here, then choose it on the project.')}
        </div>
      )}
      {connections.map((row) => (
        <div key={row.id} style={{ borderTop: `1px solid ${c.border}`, padding: '14px 0' }} data-testid="firebase-preview">
          <div style={{ display: 'flex', alignItems: 'center', gap: 8, flexWrap: 'wrap' }}>
            <b style={{ fontSize: 13 }}>{row.name}</b>
            <span style={{ color: c.muted2, fontSize: 11.5 }}>{t('Service account stored encrypted')}</span>
            <span style={{ marginLeft: 'auto', display: 'flex', gap: 8 }}>
              <Btn sm type="button" onClick={() => edit(row)}>
                {t('Edit')}
              </Btn>
              <Btn sm type="button" onClick={() => void remove(row.id)}>
                {t('Remove')}
              </Btn>
            </span>
          </div>
          <div style={{ ...mono, color: c.muted, fontSize: 11.5, margin: '7px 0', overflowWrap: 'anywhere' }}>
            {row.app_id}
          </div>
          <div style={{ color: c.muted, fontSize: 12 }}>
            {row.project_id} · {row.client_email}
          </div>
          <div style={{ color: c.muted, fontSize: 12, marginTop: 4 }}>
            {row.group_alias ? `${t('Group')} ${row.group_alias}` : null}
            {row.group_alias && (row.tester_emails || []).length > 0 ? ' · ' : null}
            {(row.tester_emails || []).join(', ')}
          </div>
        </div>
      ))}
      {editing !== null && (
        <form
          data-testid="firebase-form"
          onSubmit={(event) => {
            event.preventDefault()
            void save()
          }}
          style={{ display: 'grid', gap: 14, marginTop: 16, paddingTop: 14, borderTop: `1px solid ${c.border}` }}
        >
          <H size={13}>{editing ? t('Edit connection') : t('New connection')}</H>
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
              required={!editing}
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
                'Google Cloud → IAM → Service accounts → Keys → Add key → JSON. Role: Firebase App Distribution Admin. The key is stored encrypted and is not shown again.',
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
            <Btn type="submit" data-testid="firebase-save" disabled={busy}>
              {t('Save connection')}
            </Btn>
            <Btn
              type="button"
              variant="ghost"
              onClick={() => {
                setEditing(null)
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
