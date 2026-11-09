import { useCallback, useEffect, useRef, useState } from 'react'
import { api, Agent, ModelProvider, Sandbox, TaskSummary } from './api'
import { AgentPicker, BuildIntent, choiceBlocked, defaultChoice, preferredModel, withPreferred } from './AgentPicker'
import {
  isBinaryAsset,
  isUnsupportedMedia,
  maxChatFileBytes,
  maxChatFiles,
  promptWithAttachments,
  splitAttachedPrompt,
  uploadPaths,
} from './chatAttachments'
import { Btn } from './design/kit'
import { Icon } from './design/Icon'
import { AgentMessage } from './AgentMessage'
import { Dialog } from './design/Dialog'

const terminal = (status: string) => ['succeeded', 'failed', 'cancelled', 'error'].includes(status)

export function AgentChat({
  sb,
  onError,
  toast,
  refresh,
  initialBuild,
  consumeBuild,
  goSettings,
  ready = true,
  draft,
  saveDraft,
}: {
  sb: Sandbox | null
  onError: (m: string) => void
  toast: (m: string) => void
  refresh: () => void
  initialBuild?: BuildIntent
  consumeBuild?: () => void
  goSettings: () => void
  ready?: boolean
  draft?: BuildIntent
  saveDraft?: (draft: BuildIntent) => void
}) {
  const [choice, setChoice] = useState(initialBuild || draft || defaultChoice)
  const [providers, setProviders] = useState<ModelProvider[]>([])
  const [agents, setAgents] = useState<Agent[]>([])
  const [loaded, setLoaded] = useState(false)
  const [text, setText] = useState(initialBuild?.prompt || draft?.prompt || '')
  const [files, setFiles] = useState<File[]>([])
  const fileInput = useRef<HTMLInputElement>(null)
  const [history, setHistory] = useState<TaskSummary[]>([])
  const [active, setActive] = useState<TaskSummary | null>(null)
  const [response, setResponse] = useState('')
  const [resolved, setResolved] = useState('')
  const [tool, setTool] = useState('')
  const [cont, setCont] = useState(true)
  const [error, setError] = useState('')
  const [revertTarget, setRevertTarget] = useState<TaskSummary | null>(null)
  const [reverting, setReverting] = useState(false)
  const [issue, setIssue] = useState('')
  const submitting = useRef(false)
  const initialSent = useRef(false)
  const saveDraftRef = useRef(saveDraft)
  saveDraftRef.current = saveDraft
  useEffect(() => {
    saveDraftRef.current?.({ ...choice, prompt: text })
  }, [choice, text])
  const scroll = useRef<HTMLDivElement>(null)
  const sbID = sb?.id
  const currentSandbox = useRef(sbID)
  currentSandbox.current = sbID
  useEffect(() => {
    setHistory([])
    setActive(null)
    setResponse('')
    setResolved('')
    setTool('')
    setFiles([])
    setIssue('')
  }, [sbID])
  const sandboxRunning = sb?.status === 'running' && ready
  const blocked = choiceBlocked(choice, providers, agents)
  const loadOptions = useCallback(() => {
    Promise.all([api.getAgents(), api.listModelProviders()])
      .then(([a, p]) => {
        setAgents(a)
        setProviders(p)
        setLoaded(true)
      })
      .catch((e) => setError((e as Error).message))
  }, [])
  useEffect(loadOptions, [loadOptions])
  const appliedDefault = useRef(false)
  useEffect(() => {
    if (!loaded || initialBuild || appliedDefault.current) return
    appliedDefault.current = true
    setChoice((current) => withPreferred(current, providers))
  }, [loaded, providers, initialBuild])

  const loadHistory = useCallback(async () => {
    if (!sbID) return
    try {
      const tasks = await api.listTasks(sbID)
      if (currentSandbox.current !== sbID) return
      setHistory([...tasks].reverse())
      const inFlight = tasks.find((t) => !terminal(t.status))
      setActive((current) => {
        if (inFlight) return inFlight
        if (current && tasks.some((t) => t.id === current.id && terminal(t.status))) return null
        return current
      })
    } catch {
      /* Preserve the draft and current task across a transient outage. */
    }
  }, [sbID])
  useEffect(() => {
    void loadHistory()
    const timer = setInterval(() => {
      void loadHistory()
    }, 2500)
    return () => clearInterval(timer)
  }, [loadHistory])
  useEffect(() => {
    if (!sbID || !active?.id) return
    setResponse('')
    setResolved('')
    setTool('')
    let output = ''
    const source = new EventSource(api.taskEventsURL(sbID, active.id))
    source.addEventListener('message', (e) => {
      try {
        const m = JSON.parse((e as MessageEvent).data)
        if (m.role === 'agent' && m.text) {
          output += m.text
          setResponse(output)
        }
      } catch {
        /* invalid event */
      }
    })
    source.addEventListener('status', (e) => {
      try {
        const m = JSON.parse((e as MessageEvent).data)
        if (m.model) setResolved(m.model)
      } catch {
        /* invalid event */
      }
    })
    source.addEventListener('tool', (e) => {
      try {
        const m = JSON.parse((e as MessageEvent).data)
        setTool(`${m.name || 'Tool'}${m.path ? ` · ${m.path}` : ''}`)
      } catch {
        /* invalid event */
      }
    })
    source.addEventListener('done', () => {
      source.close()
      void loadHistory()
      refresh()
    })
    // Durable task polling remains active if streaming disconnects.
    source.onerror = () => source.close()
    return () => source.close()
  }, [sbID, active?.id, loadHistory, refresh])
  useEffect(() => {
    scroll.current?.scrollTo?.({ top: scroll.current.scrollHeight, behavior: 'smooth' })
  }, [response, history.length, active?.id])
  const addFiles = (list: FileList | null) => {
    if (!list?.length) return
    const next = [...files]
    let problem = ''
    for (const file of Array.from(list)) {
      if (next.length >= maxChatFiles) {
        problem = 'You can attach up to 2 files.'
        break
      }
      if (file.size > maxChatFileBytes) {
        problem = `${file.name} is larger than 25 MB.`
        continue
      }
      if (file.size === 0) {
        problem = `${file.name} is empty.`
        continue
      }
      if (isUnsupportedMedia(file)) {
        problem = `${file.name} is not a supported file. Attach a text file, or a JPG, PNG, or MP4.`
        continue
      }
      next.push(file)
    }
    setFiles(next)
    setError(problem)
    if (fileInput.current) fileInput.current.value = ''
  }
  const send = async (auto = false) => {
    if (!sbID || !sandboxRunning || !text.trim() || active || blocked || !loaded || submitting.current) return
    submitting.current = true
    const prompt = text.trim()
    const attached = files
    if (auto) {
      initialSent.current = true
      consumeBuild?.()
    }
    setError('')
    setText('')
    setFiles([])
    setResponse('')
    const selected = providers.find((provider) => provider.id === choice.provider)
    const model = choice.model || (selected ? preferredModel(selected) : '')
    try {
      const paths = uploadPaths(attached.map((file) => file.name))
      for (let i = 0; i < attached.length; i++) {
        const file = attached[i]
        if (isBinaryAsset(file)) {
          await api.putWorkspaceFile(sbID, paths[i], file)
          continue
        }
        const content = await file.text()
        if (content.includes('\0')) {
          throw new Error(`${file.name} is not a supported file. Attach a text file, or a JPG, PNG, or MP4.`)
        }
        await api.putWorkspaceFile(sbID, paths[i], content)
      }
      const taskPrompt = promptWithAttachments(prompt, paths)
      setActive({ id: '', prompt: taskPrompt, status: 'starting', can_revert: false })
      const task = await api.submitTask(
        sbID,
        taskPrompt,
        choice.agent,
        model || undefined,
        auto ? false : cont,
        choice.provider || undefined,
      )
      setActive({ id: task.id, prompt: taskPrompt, status: 'running', can_revert: false })
      void loadHistory()
    } catch (e) {
      setText(prompt)
      setFiles(attached)
      setActive(null)
      setError((e as Error).message)
    } finally {
      submitting.current = false
    }
  }
  useEffect(() => {
    if (initialBuild && !initialSent.current && loaded && sandboxRunning && !blocked && !active)
      void send(true)
  }, [initialBuild, loaded, sandboxRunning, blocked, active]) // send is guarded synchronously against duplicate submissions.

  const cancel = async () => {
    if (!sbID || !active?.id) return
    try {
      await api.cancelTask(sbID, active.id)
      toast('Stopping the agent…')
      void loadHistory()
    } catch (e) {
      setError((e as Error).message)
    }
  }
  const revert = async () => {
    if (!sbID || !revertTarget) return
    setReverting(true)
    try {
      await api.revertTask(sbID, revertTarget.id)
      toast('Workspace restored')
      setRevertTarget(null)
      refresh()
      await loadHistory()
    } catch (e) {
      onError((e as Error).message)
    } finally {
      setReverting(false)
    }
  }
  return (
    <section className="agent-chat" aria-label="Build conversation">
      <header className="chat-heading">
        <span className="agent-avatar">
          <Icon name="spark" size={17} />
        </span>
        <div>
          <strong>
            Build with {choice.agent === 'opencode' ? 'OpenCode' : choice.agent === 'pi' ? 'Pi' : 'Claude'}
          </strong>
          <span>{active ? 'Working on your app' : 'Your coding partner'}</span>
        </div>
        <span className={`connection-dot ${active ? 'working' : ''}`} />
      </header>
      <details className="chat-options">
        <summary>
          <Icon name="settings" size={14} /> Agent & model{' '}
          <span>{choice.provider ? providers.find((p) => p.id === choice.provider)?.name : 'Built-in'}</span>
        </summary>
        <AgentPicker
          value={choice}
          onChange={setChoice}
          agents={agents}
          providers={providers}
          disabled={!!active}
        />
        <div className="chat-option-footer">
          <label>
            <input
              type="checkbox"
              checked={cont}
              disabled={!!active}
              onChange={(e) => setCont(e.target.checked)}
              data-testid="task-continue"
            />{' '}
            Continue conversation
          </label>
          <button className="text-button" onClick={goSettings}>
            Connections
          </button>
        </div>
      </details>
      <div className="chat-messages" ref={scroll}>
        {history.length === 0 && !active && (
          <div className="chat-welcome">
            <span className="welcome-icon">
              <Icon name="spark" size={28} />
            </span>
            <h3>Let’s make it yours.</h3>
            <p>
              Describe what to build or change. Attach up to two files, such as a CSV, a logo, or a photo (JPG, PNG, or MP4). Your agent can edit files, install packages and run your app.
            </p>
            <div className="chat-suggestions">
              {[
                'Improve the layout and typography',
                'Explain this project’s structure',
                'Find and fix bugs in this app',
              ].map((p) => (
                <button key={p} onClick={() => setText(p)}>
                  {p}
                  <Icon name="arrow" size={13} />
                </button>
              ))}
            </div>
          </div>
        )}
        {history
          .filter((t) => terminal(t.status))
          .map((t) => (
            <article className="chat-turn" key={t.id}>
              {t.prompt && <UserPrompt prompt={t.prompt} />}
              <div className="agent-message">
                <span className="message-label">
                  <Icon name="spark" size={13} />
                  {t.agent || 'Agent'}
                </span>
                <AgentMessage text={replyText(t)} />
              </div>
              <div className="turn-meta">
                <span className={`task-status ${statusClass(t.status)}`}>{statusLabel(t.status)}</span>
                {t.status === 'failed' && (
                  <IssueNote message={t.error_message} onView={setIssue} />
                )}
                {!!t.files_changed?.length && (
                  <details>
                    <summary>
                      {t.files_changed.length} {t.files_changed.length === 1 ? 'file' : 'files'} changed
                    </summary>
                    <ul>
                      {t.files_changed.map((f) => (
                        <li key={f}>{f}</li>
                      ))}
                    </ul>
                  </details>
                )}
                {t.can_revert && (
                  <button
                    disabled={!sandboxRunning || !!active}
                    className="text-button"
                    onClick={() => setRevertTarget(t)}
                  >
                    <Icon name="history" size={12} /> Restore
                  </button>
                )}
              </div>
            </article>
          ))}
        {active && (
          <article className="chat-turn">
            <UserPrompt prompt={active.prompt || 'Task in progress'} />
            <div className="agent-message">
              <span className="message-label">
                <Icon name="spark" size={13} />
                Agent
              </span>
              <AgentMessage text={response || 'Working on your request…'} />
            </div>
            <div role="status" className="agent-progress">
              <span className="loading-dot" />
              {tool || 'Building in your workspace…'}
            </div>
            {resolved && <p className="field-help">{resolved}</p>}
          </article>
        )}
      </div>
      {error && (
        <div role="alert" className="chat-notice error">
          {error}
          {!loaded && (
            <button className="text-button" onClick={loadOptions}>
              Retry connection
            </button>
          )}
        </div>
      )}
      {blocked && loaded && (
        <div className="chat-notice" data-testid="agent-not-connected">
          Choose a model connection or connect your account.{' '}
          <button className="text-button" onClick={goSettings}>
            Open Settings
          </button>
        </div>
      )}
      {!sandboxRunning && (
        <div className="chat-notice">
          {sb?.status === 'running'
            ? 'Preparing your project…'
            : !sb
              ? 'Create a workspace to start building. Your prompt is kept here.'
              : sb.status === 'stopped'
                ? 'Start the workspace to continue building.'
                : 'Your workspace is starting…'}
        </div>
      )}
      <form
        className="chat-composer"
        onSubmit={(e) => {
          e.preventDefault()
          void send()
        }}
      >
        {files.length > 0 && (
          <ul className="chat-attachments">
            {files.map((file, index) => (
              <li key={`${file.name}-${index}`}>
                <span>{file.name}</span>
                <button
                  type="button"
                  aria-label={`Remove ${file.name}`}
                  onClick={() => setFiles(files.filter((_, i) => i !== index))}
                >
                  <Icon name="close" size={12} />
                </button>
              </li>
            ))}
          </ul>
        )}
        <input
          ref={fileInput}
          type="file"
          multiple
          hidden
          data-testid="task-file"
          onChange={(e) => addFiles(e.target.files)}
        />
        <textarea
          aria-label="Message the agent"
          value={text}
          rows={3}
          onChange={(e) => setText(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === 'Enter' && !e.shiftKey && !e.nativeEvent.isComposing) {
              e.preventDefault()
              void send()
            }
          }}
          placeholder="What should we build next?"
          data-testid="task-prompt"
        />
        <footer>
          <button
            type="button"
            className="attach-button"
            aria-label="Attach a file"
            data-testid="task-attach"
            disabled={!!active || files.length >= maxChatFiles}
            onClick={() => fileInput.current?.click()}
          >
            <Icon name="paperclip" size={16} />
          </button>
          <span>Up to 2 files · text, JPG, PNG, or MP4 · Enter to send</span>
          {active ? (
            <Btn disabled={!active.id} onClick={() => void cancel()} type="button">
              Stop
            </Btn>
          ) : (
            <button
              className="send-button"
              type="submit"
              disabled={!sandboxRunning || !text.trim() || blocked || !loaded}
              data-testid="task-send"
              aria-label="Send message"
            >
              <Icon name="arrow" size={18} />
            </button>
          )}
        </footer>
      </form>
      {issue && (
        <Dialog title="Issue while finishing" close={() => setIssue('')}>
          <p className="task-issue-detail">{issue}</p>
          <div className="dialog-actions">
            <Btn onClick={() => setIssue('')}>Close</Btn>
          </div>
        </Dialog>
      )}
      {revertTarget && (
        <Dialog title="Restore this checkpoint?" close={() => !reverting && setRevertTarget(null)}>
          <p>
            Restore the workspace to before this task. File changes from this task onward will be discarded.
            Installed packages and build caches are kept.
          </p>
          <div className="dialog-actions">
            <Btn disabled={reverting} onClick={() => setRevertTarget(null)}>
              Keep current version
            </Btn>
            <Btn variant="danger" disabled={reverting} onClick={() => void revert()}>
              Restore checkpoint
            </Btn>
          </div>
        </Dialog>
      )}
    </section>
  )
}

