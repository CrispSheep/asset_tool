import { ref, computed } from 'vue'
import { GetSetting, SetSetting } from '../../wailsjs/go/main/App'

export type ThemeMode = 'system' | 'dark' | 'light'

const theme = ref<ThemeMode>('system')
const systemDark = ref(false)

let mediaQuery: MediaQueryList | null = null
let initialized = false

function applyTheme() {
  const isDark = theme.value === 'dark' || (theme.value === 'system' && systemDark.value)
  document.documentElement.classList.toggle('dark', isDark)
}

function onSystemChange(e: MediaQueryListEvent) {
  systemDark.value = e.matches
  if (theme.value === 'system') applyTheme()
}

export function useTheme() {
  const actualTheme = computed<'dark' | 'light'>(() => {
    if (theme.value === 'system') return systemDark.value ? 'dark' : 'light'
    return theme.value
  })

  async function init() {
    if (initialized) return
    initialized = true

    // detect system preference
    mediaQuery = window.matchMedia('(prefers-color-scheme: dark)')
    systemDark.value = mediaQuery.matches
    mediaQuery.addEventListener('change', onSystemChange)

    // restore saved preference
    try {
      const saved = await GetSetting('theme')
      if (saved === 'dark' || saved === 'light' || saved === 'system') {
        theme.value = saved
      }
    } catch { /* ignore */ }

    applyTheme()
  }

  async function setTheme(t: ThemeMode) {
    theme.value = t
    applyTheme()
    try {
      await SetSetting('theme', t)
    } catch { /* ignore */ }
  }

  function cycleTheme() {
    const next: Record<ThemeMode, ThemeMode> = {
      system: 'dark',
      dark: 'light',
      light: 'system',
    }
    setTheme(next[theme.value])
  }

  return {
    theme,
    actualTheme,
    init,
    setTheme,
    cycleTheme,
  }
}
