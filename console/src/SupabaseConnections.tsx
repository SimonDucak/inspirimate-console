import { useEffect, useState } from 'react'
import { api, SupabaseConnection } from './api'
import { Btn, Card, H, Input, Pill, c, mono } from './design/kit'

type Feature = 'database' | 'docs' | 'storage'
type Management = 'auth' | 'storage'

type Draft = {
  name: string
  projectRef: string
  database: boolean
  docs: boolean
  storage: boolean
  auth: boolean
  token: string
}

const emptyDraft = (): Draft => ({
  name: '',
  projectRef: '',
  database: true,
  docs: false,
  storage: true,
  auth: true,
  token: '',
})

const fieldStyle = { display: 'grid', gap: 5, fontSize: 12.5, color: c.muted } as const

function featuresOf(draft: Draft): Feature[] {
  const features: Feature[] = []
  if (draft.database) features.push('database')
  if (draft.docs) features.push('docs')
  if (draft.storage) features.push('storage')
  return features
}

function managementOf(draft: Draft): Management[] {
  const management: Management[] = []
  if (draft.auth) management.push('auth')
  if (draft.storage) management.push('storage')
  return management
}

function grantsOf(connection: SupabaseConnection): string[] {
  const grants: string[] = [...(connection.features || [])]
  for (const area of connection.management || []) {
    if (area === 'storage' && grants.includes('storage')) continue
    grants.push(area)
  }
  return grants
}

function hasGrant(draft: Draft): boolean {
  return featuresOf(draft).length + managementOf(draft).length > 0
}

