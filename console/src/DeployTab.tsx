import { Suspense, lazy, useEffect, useMemo, useRef, useState } from 'react'
import { api, Sandbox } from './api'
import { Btn, Card, H, Pill } from './design/kit'
import { useT } from './i18n'
import { useUnsavedChanges } from './unsavedChanges'
import { deployStages, parseDeployYaml, starterDeployYaml, type DeployStageId } from './deploy'

const CodeEditor = lazy(() => import('./CodeEditor').then((m) => ({ default: m.CodeEditor })))

export function DeployTab({
  sb,
  onError,
  toast,
}: {
  sb: Sandbox | null
  onError: (message: string) => void
  toast: (message: string) => void
}) {
  const t = useT()
  const sandboxId = sb?.id
  const onErrorRef = useRef(onError)
  onErrorRef.current = onError
  const [content, setContent] = useState('')
  const [orig, setOrig] = useState('')
  const [missing, setMissing] = useState(false)
  const [loading, setLoading] = useState(false)
  const [saving, setSaving] = useState(false)
  const [selected, setSelected] = useState<DeployStageId | null>(null)
  const dirty = content !== orig
  useUnsavedChanges(dirty)

  useEffect(() => {
    if (!sandboxId) return
    let cancel = false
    setLoading(true)
    api
      .getWorkspaceFile(sandboxId, 'deploy.yaml')
      .then((text) => {
        if (cancel) return
        const next = text ?? ''
        setContent(next)
        setOrig(next)
        setMissing(text == null)
      })
      .catch((e) => {
        if (!cancel) onErrorRef.current((e as Error).message)
      })
      .finally(() => {
        if (!cancel) setLoading(false)
      })
    return () => {
      cancel = true
    }
  }, [sandboxId])

  const parsed = useMemo(() => parseDeployYaml(content), [content])
  const stages = parsed.error ? [] : deployStages(parsed.file)
  const active = stages.some((stage) => stage.id === selected) ? selected : (stages[0]?.id ?? null)

  const save = async () => {
    if (!sandboxId || !dirty) return
    setSaving(true)
    try {
      await api.putWorkspaceFile(sandboxId, 'deploy.yaml', content)
      setOrig(content)
      setMissing(false)
      toast(t('Saved deploy.yaml'))
    } catch (e) {
      onError((e as Error).message)
    } finally {
      setSaving(false)
    }
  }

  if (!sandboxId) {
    return (
      <div className="deploy-tab" data-testid="deploy-tab">
        <H>{t('Deploy')}</H>
        <p className="deploy-help">{t('Create a workspace to edit the deploy file.')}</p>
      </div>
    )
  }

  return (
    <div className="deploy-tab" data-testid="deploy-tab">
      <div className="deploy-heading">
        <H>{t('Deploy')}</H>
        {parsed.error ? (
          <Pill tone="bad">{t('Invalid deploy.yaml')}</Pill>
        ) : stages.length > 0 ? (
          <Pill tone="good">
            {stages.length === 1 ? t('1 stage') : t('{count} stages', { count: stages.length })}
          </Pill>
        ) : null}
      </div>
      <p className="deploy-help">
        {t(
          'Edit deploy.yaml. A Build stage appears when build.command is set. A Deploy stage appears when steps are listed.',
        )}
      </p>

      {parsed.error ? (
        <p className="deploy-error" role="alert" data-testid="deploy-error">
          {parsed.error}
        </p>
      ) : stages.length === 0 ? (
        <p className="deploy-empty" data-testid="deploy-empty">
          {t('No stages yet. Set build.command for a Build stage, or add steps for a Deploy stage.')}
        </p>
      ) : (
        <div className="deploy-pipeline" role="list" data-testid="deploy-pipeline">
          {stages.map((stage, index) => (
            <div className="deploy-stage" key={stage.id} role="listitem">
              {index > 0 && <span className="deploy-stage-link" aria-hidden="true" />}
              <button
                type="button"
                className={active === stage.id ? 'deploy-stage-card selected' : 'deploy-stage-card'}
                data-testid={`deploy-stage-${stage.id}`}
                aria-pressed={active === stage.id}
                onClick={() => setSelected(stage.id)}
              >
                <span className="deploy-stage-index">{index + 1}</span>
                <span className="deploy-stage-name">{t(stage.label)}</span>
                <ul className="deploy-stage-lines">
                  {stage.lines.map((line) => (
                    <li key={line}>{line}</li>
                  ))}
                </ul>
              </button>
            </div>
          ))}
        </div>
      )}

      <Card style={{ padding: 0, overflow: 'hidden', marginTop: 14 }}>
        <div className="deploy-editor-bar">
          <span>deploy.yaml</span>
          {dirty && <Pill tone="warn">{t('Unsaved')}</Pill>}
          <span className="deploy-editor-actions">
            {(missing || content.trim() === '') && (
              <Btn
                sm
                data-testid="deploy-starter"
                onClick={() => {
                  setContent(starterDeployYaml)
                  setMissing(false)
                }}
              >
                {t('Insert starter')}
              </Btn>
            )}
            <Btn
              sm
              variant="primary"
              data-testid="deploy-save"
              disabled={!dirty || saving || loading}
              onClick={() => void save()}
            >
              {saving ? t('Saving…') : t('Save')}
            </Btn>
          </span>
        </div>
        {loading ? (
          <div className="deploy-loading">{t('Loading…')}</div>
        ) : (
          <Suspense fallback={<div className="deploy-loading">{t('Loading…')}</div>}>
            <CodeEditor path="deploy.yaml" value={content} onChange={setContent} onSave={() => void save()} height="420px" />
          </Suspense>
        )}
      </Card>
    </div>
  )
}
