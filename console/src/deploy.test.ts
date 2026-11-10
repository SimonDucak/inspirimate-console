import { describe, expect, it } from 'vitest'
import { deployStages, parseDeployYaml, starterDeployYaml } from './deploy'

describe('parseDeployYaml', () => {
  it('reads a build and ssh-style deploy steps', () => {
    const { file, error } = parseDeployYaml(`version: 1
target: pi
build:
  command: pnpm build
  output: dist
steps:
  - copy: dist
    to: /var/www/app
  - run: sudo systemctl restart app
`)
    expect(error).toBeUndefined()
    expect(file.version).toBe(1)
    expect(file.target).toBe('pi')
    expect(file.build).toEqual({ command: 'pnpm build', output: 'dist' })
    expect(file.steps).toEqual([
      { copy: 'dist', to: '/var/www/app' },
      { run: 'sudo systemctl restart app' },
    ])
  })

  it('ignores comments and an empty build command', () => {
    const { file, error } = parseDeployYaml(starterDeployYaml)
    expect(error).toBeUndefined()
    expect(file.build).toBeUndefined()
    expect(file.steps).toHaveLength(2)
  })

  it('rejects a copy step with no destination', () => {
    const { error } = parseDeployYaml('steps:\n  - copy: .\n')
    expect(error).toMatch(/destination/)
  })

  it('rejects a version other than 1', () => {
    const { error } = parseDeployYaml('version: 2\n')
    expect(error).toMatch(/version must be 1/)
  })
})

describe('deployStages', () => {
  it('shows Build only when a command is set, then Deploy when steps exist', () => {
    const both = parseDeployYaml('build:\n  command: flutter build web\n  output: build/web\nsteps:\n  - copy: build/web\n    to: /srv/app\n')
    expect(deployStages(both.file).map((stage) => stage.id)).toEqual(['build', 'deploy'])
    expect(deployStages(both.file)[0].lines).toEqual(['flutter build web', 'output build/web'])

    const deployOnly = parseDeployYaml(starterDeployYaml)
    expect(deployStages(deployOnly.file).map((stage) => stage.id)).toEqual(['deploy'])
    expect(deployStages(deployOnly.file)[0].lines[0]).toBe('pi')

    const empty = parseDeployYaml('')
    expect(deployStages(empty.file)).toEqual([])
  })
})
