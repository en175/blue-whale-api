import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'

import { Skeleton } from '@/components/ui/skeleton'

import { getSystemOptions } from '../api'
import { ConfigCard } from './config-card'

/**
 * 通用配置分区：列出所有以 `.config` 结尾的 JSON 配置（如 material_library.config），
 * 以 JSON 编辑器管理。复用 /api/option（RootAuth），保存后配置即时生效。
 */
export function GeneralConfigSection() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()

  const { data, isLoading, isError } = useQuery({
    queryKey: ['general-config-options'],
    queryFn: getSystemOptions,
  })

  // 约定：注册的 JSON 配置收敛在 `xxx.config` 键（见 setting/config 的 Register 模式）。
  const configs = (data?.data ?? []).filter((entry) =>
    entry.key.endsWith('.config')
  )

  const refresh = () =>
    queryClient.invalidateQueries({ queryKey: ['general-config-options'] })

  if (isLoading) {
    return (
      <div className='space-y-3'>
        <Skeleton className='h-48 w-full' />
        <Skeleton className='h-48 w-full' />
      </div>
    )
  }

  if (isError) {
    return (
      <div className='text-muted-foreground flex h-32 items-center justify-center rounded-lg border text-sm'>
        {t('Load failed')}
      </div>
    )
  }

  if (configs.length === 0) {
    return (
      <div className='text-muted-foreground flex h-32 items-center justify-center rounded-lg border text-sm'>
        {t('No configs')}
      </div>
    )
  }

  return (
    <div className='space-y-4'>
      {configs.map((entry) => (
        <ConfigCard key={entry.key} entry={entry} onSaved={refresh} />
      ))}
    </div>
  )
}
