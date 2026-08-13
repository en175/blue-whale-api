'use client';

import { buttonVariants } from 'fumadocs-ui/components/ui/button';
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from 'fumadocs-ui/components/ui/popover';
import { Check, SunMoon } from 'lucide-react';
import { useTheme } from 'next-themes';
import { useCallback } from 'react';

import { cn } from '@/lib/cn';

const OPTIONS = [
  { value: 'light', label: '浅色' },
  { value: 'dark', label: '深色' },
] as const;

/**
 * 自定义主题切换：与门户首页同款 View Transitions 整页交叉淡化；
 * 不支持的浏览器回退颜色过渡窗（由 ThemeBreathing 兜底）
 */
export function ThemeSwitcher({ locale = 'zh' }: { locale?: string }) {
  const { theme, setTheme } = useTheme();
  const isZh = locale.startsWith('zh');
  const text = isZh
    ? { light: '浅色', dark: '深色', title: '选择主题', aria: '切换主题' }
    : { light: 'Light', dark: 'Dark', title: 'Choose a theme', aria: 'Toggle theme' };

  const change = useCallback(
    (next: string) => {
      const apply = () => setTheme(next);
      const reduce = window.matchMedia(
        '(prefers-reduced-motion: reduce)'
      ).matches;
      const doc = document as Document & {
        startViewTransition?: (cb: () => void) => unknown;
      };
      if (!reduce && typeof doc.startViewTransition === 'function') {
        doc.startViewTransition(apply);
      } else {
        apply();
      }
    },
    [setTheme]
  );

  return (
    <Popover>
      <PopoverTrigger
        aria-label={text.aria}
        className={cn(
          buttonVariants({ color: 'ghost', className: 'gap-1.5 p-1.5' })
        )}
      >
        <SunMoon className="size-4.5" />
      </PopoverTrigger>
      <PopoverContent className="flex flex-col overflow-x-hidden p-0">
        <p className="mb-1 p-2 text-xs font-medium text-fd-muted-foreground">
          {text.title}
        </p>
        {OPTIONS.map((opt) => (
          <button
            key={opt.value}
            type="button"
            className={cn(
              'flex items-center p-2 text-start text-sm',
              theme === opt.value
                ? 'bg-fd-primary/10 font-medium text-fd-primary'
                : 'hover:bg-fd-accent hover:text-fd-accent-foreground'
            )}
            onClick={() => change(opt.value)}
          >
            {opt.value === 'light' ? text.light : text.dark}
            <Check
              size={14}
              className={cn('ms-auto', theme !== opt.value && 'hidden')}
            />
          </button>
        ))}
      </PopoverContent>
    </Popover>
  );
}
