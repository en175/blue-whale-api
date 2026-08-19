import { Check, Copy } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { TruncatedCell } from '@/components/data-table'
import { Button } from '@/components/ui/button'
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import { copyToClipboard } from '@/lib/copy-to-clipboard'

/** URL 单元格：截断展示，悬浮时最右侧出现复制按钮（参考 API Key 页的复制）。 */
export function UrlCell({ url }: { url: string }) {
  const { t } = useTranslation()
  const [copied, setCopied] = useState(false)

  const handleCopy = async () => {
    const ok = await copyToClipboard(url)
    if (ok) {
      setCopied(true)
      window.setTimeout(() => setCopied(false), 1500)
    }
  }

  return (
    <div className='group flex max-w-full min-w-0 items-center gap-1'>
      <TruncatedCell
        className='max-w-44'
        tooltipContent={url}
      >
        <a
          href={url}
          target='_blank'
          rel='noreferrer'
          className='text-primary hover:underline'
        >
          {url}
        </a>
      </TruncatedCell>
      <Tooltip>
        <TooltipTrigger
          render={
            <Button
              type='button'
              variant='ghost'
              size='icon'
              className='text-muted-foreground size-6 shrink-0 opacity-0 group-hover:opacity-100'
              onClick={handleCopy}
            />
          }
        >
          {copied ? (
            <Check className='size-3.5 text-green-600' />
          ) : (
            <Copy className='size-3.5' />
          )}
        </TooltipTrigger>
        <TooltipContent>
          {copied ? t('Copied!') : t('Copy')}
        </TooltipContent>
      </Tooltip>
    </div>
  )
}
