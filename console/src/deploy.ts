// deploy.yaml — per-project publish description.
//
// The preview runtime stays in sandbox.yaml. This file says how to publish:
// an optional build, then deploy steps (copy and remote commands). Host,
// username, and keys are not stored here.

export type DeployStep = {
  copy?: string
  to?: string
  run?: string
}

export type DeployFile = {
  version?: number
  target?: string
  build?: { command?: string; output?: string }
  steps: DeployStep[]
}

export type DeployStageId = 'build' | 'deploy'

export type DeployStage = {
  id: DeployStageId
  label: 'Build' | 'Deploy'
  lines: string[]
}

export const starterDeployYaml = `version: 1
target: pi
# Uncomment to add a Build stage:
# build:
#   command: pnpm build
#   output: dist
steps:
  - copy: .
    to: /home/simon/apps/hello
  - run: python3 main.py
`

type Section = 'root' | 'build' | 'steps' | 'skip'

function unquote(value: string): string {
  const trimmed = value.trim()
  if (
    (trimmed.startsWith('"') && trimmed.endsWith('"') && trimmed.length >= 2) ||
    (trimmed.startsWith("'") && trimmed.endsWith("'") && trimmed.length >= 2)
  ) {
    return trimmed.slice(1, -1)
  }
  return trimmed
}

function splitKey(body: string): [string, string] | null {
  const colon = body.indexOf(':')
  if (colon <= 0) return null
  return [body.slice(0, colon).trim(), unquote(body.slice(colon + 1))]
}

function assignStep(step: DeployStep, key: string, value: string) {
  if (key === 'copy') step.copy = value
  else if (key === 'to') step.to = value
  else if (key === 'run') step.run = value
}

// parseDeployYaml reads the small deploy.yaml subset. Unknown keys are ignored
// so later fields can be added without breaking existing files.
export function parseDeployYaml(text: string): { file: DeployFile; error?: string } {
  const file: DeployFile = { steps: [] }
  let section: Section = 'root'
  let step: DeployStep | null = null
  let stepLine = 0
  const seen = new Set<string>()

  const flush = (): string | undefined => {
    if (!step) return
    if (!step.copy && !step.run) return `each step needs copy or run (line ${stepLine})`
    if (step.copy && !step.to?.trim()) return `a copy step needs a destination (line ${stepLine})`
    file.steps.push(step)
    step = null
    return
  }

  const lines = text.replace(/\t/g, '  ').split(/\r?\n/)
  for (let i = 0; i < lines.length; i++) {
    const raw = lines[i]
    if (/^\s*$/.test(raw) || /^\s*#/.test(raw)) continue
    const indent = raw.match(/^ */)?.[0].length ?? 0
    const body = raw.trim()
    const lineNo = i + 1

    if (indent === 0) {
      const flushError = flush()
      if (flushError) return { file, error: flushError }
      section = 'root'
      const kv = splitKey(body)
      if (!kv) return { file, error: `could not read line ${lineNo}` }
      const [key, value] = kv
      if (seen.has(key)) return { file, error: `duplicate ${key} (line ${lineNo})` }
      seen.add(key)
      if (key === 'version') {
        if (value !== '1') return { file, error: `version must be 1 (line ${lineNo})` }
        file.version = 1
      } else if (key === 'target') {
        file.target = value
      } else if (key === 'build') {
        if (value) return { file, error: `build must be a block (line ${lineNo})` }
        file.build = file.build || {}
        section = 'build'
      } else if (key === 'steps') {
        if (value) return { file, error: `steps must be a list (line ${lineNo})` }
        section = 'steps'
      } else {
        section = value ? 'root' : 'skip'
      }
      continue
    }

    if (section === 'skip' || section === 'root') continue

    if (section === 'build') {
      const kv = splitKey(body)
      if (!kv) return { file, error: `could not read line ${lineNo}` }
      file.build = file.build || {}
      if (kv[0] === 'command') file.build.command = kv[1]
      else if (kv[0] === 'output') file.build.output = kv[1]
      continue
    }

    if (body.startsWith('- ')) {
      const flushError = flush()
      if (flushError) return { file, error: flushError }
      const kv = splitKey(body.slice(2).trim())
      if (!kv) return { file, error: `could not read line ${lineNo}` }
      step = {}
      stepLine = lineNo
      assignStep(step, kv[0], kv[1])
      continue
    }
    if (!step) return { file, error: `step field is outside a step (line ${lineNo})` }
    const kv = splitKey(body)
    if (!kv) return { file, error: `could not read line ${lineNo}` }
    assignStep(step, kv[0], kv[1])
  }

  const flushError = flush()
  if (flushError) return { file, error: flushError }
  return { file }
}

// deployStages turns a parsed file into the pipeline strip. A Build stage
// exists only when build.command is set. A Deploy stage exists only when
// there is at least one step.
export function deployStages(file: DeployFile): DeployStage[] {
  const stages: DeployStage[] = []
  const command = file.build?.command?.trim()
  if (command) {
    const lines = [command]
    const output = file.build?.output?.trim()
    if (output) lines.push(`output ${output}`)
    stages.push({ id: 'build', label: 'Build', lines })
  }
  if (file.steps.length > 0) {
    const lines: string[] = []
    const target = file.target?.trim()
    if (target) lines.push(target)
    for (const step of file.steps) {
      if (step.copy) lines.push(`copy ${step.copy} → ${step.to}`)
      if (step.run) lines.push(step.run)
    }
    stages.push({ id: 'deploy', label: 'Deploy', lines })
  }
  return stages
}
