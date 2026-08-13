'use client';

import { RootProvider } from 'fumadocs-ui/provider/next';
import { useTheme } from 'next-themes';
import { useEffect, useRef, type ReactNode } from 'react';

/** 主题变化时开颜色过渡窗，让深浅切换有呼吸感 */
function ThemeBreathing() {
  const { resolvedTheme } = useTheme();
  const last = useRef<string | null>(null);

  useEffect(() => {
    if (!resolvedTheme) return;
    if (last.current && last.current !== resolvedTheme) {
      // 支持 View Transitions 时由切换按钮走整页交叉淡化，这里仅对老浏览器兼底颜色过渡窗
      const doc = document as Document & {
        startViewTransition?: (cb: () => void) => unknown;
      };
      const reduce = window.matchMedia(
        '(prefers-reduced-motion: reduce)'
      ).matches;
      if (!reduce && typeof doc.startViewTransition !== 'function') {
        const root = document.documentElement;
        root.classList.add('theme-anim');
        const id = window.setTimeout(
          () => root.classList.remove('theme-anim'),
          700
        );
        last.current = resolvedTheme;
        return () => window.clearTimeout(id);
      }
    }
    last.current = resolvedTheme;
  }, [resolvedTheme]);

  return null;
}

export function Provider({
  children,
  i18n,
}: {
  children: ReactNode;
  i18n: Parameters<typeof RootProvider>[0]['i18n'];
  lang?: string;
}) {
  return (
    <RootProvider
      i18n={i18n}
      theme={{
        defaultTheme: 'dark',
        // 首期仅浅/深两档，与控制台一致；存量 system 值由 next-themes 回退默认
        enableSystem: false,
        // 与控制台共用 localStorage 键 'theme'（next-themes 默认键），任一站切换两站同步
        storageKey: 'theme',
        // 关闭“切换时禁用过渡”，配合 theme-anim 过渡窗实现呼吸感
        disableTransitionOnChange: false,
      }}
    >
      <ThemeBreathing />
      {children}
    </RootProvider>
  );
}
