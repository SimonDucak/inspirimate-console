// CSS viewport sizes the page inside the preview sees, not the physical screen.
export const previewPhones = [
  { id: 'iphone-se', label: 'iPhone SE', width: 375, height: 667 },
  { id: 'iphone-16', label: 'iPhone 16', width: 393, height: 852 },
  { id: 'iphone-16-pro', label: 'iPhone 16 Pro', width: 402, height: 874 },
  { id: 'iphone-16-pro-max', label: 'iPhone 16 Pro Max', width: 440, height: 956 },
  { id: 'galaxy-s24', label: 'Galaxy S24', width: 360, height: 780 },
  { id: 'galaxy-s24-ultra', label: 'Galaxy S24 Ultra', width: 384, height: 824 },
  { id: 'pixel-8', label: 'Pixel 8', width: 412, height: 915 },
] as const

export type PreviewPhoneId = (typeof previewPhones)[number]['id']

export function previewPhone(id: string) {
  return previewPhones.find((phone) => phone.id === id)
}

// Shrink a device frame so it fits the canvas. Never enlarge it past 1:1.
export function fitPreviewScale(availableWidth: number, availableHeight: number, width: number, height: number) {
  if (availableWidth <= 0 || availableHeight <= 0 || width <= 0 || height <= 0) return 1
  return Math.min(1, availableWidth / width, availableHeight / height)
}
