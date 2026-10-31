import { describe, expect, it } from 'vitest'
import { fitPreviewScale, previewPhone, previewPhones } from './previewDevices'

describe('preview phone sizes', () => {
  it('includes iPhone and Samsung sizes and does not enlarge a frame that already fits', () => {
    expect(previewPhones.map((phone) => phone.label)).toEqual([
      'iPhone SE',
      'iPhone 16',
      'iPhone 16 Pro',
      'iPhone 16 Pro Max',
      'Galaxy S24',
      'Galaxy S24 Ultra',
      'Pixel 8',
    ])
    expect(previewPhone('iphone-16')).toMatchObject({ width: 393, height: 852 })
    expect(previewPhone('galaxy-s24')).toMatchObject({ width: 360, height: 780 })
    expect(fitPreviewScale(800, 1000, 393, 852)).toBe(1)
    expect(fitPreviewScale(300, 800, 393, 852)).toBeCloseTo(300 / 393)
    expect(fitPreviewScale(500, 400, 393, 852)).toBeCloseTo(400 / 852)
  })
})
