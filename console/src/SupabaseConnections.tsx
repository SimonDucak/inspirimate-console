import { useState } from 'react'
import { Btn, Card, H, Input, Pill, c, mono } from './design/kit'

type Feature = 'database' | 'docs'

type Connection = {
  id: string
  name: string
  projectRef: string
  features: Feature[]
  hasToken: boolean
}

type Draft = {
  name: string
  projectRef: string
  database: boolean
  docs: boolean
  token: string
}

const emptyDraft = (): Draft => ({ name: '', projectRef: '', database: true, docs: false, token: '' })

const sample = (): Connection[] => [
  { id: 'sample-production', name: 'Production', projectRef: 'abcd1234', features: ['database'], hasToken: true },
]

const fieldStyle = { display: 'grid', gap: 5, fontSize: 12.5, color: c.muted } as const

function featuresOf(draft: Draft): Feature[] {
  const features: Feature[] = []
  if (draft.database) features.push('database')
  if (draft.docs) features.push('docs')
  return features
}

export function SupabaseConnections({ toast }: { toast: (message: string) => void }) {
  const [connections, setConnections] = useState<Connection[]>(sample)
  const [editing, setEditing] = useState<string | null>(null)
  const [form, setForm] = useState<Draft>(emptyDraft)
  const [confirmDelete, setConfirmDelete] = useState('')

  const edit = (connection?: Connection) => {
    setConfirmDelete('')
    setEditing(connection?.id || '')
    setForm(
      connection
        ? {
            name: connection.name,
            projectRef: connection.projectRef,
            database: connection.features.includes('database'),
            docs: connection.features.includes('docs'),
            token: '',
          }
        : emptyDraft(),
    )
  }

  const save = () => {
    const features = featuresOf(form)
    if (features.length === 0) return
    const existing = connections.find((item) => item.id === editing)
    if (!existing && !form.token.trim()) return
    const next: Connection = {
      id: existing?.id || `supabase-${Date.now()}`,
      name: form.name.trim(),
      projectRef: form.projectRef.trim(),
      features,
      hasToken: form.token.trim() ? true : !!existing?.hasToken,
    }
    setConnections((list) => (existing ? list.map((item) => (item.id === existing.id ? next : item)) : [...list, next]))
    setEditing(null)
    setForm(emptyDraft())
    toast('Supabase connection saved on this page only')
  }

  const remove = (id: string) => {
    setConnections((list) => list.filter((item) => item.id !== id))
    setConfirmDelete('')
    if (editing === id) setEditing(null)
    toast('Supabase connection removed')
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
        Save a Supabase personal access token (<span style={mono}>sbp_…</span>) for each project you want an agent to
        change. Scope it to that one project with <b>Migrations: read-write</b>. This is not the anon key or the
        service-role key. This screen is a preview and does not connect to Supabase yet. Connections stay in this
        browser tab and disappear on reload.
      </p>
      {connections.length === 0 && editing === null && (
        <div style={{ padding: 16, background: c.panel2, borderRadius: 8, color: c.muted, fontSize: 12.5 }}>
          Add a connection for each Supabase project an agent should be able to change.
        </div>
      )}
      {connections.map((connection) => (
        <div key={connection.id} style={{ borderTop: `1px solid ${c.border}`, padding: '14px 0' }}>
          <div style={{ display: 'flex', alignItems: 'center', gap: 8, flexWrap: 'wrap' }}>
            <b style={{ fontSize: 13 }}>{connection.name}</b>
            {connection.features.map((feature) => (
              <Pill key={feature} tone="neutral">
                {feature}
              </Pill>
            ))}
            <span style={{ color: c.muted2, fontSize: 11.5 }}>{connection.hasToken ? 'Token saved' : 'No token'}</span>
            <Btn sm variant="ghost" style={{ marginLeft: 'auto' }} onClick={() => edit(connection)}>
              Edit
            </Btn>
            <Btn sm variant="ghost" onClick={() => setConfirmDelete(connection.id)}>
              Remove
            </Btn>
          </div>
          <div style={{ ...mono, color: c.muted, fontSize: 11.5, margin: '7px 0', overflowWrap: 'anywhere' }}>
            {connection.projectRef}
          </div>
          {confirmDelete === connection.id && (
            <div role="alert" style={{ padding: 10, marginTop: 8, background: c.panel2, borderRadius: 6, fontSize: 12 }}>
              Remove this connection from the preview list?
              <div style={{ display: 'flex', gap: 8, marginTop: 8 }}>
                <Btn sm onClick={() => remove(connection.id)}>
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
            save()
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
            <span style={{ fontSize: 11.5 }}>The id in the Supabase dashboard URL, after /project/.</span>
          </label>
          <fieldset style={{ border: `1px solid ${c.border}`, borderRadius: 8, padding: 12 }}>
            <legend style={{ fontSize: 12.5, color: c.muted, padding: '0 6px' }}>Features</legend>
            <label style={{ display: 'flex', gap: 8, fontSize: 12.5, marginBottom: 8 }}>
              <input
                type="checkbox"
                checked={form.database}
                onChange={(event) => setForm({ ...form, database: event.target.checked })}
              />
              database — create and change tables
            </label>
            <label style={{ display: 'flex', gap: 8, fontSize: 12.5 }}>
              <input type="checkbox" checked={form.docs} onChange={(event) => setForm({ ...form, docs: event.target.checked })} />
              docs — read Supabase documentation
            </label>
            {featuresOf(form).length === 0 && (
              <p role="alert" style={{ color: c.muted, fontSize: 12, margin: '8px 0 0' }}>
                Choose at least one feature.
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
            <Btn type="submit" disabled={featuresOf(form).length === 0}>
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
