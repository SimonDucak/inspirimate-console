import { useCallback, useEffect, useRef, useState } from 'react'
import { api, Sandbox } from './api'
import { Btn, H, Input, Pill } from './design/kit'
import { markDownloadable, type ApkBuildRow } from './flutter'
import { useT } from './i18n'

type Target = 'android' | 'ios' | 'web'

export function DeploymentTab({
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
  const running = sb?.status === 'running'
  const onErrorRef = useRef(onError)
  onErrorRef.current = onError
  const [target, setTarget] = useState<Target>('android')
  const [version, setVersion] = useState('')
  const [builds, setBuilds] = useState<ApkBuildRow[]>([])
  const [loading, setLoading] = useState(false)
  const [starting, setStarting] = useState(false)
  const [openId, setOpenId] = useState<string | null>(null)

  const load = useCallback(async () => {
    if (!sandboxId) {
      setBuilds([])
      return
    }
    const rows = await api.listApkBuilds(sandboxId)
    setBuilds(markDownloadable(rows))
  }, [sandboxId])

  useEffect(() => {
    if (!sandboxId) return
    let cancel = false
    setLoading(true)
    load()
      .catch((e) => {
        if (!cancel) onErrorRef.current((e as Error).message)
      })
      .finally(() => {
        if (!cancel) setLoading(false)
      })
    return () => {
      cancel = true
    }
  }, [sandboxId, load])

  const busy = builds.some((build) => build.status === 'running')
  useEffect(() => {
    if (!sandboxId || !busy) return
    const timer = setInterval(() => {
      load().catch((e) => onErrorRef.current((e as Error).message))
    }, 3000)
    return () => clearInterval(timer)
  }, [sandboxId, busy, load])

  const start = async () => {
    if (!sandboxId || busy || !running) return
    setStarting(true)
    try {
      const label = version.trim()
      const created = await api.startApkBuild(sandboxId, label || undefined)
      setVersion('')
      if (created?.id) setOpenId(created.id)
      toast(t('Pipeline started'))
      await load()
    } catch (e) {
      onError((e as Error).message)
    } finally {
      setStarting(false)
    }
  }

  return (
    <div className="deployment-tab" data-testid="deployment-tab">
      <H>{t('Deployment')}</H>
      <div className="deployment-subtabs" role="tablist" aria-label={t('Deployment')}>
        <button
          type="button"
          role="tab"
          aria-selected={target === 'android'}
          className={target === 'android' ? 'active' : ''}
          onClick={() => setTarget('android')}
        >
          {t('Android')}
        </button>
        <button type="button" role="tab" disabled title={t('Coming soon')}>
          {t('iOS')} · {t('Coming soon')}
        </button>
        <button type="button" role="tab" disabled title={t('Coming soon')}>
          {t('Web')} · {t('Coming soon')}
        </button>
      </div>

      {target === 'android' && (
        <div role="tabpanel">
          <p className="deploy-help">
            {t(
              'Run a pipeline for this Android app. Today only the APK build runs. Firebase and user notification are shown as the next stages. Only the latest successful APK can be downloaded.',
            )}
          </p>
          {!sandboxId ? (
            <p className="deploy-help">{t('Create a workspace to build an APK.')}</p>
          ) : (
            <>
              <label className="deployment-version">
                <span>{t('Version')}</span>
                <Input
                  value={version}
                  placeholder="1.0.0"
                  data-testid="apk-version"
                  disabled={busy || starting}
                  onChange={(e) => setVersion(e.target.value)}
                />
              </label>
              <p className="deploy-help">
                {t('Optional. Stored on the build only — not written into the app.')}
              </p>
              <div className="deployment-actions">
                <Btn
                  variant="primary"
                  data-testid="apk-build"
                  disabled={!running || busy || starting}
                  title={running ? '' : t('Start the workspace to run a pipeline.')}
                  onClick={() => void start()}
                >
                  {busy || starting ? t('Running…') : t('Run pipeline')}
                </Btn>
              </div>
              {loading && builds.length === 0 ? (
                <p className="deploy-help">{t('Loading…')}</p>
              ) : builds.length === 0 ? (
                <p className="deploy-empty" data-testid="apk-empty">
                  {t('No pipelines yet.')}
                </p>
              ) : (
                <ul className="pipeline-list" data-testid="apk-history">
                  {builds.map((build) => (
                    <PipelineRow
                      key={build.id}
                      build={build}
                      sandboxId={sandboxId}
                      open={openId === build.id}
                      onToggle={() => setOpenId((current) => (current === build.id ? null : build.id))}
                    />
                  ))}
                </ul>
              )}
            </>
          )}
        </div>
      )}
    </div>
  )
}

function PipelineRow({
  build,
  sandboxId,
  open,
  onToggle,
}: {
  build: ApkBuildRow
  sandboxId: string
  open: boolean
  onToggle: () => void
}) {
  const t = useT()
  return (
    <li>
      <button
        type="button"
        className="pipeline-row"
        aria-expanded={open}
        data-testid={`pipeline-${build.id}`}
        onClick={onToggle}
      >
        <span className="pipeline-chevron" aria-hidden="true">
          {open ? '▾' : '▸'}
        </span>
        <time dateTime={build.started_at}>{new Date(build.started_at).toLocaleString()}</time>
        {build.version ? <span className="deployment-version-label">{build.version}</span> : null}
        <Pill tone={pipelineTone(build.status)}>{t(pipelineLabel(build.status))}</Pill>
      </button>
      {open ? <PipelineStages build={build} sandboxId={sandboxId} /> : null}
    </li>
  )
}

function PipelineStages({ build, sandboxId }: { build: ApkBuildRow; sandboxId: string }) {
  const t = useT()
  const later = laterStage(build.status)
  return (
    <ol className="pipeline-stages">
      <li className="pipeline-stage">
        <StageMark n={1} last={false} />
        <div className="pipeline-stage-body">
          <div className="pipeline-stage-head">
            <span>{t('Build APK')}</span>
            <Pill tone={pipelineTone(build.status)}>{t(statusLabel(build.status))}</Pill>
          </div>
          {build.log ? <pre className="deployment-log">{build.log}</pre> : null}
          {!build.log && build.status === 'running' ? (
            <p className="deploy-help">{t('The log appears when this stage finishes.')}</p>
          ) : null}
          {build.downloadable ? (
            <a href={api.apkDownloadUrl(sandboxId)} download="app-release.apk" data-testid="apk-download">
              {t('Download APK')}
            </a>
          ) : null}
        </div>
      </li>
      <li className="pipeline-stage">
        <StageMark n={2} last={false} />
        <div className="pipeline-stage-body">
          <div className="pipeline-stage-head">
            <span>{t('Send to Firebase App Distribution')}</span>
            <Pill tone={later.tone}>{t(later.label)}</Pill>
          </div>
          <p className="deploy-help">{t(later.firebase)}</p>
        </div>
      </li>
      <li className="pipeline-stage">
        <StageMark n={3} last />
        <div className="pipeline-stage-body">
          <div className="pipeline-stage-head">
            <span>{t('Notify users')}</span>
            <Pill tone={later.tone}>{t(later.label)}</Pill>
          </div>
          <p className="deploy-help">{t(later.notify)}</p>
        </div>
      </li>
    </ol>
  )
}

function StageMark({ n, last }: { n: number; last: boolean }) {
  return (
    <div className="pipeline-stage-mark" aria-hidden="true">
      <span className="pipeline-stage-index">{n}</span>
      {last ? null : <span className="pipeline-stage-line" />}
    </div>
  )
}

function pipelineTone(status: string): 'good' | 'warn' | 'bad' | 'neutral' {
  if (status === 'succeeded') return 'good'
  if (status === 'failed') return 'bad'
  if (status === 'running') return 'warn'
  return 'neutral'
}

function pipelineLabel(status: string): string {
  if (status === 'succeeded') return 'APK ready'
  return statusLabel(status)
}

function statusLabel(status: string): string {
  if (status === 'succeeded') return 'Succeeded'
  if (status === 'failed') return 'Failed'
  if (status === 'running') return 'Running'
  return status
}

function laterStage(status: string): { tone: 'neutral'; label: string; firebase: string; notify: string } {
  if (status === 'failed') {
    return {
      tone: 'neutral',
      label: 'Skipped',
      firebase: 'Skipped because the APK build failed.',
      notify: 'Skipped because the APK build failed.',
    }
  }
  if (status === 'running') {
    return {
      tone: 'neutral',
      label: 'Waiting',
      firebase: 'Waiting for the APK.',
      notify: 'Waiting for Firebase.',
    }
  }
  return {
    tone: 'neutral',
    label: 'Not connected yet',
    firebase: 'This will upload the APK and return a tester link.',
    notify: 'This will tell the invited testers.',
  }
}