function IssueNote({ message, onView }: { message?: string; onView: (issue: string) => void }) {
  const issue = message?.trim() || ''
  return (
    <p className="task-issue" role="status">
      The agent ran into an issue while finishing. The feature may still be implemented correctly — please test it.
      {issue && (
        <button type="button" className="text-button" onClick={() => onView(issue)}>
          View issue
        </button>
      )}
    </p>
  )
}

function statusLabel(status: string) {
  return status === 'failed' ? 'finished' : status
}

function statusClass(status: string) {
  return status === 'failed' ? 'status-finished' : `status-${status}`
}

function replyText(t: TaskSummary) {
  const issue = t.error_message?.trim() || ''
  const reply = t.agent_message?.trim() || ''
  if (t.status === 'failed' && (!reply || reply === issue)) return 'The agent finished this turn.'
  return reply || issue || `Task ${t.status}`
}

function UserPrompt({ prompt }: { prompt: string }) {
  const shown = splitAttachedPrompt(prompt)
  return (
    <div className="user-message">
      {shown.files.length > 0 && (
        <ul className="chat-files">
          {shown.files.map((file) => (
            <li key={file}>{file.replace(/^uploads\//, '')}</li>
          ))}
        </ul>
      )}
      <span className="user-message-text">{shown.text}</span>
    </div>
  )
}
