import { useCallback, useState } from 'react'

/**
 * A piece of viewer preference (font size, theme, and so on) remembered in
 * local storage. Private browsing and locked-down devices are handled by
 * quietly falling back to session-only state.
 */
export function useSetting<T extends string | number | boolean>(
  key: string,
  fallback: T,
): [T, (value: T) => void] {
  const [value, setValue] = useState<T>(() => {
    try {
      const raw = window.localStorage.getItem(`ears.${key}`)
      if (raw === null) return fallback
      return JSON.parse(raw) as T
    } catch {
      return fallback
    }
  })

  const update = useCallback(
    (next: T) => {
      setValue(next)
      try {
        window.localStorage.setItem(`ears.${key}`, JSON.stringify(next))
      } catch {
        /* nothing to be done about it */
      }
    },
    [key],
  )

  return [value, update]
}
