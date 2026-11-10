import { describe, expect, it } from 'vitest'
import {
  isFlutterApp,
  markDownloadable,
  projectFlagIsFlutter,
  projectJsonWithFlutter,
  pubspecIsFlutter,
} from './flutter'

const flutterPubspec = `name: client_app
environment:
  sdk: ^3.8.0
dependencies:
  flutter:
    sdk: flutter
`

describe('Flutter detection', () => {
  it('recognizes a Flutter SDK dependency in pubspec.yaml', () => {
    expect(pubspecIsFlutter(flutterPubspec)).toBe(true)
    expect(pubspecIsFlutter("dependencies:\n  http: ^1.0.0\n")).toBe(false)
    expect(pubspecIsFlutter(null)).toBe(false)
    expect(pubspecIsFlutter("sdk: 'flutter'\n")).toBe(true)
  })

  it('recognizes the project flag and keeps other keys', () => {
    expect(projectFlagIsFlutter(null)).toBe(false)
    expect(projectFlagIsFlutter('{')).toBe(false)
    expect(projectFlagIsFlutter('{"flutter":false}')).toBe(false)
    expect(projectFlagIsFlutter('{"flutter":true}')).toBe(true)
    expect(isFlutterApp(null, '{"flutter":true,"name":"x"}')).toBe(true)
    expect(isFlutterApp(flutterPubspec, '{"flutter":false}')).toBe(true)
    expect(isFlutterApp('name: plain\n', null)).toBe(false)

    const next = projectJsonWithFlutter('{"name":"x"}', true)
    expect(JSON.parse(next)).toEqual({ name: 'x', flutter: true })
    expect(JSON.parse(projectJsonWithFlutter('{', false))).toEqual({ flutter: false })
  })
})

describe('APK download rule', () => {
  it('lets only the latest succeeded build be downloaded', () => {
    const rows = markDownloadable([
      { id: 'old', status: 'succeeded', started_at: '2020-01-01T00:00:00Z' },
      { id: 'bad', status: 'failed', started_at: '2020-01-03T00:00:00Z' },
      { id: 'new', status: 'succeeded', started_at: '2020-01-02T00:00:00Z' },
    ])
    expect(rows.map((row) => [row.id, row.downloadable])).toEqual([
      ['bad', false],
      ['new', true],
      ['old', false],
    ])
  })

  it('keeps the last good APK downloadable while a newer build is running or failed', () => {
    const running = markDownloadable([
      { id: 'good', status: 'succeeded', started_at: '2020-01-01T00:00:00Z' },
      { id: 'now', status: 'running', started_at: '2020-01-02T00:00:00Z' },
    ])
    expect(running.find((row) => row.downloadable)?.id).toBe('good')

    const failed = markDownloadable([
      { id: 'good', status: 'succeeded', started_at: '2020-01-01T00:00:00Z' },
      { id: 'bad', status: 'failed', started_at: '2020-01-02T00:00:00Z' },
    ])
    expect(failed.filter((row) => row.downloadable).map((row) => row.id)).toEqual(['good'])
  })
})
