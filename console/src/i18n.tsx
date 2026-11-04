import { useEffect, useState } from 'react'
import { sk } from './i18n.sk'

export type Locale = 'en' | 'sk'
export type Vars = Record<string, string | number>

const storageKey = 'nakopavac-locale'

function storedLocale(): Locale {
  try {
    return localStorage.getItem(storageKey) === 'sk' ? 'sk' : 'en'
  } catch {
    return 'en'
  }
}

let locale: Locale = typeof localStorage === 'undefined' ? 'en' : storedLocale()
const listeners = new Set<() => void>()

export function currentLocale(): Locale {
  return locale
}

export function applyLocale(value?: string) {
  const next: Locale = value === 'sk' ? 'sk' : 'en'
  try {
    localStorage.setItem(storageKey, next)
  } catch {
    /* private mode */
  }
  if (typeof document !== 'undefined') document.documentElement.lang = next
  if (next === locale) return
  locale = next
  listeners.forEach((fn) => fn())
}

export function translate(message: string, vars?: Vars): string {
  let out = locale === 'sk' ? sk[message] || message : message
  if (vars) {
    for (const [key, value] of Object.entries(vars)) out = out.split(`{${key}}`).join(String(value))
  }
  return out
}

export function useT() {
  const [, bump] = useState(0)
  useEffect(() => {
    const fn = () => bump((n) => n + 1)
    listeners.add(fn)
    return () => {
      listeners.delete(fn)
    }
  }, [])
  return translate
}
