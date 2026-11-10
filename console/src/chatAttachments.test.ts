import { describe, expect, it } from 'vitest'
import { isBinaryAsset, promptWithAttachments, safeUploadName, splitAttachedPrompt, uploadAsText, uploadPaths } from './chatAttachments'

describe('chat attachments', () => {
  it('keeps a safe file name and drops path pieces', () => {
    expect(safeUploadName('nutrients.csv')).toBe('nutrients.csv')
    expect(safeUploadName('../../etc/passwd')).toBe('passwd')
    expect(safeUploadName('my nutrients (1).csv')).toBe('my_nutrients_1_.csv')
    expect(safeUploadName('...')).toBe('file')
  })

  it('gives two files with the same name different paths', () => {
    expect(uploadPaths(['nutrients.csv', 'nutrients.csv'])).toEqual([
      'uploads/nutrients.csv',
      'uploads/nutrients-2.csv',
    ])
  })

  it('puts the file paths in the prompt and keeps the user text for display', () => {
    const sent = promptWithAttachments('Seed the nutrients table from this file.', ['uploads/nutrients.csv'])
    expect(sent).toContain('uploads/nutrients.csv')
    expect(sent).toContain('do not ask the user to paste them')
    expect(splitAttachedPrompt(sent)).toEqual({
      text: 'Seed the nutrients table from this file.',
      files: ['uploads/nutrients.csv'],
    })
    expect(splitAttachedPrompt('Just a message')).toEqual({ text: 'Just a message', files: [] })
  })

  it('treats JPG, PNG, and MP4 as assets and leaves other text files as text', () => {
    expect(isBinaryAsset({ name: 'Logo.PNG', type: '' })).toBe(true)
    expect(isBinaryAsset({ name: 'clip.mp4', type: 'video/mp4' })).toBe(true)
    expect(isBinaryAsset({ name: 'photo', type: 'image/jpeg' })).toBe(true)
    expect(isBinaryAsset({ name: 'notes.txt', type: 'text/plain' })).toBe(false)
    expect(uploadAsText({ name: 'notes.txt', type: 'text/plain' }, 'hello')).toBe(true)
    expect(uploadAsText({ name: 'upload-keystore.jks', type: 'application/octet-stream' }, 'key\0store')).toBe(false)
    expect(uploadAsText({ name: 'clip.webm', type: 'video/webm' }, 'webm')).toBe(false)
    expect(uploadAsText({ name: 'logo.svg', type: 'image/svg+xml' }, '<svg></svg>')).toBe(false)

    const sent = promptWithAttachments('Use this logo in the header.', ['uploads/logo.png'])
    expect(sent).toContain('copied as-is')
    expect(splitAttachedPrompt(sent)).toEqual({
      text: 'Use this logo in the header.',
      files: ['uploads/logo.png'],
    })
  })
})