export function SupabaseConnections({ toast }: { toast: (message: string) => void }) {
  const [connections, setConnections] = useState<SupabaseConnection[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)
  const [editing, setEditing] = useState<string | null>(null)
  const [form, setForm] = useState<Draft>(emptyDraft)
  const [confirmDelete, setConfirmDelete] = useState('')

  const load = () => {
    setLoading(true)
    api
      .listSupabaseConnections()
      .then((rows) => {
        setConnections(rows)
        setError('')
      })
      .catch((err: Error) => setError(err.message || 'Could not load Supabase connections'))
      .finally(() => setLoading(false))
  }

  useEffect(load, [])

  const edit = (connection?: SupabaseConnection) => {
    setConfirmDelete('')
    setError('')
    setEditing(connection?.id || '')
    setForm(
      connection
        ? {
            name: connection.name,
            projectRef: connection.project_ref,
            database: connection.features.includes('database'),
            docs: connection.features.includes('docs'),
            storage: connection.features.includes('storage') || (connection.management || []).includes('storage'),
            auth: (connection.management || []).includes('auth'),
            token: '',
          }
        : emptyDraft(),
    )
  }

  const save = async () => {
    const features = featuresOf(form)
    const management = managementOf(form)
    if (features.length + management.length === 0 || busy) return
    const existing = connections.find((item) => item.id === editing)
    const token = form.token.trim()
    if (!existing && !token) return
    setBusy(true)
    setError('')
    const body = {
      name: form.name.trim(),
      project_ref: form.projectRef.trim(),
      features,
      management,
      ...(token ? { token } : {}),
    }
    try {
      if (existing) await api.updateSupabaseConnection(existing.id, body)
      else await api.createSupabaseConnection({ ...body, token })
      setEditing(null)
      setForm(emptyDraft())
      toast('Supabase connection saved')
      load()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Could not save the connection')
    } finally {
      setBusy(false)
    }
  }

  const remove = async (id: string) => {
    if (busy) return
    setBusy(true)
    setError('')
    try {
      await api.deleteSupabaseConnection(id)
      setConfirmDelete('')
      if (editing === id) setEditing(null)
      toast('Supabase connection removed')
      load()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Could not remove the connection')
    } finally {
      setBusy(false)
    }
  }

  return (
    <Card style={{ padding: 16, marginBottom: 12 }} data-testid="settings-supabase-connections">
      <div style={{ display: 'flex', alignItems: 'center', gap: 12 }}>
        <H>Supabase connections</H>
        {editing === null && (
          <Btn sm style={{ marginLeft: 'auto' }} onClick={() => edit()}>
            Add connection
          </Btn>
        )}
      </div>
      <p style={{ color: c.muted, fontSize: 12.5, lineHeight: 1.6 }}>
        Save one Supabase personal access token (<span style={mono}>sbp_…</span>) per project. Create it for that
        project and allow <b>Database</b> (Migrations read-write), <b>Auth</b>, and <b>Storage</b>. This is not the anon
        key or the service-role key. The token is stored encrypted and is not shown again. The boxes below are the
        areas an agent is allowed to change. Google and Apple client ids still come from you when you ask for that
        sign-in.
      </p>
      {error && (
        <p role="alert" style={{ color: c.muted, fontSize: 12.5 }}>
          {error}
        </p>
      )}
      {loading && connections.length === 0 && editing === null && (
        <div style={{ padding: 16, background: c.panel2, borderRadius: 8, color: c.muted, fontSize: 12.5 }}>
          Loading connections…
        </div>
      )}
      {!loading && connections.length === 0 && editing === null && (
        <div style={{ padding: 16, background: c.panel2, borderRadius: 8, color: c.muted, fontSize: 12.5 }}>
          Add a connection for each Supabase project an agent should be able to change.
        </div>
      )}
      {connections.map((connection) => (
        <div key={connection.id} style={{ borderTop: `1px solid ${c.border}`, padding: '14px 0' }}>
          <div style={{ display: 'flex', alignItems: 'center', gap: 8, flexWrap: 'wrap' }}>
            <b style={{ fontSize: 13 }}>{connection.name}</b>
            {grantsOf(connection).map((grant) => (
              <Pill key={grant} tone="neutral">
                {grant}
              </Pill>
            ))}
            <span style={{ color: c.muted2, fontSize: 11.5 }}>{connection.token_set ? 'Token saved' : 'No token'}</span>
            <Btn sm variant="ghost" style={{ marginLeft: 'auto' }} onClick={() => edit(connection)}>
              Edit
            </Btn>
            <Btn sm variant="ghost" onClick={() => setConfirmDelete(connection.id)}>
              Remove
            </Btn>
          </div>
          <div style={{ ...mono, color: c.muted, fontSize: 11.5, margin: '7px 0', overflowWrap: 'anywhere' }}>
            {connection.project_ref}
          </div>
          {confirmDelete === connection.id && (
            <div role="alert" style={{ padding: 10, marginTop: 8, background: c.panel2, borderRadius: 6, fontSize: 12 }}>
              Remove this connection? The saved token is deleted with it.
              <div style={{ display: 'flex', gap: 8, marginTop: 8 }}>
                <Btn sm disabled={busy} onClick={() => remove(connection.id)}>
                  Remove connection
                </Btn>
                <Btn sm variant="ghost" onClick={() => setConfirmDelete('')}>
                  Cancel
                </Btn>
              </div>
            </div>
          )}
        </div>
      ))}
      {editing !== null && (
        <form
          onSubmit={(event) => {
            event.preventDefault()
            void save()
          }}
          style={{ display: 'grid', gap: 14, marginTop: 16, paddingTop: 14, borderTop: `1px solid ${c.border}` }}
        >
          <H size={13}>{editing ? 'Edit connection' : 'New connection'}</H>
          <label style={fieldStyle}>
            Connection name
            <Input
              required
              maxLength={100}
              value={form.name}
              placeholder="Production"
              onChange={(event) => setForm({ ...form, name: event.target.value })}
            />
          </label>
          <label style={fieldStyle}>
            Project ref
            <Input
              required
              maxLength={40}
              value={form.projectRef}
              placeholder="abcd1234"
              spellCheck={false}
              onChange={(event) => setForm({ ...form, projectRef: event.target.value })}
            />
            <span style={{ fontSize: 11.5 }}>The id in the Supabase dashboard URL, after /project/. Lowercase letters and digits only.</span>
          </label>
          <fieldset style={{ border: `1px solid ${c.border}`, borderRadius: 8, padding: 12 }}>
            <legend style={{ fontSize: 12.5, color: c.muted, padding: '0 6px' }}>Database and docs</legend>
            <label style={{ display: 'flex', gap: 8, fontSize: 12.5, marginBottom: 8 }}>
              <input
                type="checkbox"
                checked={form.database}
                onChange={(event) => setForm({ ...form, database: event.target.checked })}
              />
              database — create and change tables
            </label>
            <label style={{ display: 'flex', gap: 8, fontSize: 12.5, marginBottom: 8 }}>
              <input
                type="checkbox"
                checked={form.storage}
                onChange={(event) => setForm({ ...form, storage: event.target.checked })}
              />
              storage — buckets and storage settings
            </label>
            <label style={{ display: 'flex', gap: 8, fontSize: 12.5 }}>
              <input type="checkbox" checked={form.docs} onChange={(event) => setForm({ ...form, docs: event.target.checked })} />
              docs — read Supabase documentation
            </label>
          </fieldset>
          <fieldset style={{ border: `1px solid ${c.border}`, borderRadius: 8, padding: 12 }}>
            <legend style={{ fontSize: 12.5, color: c.muted, padding: '0 6px' }}>Authentication</legend>
            <label style={{ display: 'flex', gap: 8, fontSize: 12.5 }}>
              <input
                type="checkbox"
                checked={form.auth}
                onChange={(event) => setForm({ ...form, auth: event.target.checked })}
              />
              authentication — turn sign-in methods on and off
            </label>
            {!hasGrant(form) && (
              <p role="alert" style={{ color: c.muted, fontSize: 12, margin: '8px 0 0' }}>
                Choose at least one area.
              </p>
            )}
          </fieldset>
          <label style={fieldStyle}>
            Personal access token
            <Input
              type="password"
              autoComplete="new-password"
              required={!editing}
              value={form.token}
              placeholder={editing ? 'Leave blank to keep the saved token' : 'sbp_…'}
              onChange={(event) => setForm({ ...form, token: event.target.value })}
            />
          </label>
          <div style={{ display: 'flex', gap: 8 }}>
            <Btn type="submit" disabled={busy || !hasGrant(form)}>
              Save connection
            </Btn>
            <Btn
              type="button"
              variant="ghost"
              onClick={() => {
                setEditing(null)
                setForm(emptyDraft())
              }}
            >
              Cancel
            </Btn>
          </div>
        </form>
      )}
    </Card>
  )
}
