export const maxChatFiles = 2
export const maxChatFileBytes = 25 * 1024 * 1024

const attachmentLead = 'Attached project files (read these paths from the project, do not ask the user to paste them):'
const attachmentLeadWithBytes =
  'Attached project files (read these paths from the project, do not ask the user to paste them). Files saved as bytes must be copied as-is to the place the user names. Do not open those as text. JPG, PNG, and MP4 may also be used as site assets. Read text files as text:'
const attachmentMark = '\n\n---\n\n'
const binaryAssetExtensions = new Set(['jpg', 'jpeg', 'png', 'mp4'])
const binaryAssetTypes = new Set(['image/jpeg', 'image/png', 'video/mp4'])

export function isBinaryAsset(file: { name: string; type?: string }): boolean {
  return binaryAssetExtensions.has(fileExtension(file.name)) || binaryAssetTypes.has((file.type || '').toLowerCase())
}

// Text stays text so the agent can read a CSV or note. Everything else,
// including a keystore, is stored as the original bytes.
export function uploadAsText(file: { name: string; type?: string }, decoded: string): boolean {
  if (decoded.includes('\0')) return false
  if (isBinaryAsset(file)) return false
  const type = (file.type || '').toLowerCase()
  if (type.startsWith('image/') || type.startsWith('video/') || type.startsWith('audio/')) return false
  return true
}

function fileExtension(name: string): string {
  const base = name.split(/[/\\]/).pop() || ''
  const dot = base.lastIndexOf('.')
  return dot > 0 ? base.slice(dot + 1).toLowerCase() : ''
}

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

export function promptWithAttachments(prompt: string, paths: string[], rawPaths: string[] = []): string {
  if (paths.length === 0) return prompt
  const raw = new Set(rawPaths)
  const lead = paths.some((path) => raw.has(path) || isBinaryAsset({ name: path })) ? attachmentLeadWithBytes : attachmentLead
  const list = paths.map((path) => `- ${path}`).join('\n')
  return `${lead}\n${list}${attachmentMark}${prompt}`
}

export function splitAttachedPrompt(prompt: string): { text: string; files: string[] } {
  const lead = [attachmentLeadWithBytes, attachmentLead].find((item) => prompt.startsWith(`${item}\n`))
  if (!lead) return { text: prompt, files: [] }
  const rest = prompt.slice(lead.length + 1)
  const at = rest.indexOf(attachmentMark)
  if (at < 0) return { text: prompt, files: [] }
  const files = rest
    .slice(0, at)
    .split('\n')
    .map((line) => line.replace(/^- /, '').trim())
    .filter(Boolean)
  return { text: rest.slice(at + attachmentMark.length), files }
}
