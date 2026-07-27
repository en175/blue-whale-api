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
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'

import {
  EXCLUDED_GROUPS,
  VIEW_MODES,
  type ViewMode,
} from '../constants'
import type { PricingModel, PricingVendor, TokenUnit } from '../types'
import { DirectoryTable } from './directory-table'
import { getVendorCatalog, groupModelsByVendor, VendorTabs } from './vendor-tabs'

// ----------------------------------------------------------------------------
// DIRECTORY 视图容器：厂商 Tab + 分组/标签/排序 + 精简价表(分页) + 其他计费
// ----------------------------------------------------------------------------

const PAGE_SIZE = 10

type TagFilter = 'all' | 'text' | 'image' | 'video'
type SortBy = 'latest' | 'name'

/** 按 endpoint + 模型名归类：文字 / 图片 / 视频 */
function modelTag(model: PricingModel): 'text' | 'image' | 'video' {
  const eps = model.supported_endpoint_types || []
  const name = model.model_name.toLowerCase()
  if (
    eps.includes('image-generation') ||
    /seedream|image|sdxl|flux|dall|stable-diffusion/i.test(name)
  ) {
    return 'image'
  }
  if (
    eps.includes('openai-video') ||
    /seedance|video|wan|cogvideo|kling/i.test(name)
  ) {
    return 'video'
  }
  return 'text'
}

/** 提取模型名尾部 YYMMDD 日期，用于「新优先」排序；无日期返回 0。 */
function dateSuffix(modelName: string): number {
  const m = modelName.match(/(\d{6})$/)
  return m ? parseInt(m[1], 10) : 0
}

export interface ModelDirectoryProps {
  models: PricingModel[]
  vendors: PricingVendor[]
  groupRatio: Record<string, number>
  usableGroup: Record<string, { desc: string; ratio: number }>
  priceRate: number
  usdExchangeRate: number
  tokenUnit: TokenUnit
  onModelClick: (modelName: string) => void
  onViewModeChange?: (mode: ViewMode) => void
}

