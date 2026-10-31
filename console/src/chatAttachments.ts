export const maxChatFiles = 2
export const maxChatFileBytes = 25 * 1024 * 1024

const attachmentLead = 'Attached project files (read these paths from the project, do not ask the user to paste them):'
const attachmentMark = '\n\n---\n\n'

export function safeUploadName(name: string): string {
  const base = name.split(/[/\\]/).pop() || 'file'
  const cleaned = base.replace(/[^A-Za-z0-9._-]+/g, '_').replace(/^\.+/, '').slice(0, 80)
  return cleaned || 'file'
}

export function uploadPaths(names: string[]): string[] {
  const used = new Set<string>()
  return names.map((name) => {
    const safe = safeUploadName(name)
    const dot = safe.lastIndexOf('.')
    const stem = dot > 0 ? safe.slice(0, dot) : safe
    const ext = dot > 0 ? safe.slice(dot) : ''
    let candidate = safe
    let n = 2
    while (used.has(candidate)) {
      candidate = `${stem}-${n}${ext}`
      n++
    }
    used.add(candidate)
    return `uploads/${candidate}`
  })
}

export function promptWithAttachments(prompt: string, paths: string[]): string {
  if (paths.length === 0) return prompt
  const list = paths.map((path) => `- ${path}`).join('\n')
  return `${attachmentLead}\n${list}${attachmentMark}${prompt}`
}

export function splitAttachedPrompt(prompt: string): { text: string; files: string[] } {
  const lead = `${attachmentLead}\n`
  if (!prompt.startsWith(lead)) return { text: prompt, files: [] }
  const rest = prompt.slice(lead.length)
  const at = rest.indexOf(attachmentMark)
  if (at < 0) return { text: prompt, files: [] }
  const files = rest
    .slice(0, at)
    .split('\n')
    .map((line) => line.replace(/^- /, '').trim())
    .filter(Boolean)
  return { text: rest.slice(at + attachmentMark.length), files }
}
