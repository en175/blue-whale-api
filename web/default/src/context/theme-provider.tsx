/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useRef,
  useState,
} from 'react'

import { getCookie, setCookie, removeCookie } from '@/lib/cookies'

type Theme = 'dark' | 'light' | 'system'
type ResolvedTheme = Exclude<Theme, 'system'>

// 首期默认深色（品牌定稿视觉）；用户选择后尊重存储值
const DEFAULT_THEME = 'dark'
const THEME_COOKIE_NAME = 'vite-ui-theme'
// 与文档站 next-themes 共用同源的 localStorage 键，任一站切换两站同步
const THEME_LOCAL_KEY = 'theme'
const THEME_COOKIE_MAX_AGE = 60 * 60 * 24 * 365 // 1 year
// 首期仅浅/深两档（默认深色）；存量 'system' 存储值视为无效回退默认
const THEMES = new Set<Theme>(['dark', 'light'])

type ThemeProviderProps = {
  children: React.ReactNode
  defaultTheme?: Theme
  storageKey?: string
}

type ThemeProviderState = {
  defaultTheme: Theme
  resolvedTheme: ResolvedTheme
  theme: Theme
  setTheme: (theme: Theme) => void
  resetTheme: () => void
}

const initialState: ThemeProviderState = {
  defaultTheme: DEFAULT_THEME,
  resolvedTheme: 'light',
  theme: DEFAULT_THEME,
  setTheme: () => null,
  resetTheme: () => null,
}

const ThemeContext = createContext<ThemeProviderState>(initialState)

function getSystemTheme(): ResolvedTheme {
  if (typeof window === 'undefined') return 'light'
  return window.matchMedia('(prefers-color-scheme: dark)').matches
    ? 'dark'
    : 'light'
}

function resolveTheme(theme: Theme): ResolvedTheme {
  return theme === 'system' ? getSystemTheme() : theme
}

function getStoredTheme(storageKey: string, fallback: Theme): Theme {
  const storedTheme = getCookie(storageKey) as Theme | undefined
  if (storedTheme && THEMES.has(storedTheme)) return storedTheme
  // cookie 缺失时回退文档站写入的 localStorage（跨站同步）
  try {
    const localTheme = window.localStorage.getItem(THEME_LOCAL_KEY) as
      | Theme
      | null
    if (localTheme && THEMES.has(localTheme)) return localTheme
  } catch {
    // localStorage 不可用时忽略
  }
  return fallback
}

export function ThemeProvider({
  children,
  defaultTheme = DEFAULT_THEME,
  storageKey = THEME_COOKIE_NAME,
  ...props
}: ThemeProviderProps) {
  const [theme, _setTheme] = useState<Theme>(() =>
    getStoredTheme(storageKey, defaultTheme)
  )
  const [resolvedTheme, setResolvedTheme] = useState<ResolvedTheme>(() =>
    resolveTheme(getStoredTheme(storageKey, defaultTheme))
  )
  const lastAppliedTheme = useRef<ResolvedTheme | null>(null)

  useEffect(() => {
    const root = window.document.documentElement
    const mediaQuery = window.matchMedia('(prefers-color-scheme: dark)')

    const applyTheme = () => {
      const nextResolvedTheme = theme === 'system' ? getSystemTheme() : theme
      const changed =
        lastAppliedTheme.current !== null &&
        lastAppliedTheme.current !== nextResolvedTheme
      // 根节点 class 与首页 shadow DOM 主题同一帧内改，保证 View Transition 快照覆盖整页
      const mutate = () => {
        root.classList.remove('light', 'dark')
        root.classList.add(nextResolvedTheme)
        const shadowHome = document
          .querySelector('.custom-home-content')
          ?.shadowRoot?.querySelector<HTMLElement>('.bw-home')
        if (shadowHome) shadowHome.dataset.theme = nextResolvedTheme
      }
      if (changed) {
        const reduce = window.matchMedia(
          '(prefers-reduced-motion: reduce)'
        ).matches
        const doc = document as Document & {
          startViewTransition?: (cb: () => void) => unknown
        }
        if (!reduce && typeof doc.startViewTransition === 'function') {
          // 路线 A：compositor 级整页交叉淡化，零重绘卡帧
          doc.startViewTransition(mutate)
        } else if (!reduce) {
          // 回退：颜色过渡窗（呼吸档）
          root.classList.add('theme-anim')
          window.setTimeout(() => root.classList.remove('theme-anim'), 700)
          mutate()
        } else {
          mutate()
        }
      } else {
        mutate()
      }
      lastAppliedTheme.current = nextResolvedTheme
      setResolvedTheme(nextResolvedTheme)
    }

    applyTheme()

    mediaQuery.addEventListener('change', applyTheme)

    return () => mediaQuery.removeEventListener('change', applyTheme)
  }, [theme])

  const setTheme = useCallback(
    (theme: Theme) => {
      setCookie(storageKey, theme, THEME_COOKIE_MAX_AGE)
      try {
        window.localStorage.setItem(THEME_LOCAL_KEY, theme)
      } catch {
        // localStorage 不可用时忽略
      }
      _setTheme(theme)
    },
    [storageKey]
  )

  const resetTheme = useCallback(() => {
    removeCookie(storageKey)
    try {
      window.localStorage.removeItem(THEME_LOCAL_KEY)
    } catch {
      // localStorage 不可用时忽略
    }
    _setTheme(defaultTheme)
  }, [defaultTheme, storageKey])

  const contextValue = useMemo(
    () => ({
      defaultTheme,
      resolvedTheme,
      resetTheme,
      theme,
      setTheme,
    }),
    [defaultTheme, resolvedTheme, resetTheme, theme, setTheme]
  )

  return (
    <ThemeContext value={contextValue} {...props}>
      {children}
    </ThemeContext>
  )
}

// eslint-disable-next-line react-refresh/only-export-components
export const useTheme = () => {
  const context = useContext(ThemeContext)

  if (!context) throw new Error('useTheme must be used within a ThemeProvider')

  return context
}