export function ModelDirectory({
  models,
  vendors,
  groupRatio,
  usableGroup,
  priceRate,
  usdExchangeRate,
  tokenUnit,
  onModelClick,
  onViewModeChange,
}: ModelDirectoryProps) {
  const { t } = useTranslation()
  const catalog = getVendorCatalog()
  const grouped = useMemo(
    () => groupModelsByVendor(models, vendors),
    [models, vendors]
  )

  const firstEnabledKey = useMemo(() => {
    const found = catalog.find((item) => (grouped[item.key]?.length || 0) > 0)
    return found?.key || catalog[0].key
  }, [catalog, grouped])

  const [vendorKey, setVendorKey] = useState(firstEnabledKey)
  const [tagFilter, setTagFilter] = useState<TagFilter>('all')
  const [sortBy, setSortBy] = useState<SortBy>('latest')
  const [page, setPage] = useState(1)

  const availableGroups = useMemo(
    () =>
      Object.keys(usableGroup || {}).filter(
        (g) => !EXCLUDED_GROUPS.includes(g)
      ),
    [usableGroup]
  )
  const [selectedGroup, setSelectedGroup] = useState<string>(
    availableGroups[0] || ''
  )
  const effectiveGroup = selectedGroup || availableGroups[0] || ''

  const vendorModels = useMemo(
    () => grouped[vendorKey] || [],
    [grouped, vendorKey]
  )

  // 该厂商实际存在的标签(只显示有意义的)
  const availableTags = useMemo(() => {
    const tags = new Set(vendorModels.map(modelTag))
    return (['text', 'image', 'video'] as const).filter((tg) => tags.has(tg))
  }, [vendorModels])

  // 过滤链:标签 → 排序 → 分页（按次/按秒模型与 token 模型同列展示，不再单列「其他计费」）
  const tagged = useMemo(
    () =>
      tagFilter === 'all'
        ? vendorModels
        : vendorModels.filter((m) => modelTag(m) === tagFilter),
    [vendorModels, tagFilter]
  )
  const sorted = useMemo(() => {
    const arr = [...tagged]
    arr.sort((a, b) => {
      if (sortBy === 'latest') {
        return dateSuffix(b.model_name) - dateSuffix(a.model_name)
      }
      return a.model_name.localeCompare(b.model_name)
    })
    return arr
  }, [tagged, sortBy])

  const pageCount = Math.max(1, Math.ceil(sorted.length / PAGE_SIZE))
  const currentPage = Math.min(page, pageCount)
  const paged = useMemo(
    () =>
      sorted.slice((currentPage - 1) * PAGE_SIZE, currentPage * PAGE_SIZE),
    [sorted, currentPage]
  )

  const resetPage = () => setPage(1)
  const unitLabel = tokenUnit === 'K' ? '1K' : '1M'
  const vendorName =
    catalog.find((item) => item.key === vendorKey)?.name || ''

  return (
    <div className='mx-auto w-full max-w-[1240px] px-6 pb-20 pt-10'>
      {onViewModeChange && (
        <div className='flex justify-end pb-3'>
          <button
            type='button'
            onClick={() => onViewModeChange(VIEW_MODES.TABLE)}
            className='text-muted-foreground hover:text-foreground text-xs transition-colors'
          >
            ← {t('Table view')}
          </button>
        </div>
      )}
      <VendorTabs
        models={models}
        vendors={vendors}
        selectedKey={vendorKey}
        onSelect={(k) => {
          setVendorKey(k)
          resetPage()
        }}
      />

      <div className='flex flex-col gap-5 py-8 sm:flex-row sm:items-end sm:justify-between'>
        <div>
          <h2 className='text-[22px] font-semibold tracking-tight'>
            {vendorName}
          </h2>
          <p className='text-muted-foreground mt-2 text-sm'>{t('Price list')}</p>
          <p className='text-muted-foreground/70 mt-1 text-sm'>
            {t('Select a group to view RMB prices for each model.')}
          </p>
        </div>
        <div className='text-muted-foreground flex flex-wrap items-center gap-3 text-sm'>
          {availableGroups.length > 0 && (
            <label className='flex items-center gap-2'>
              <span>{t('Price group')}</span>
              <select
                value={effectiveGroup}
                onChange={(e) => setSelectedGroup(e.target.value)}
                className='bg-background min-w-[140px] rounded-lg border px-3 py-2 text-sm font-medium text-foreground outline-none'
              >
                {availableGroups.map((g) => (
                  <option key={g} value={g}>
                    {g} · {groupRatio[g] ?? 1}x
                  </option>
                ))}
              </select>
            </label>
          )}
          {availableTags.length > 1 && (
            <label className='flex items-center gap-2'>
              <span>{t('Tag')}</span>
              <select
                value={tagFilter}
                onChange={(e) => {
                  setTagFilter(e.target.value as TagFilter)
                  resetPage()
                }}
                className='bg-background rounded-lg border px-3 py-2 text-sm font-medium text-foreground outline-none'
              >
                <option value='all'>{t('All')}</option>
                {availableTags.map((tg) => (
                  <option key={tg} value={tg}>
                    {tg === 'text'
                      ? t('Text')
                      : tg === 'image'
                        ? t('Image')
                        : t('Video')}
                  </option>
                ))}
              </select>
            </label>
          )}
          <label className='flex items-center gap-2'>
            <span>{t('Sort')}</span>
            <select
              value={sortBy}
              onChange={(e) => setSortBy(e.target.value as SortBy)}
              className='bg-background rounded-lg border px-3 py-2 text-sm font-medium text-foreground outline-none'
            >
              <option value='latest'>{t('Latest')}</option>
              <option value='name'>{t('Name')}</option>
            </select>
          </label>
        </div>
      </div>

      <div className='mb-3 flex items-center justify-between'>
        <span className='text-muted-foreground text-xs'>
          {sorted.length} {t('models')}
        </span>
        <span className='text-muted-foreground text-xs'>
          {t('Unit: RMB / {{unit}} tokens', { unit: unitLabel })}
        </span>
      </div>

      {paged.length > 0 ? (
        <DirectoryTable
          models={paged}
          selectedGroup={effectiveGroup}
          tokenUnit={tokenUnit}
          priceRate={priceRate}
          usdExchangeRate={usdExchangeRate}
          onModelClick={onModelClick}
        />
      ) : (
        <div className='border-y py-16 text-center'>
          <p className='text-muted-foreground text-sm'>{t('No models')}</p>
        </div>
      )}

      {pageCount > 1 && (
        <div className='mt-4 flex items-center justify-center gap-6'>
          <button
            type='button'
            disabled={currentPage <= 1}
            onClick={() => setPage((p) => Math.max(1, p - 1))}
            className='text-muted-foreground hover:text-foreground disabled:opacity-30 text-sm'
          >
            ← {t('Prev')}
          </button>
          <span className='text-muted-foreground text-xs tabular-nums'>
            {currentPage} / {pageCount}
          </span>
          <button
            type='button'
            disabled={currentPage >= pageCount}
            onClick={() => setPage((p) => Math.min(pageCount, p + 1))}
            className='text-muted-foreground hover:text-foreground disabled:opacity-30 text-sm'
          >
            {t('Next')} →
          </button>
        </div>
      )}

    </div>
  )
}
