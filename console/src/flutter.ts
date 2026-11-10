// Flutter project detection and APK history rules for the Deployment tab.
//
// An app is Flutter when pubspec.yaml depends on the Flutter SDK, or when
// the user turns the project flag on. The flag lives in .sandboxd/project.json
// so a repo whose pubspec is not at the workspace root can still opt in.

export const projectJsonPath = '.sandboxd/project.json'

export type ApkBuildStatus = 'running' | 'succeeded' | 'failed'

export type ApkBuildRow = {
  id: string
  status: ApkBuildStatus | string
  started_at: string
  version?: string
  finished_at?: string
  log?: string
  downloadable?: boolean
}

export function pubspecIsFlutter(text: string | null | undefined): boolean {
  if (!text) return false
  return /(^|\n)\s*sdk:\s*['"]?flutter['"]?\s*(\n|$)/.test(text)
}

export function projectFlagIsFlutter(text: string | null | undefined): boolean {
  if (!text || !text.trim()) return false
  try {
    const parsed = JSON.parse(text) as { flutter?: unknown }
    return parsed?.flutter === true
  } catch {
    return false
  }
}

export function isFlutterApp(pubspec: string | null | undefined, projectJson: string | null | undefined): boolean {
  return pubspecIsFlutter(pubspec) || projectFlagIsFlutter(projectJson)
}

// projectJsonWithFlutter keeps any other keys in the project file and sets flutter.
export function projectJsonWithFlutter(text: string | null | undefined, on: boolean): string {
  let obj: Record<string, unknown> = {}
  if (text && text.trim()) {
    try {
      const parsed = JSON.parse(text) as unknown
      if (parsed && typeof parsed === 'object' && !Array.isArray(parsed)) {
        obj = parsed as Record<string, unknown>
      }
    } catch {
      /* replace an unreadable file */
    }
  }
  obj.flutter = on
  return JSON.stringify(obj, null, 2) + '\n'
}

// markDownloadable keeps history rows and allows a download on the newest
// succeeded build only. Newer failures stay in the list with no file.
export function markDownloadable<T extends { status: string; started_at: string }>(
  builds: T[],
): Array<T & { downloadable: boolean }> {
  const sorted = [...builds].sort((a, b) => {
    if (a.started_at === b.started_at) return 0
    return a.started_at < b.started_at ? 1 : -1
  })
  let seen = false
  return sorted.map((build) => {
    const downloadable = !seen && build.status === 'succeeded'
    if (downloadable) seen = true
    return { ...build, downloadable }
  })
}
