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
import { CheckCircle2, FileText, Image as ImageIcon, Video } from 'lucide-react'
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { PageTransition } from '@/components/page-transition'
import { cn } from '@/lib/utils'

import { EXCLUDED_GROUPS } from '../constants'
import type { PricingModel, PricingVendor, TokenUnit } from '../types'
import { DirectoryTable, type DirectoryPriceMode } from './directory-table'
import {
  getVendorCatalog,
  groupModelsByVendor,
  VendorTabs,
} from './vendor-tabs'

// ----------------------------------------------------------------------------
// DIRECTORY 视图容器：页面介绍 + 厂商导航 + 价格/模型类型选择 + 统一价表
// ----------------------------------------------------------------------------

const PAGE_SIZE = 10

type ModelCategory = 'text' | 'image' | 'video'

const MODEL_CATEGORIES = [
  { key: 'text', label: 'Text models', icon: FileText },
  { key: 'image', label: 'Image models', icon: ImageIcon },
  { key: 'video', label: 'Video models', icon: Video },
] as const

/** 提取模型名尾部 YYMMDD 日期，用于「新优先」排序；无日期返回 0。 */
function dateSuffix(modelName: string): number {
  const match = modelName.match(/(\d{6})$/)
  return match ? Number.parseInt(match[1], 10) : 0
}

/** 优先使用后端标签；旧数据没有模态字段时再按模型族名称兜底。 */
function modelCategory(model: PricingModel): ModelCategory {
  const searchable = [model.model_name, model.tags, model.description]
    .filter(Boolean)
    .join(' ')
    .toLowerCase()

  if (/图片|图像|seedream|image[- ]?generation/.test(searchable)) {
    return 'image'
  }
  if (/视频|seedance|seaweed|\bwan\b|[it]2v|video/.test(searchable)) {
    return 'video'
  }
  return 'text'
}

export interface ModelDirectoryProps {
  models: PricingModel[]
  vendors: PricingVendor[]
  groupRatio: Record<string, number>
  usableGroup: Record<string, { desc: string; ratio: number }>
  priceRate: number
  tokenUnit: TokenUnit
  onModelClick: (modelName: string) => void
}

export function ModelDirectory({
  models,
  vendors,
  groupRatio,
  usableGroup,
  priceRate,
  tokenUnit,
  onModelClick,
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
  const [page, setPage] = useState(1)
  const [priceMode, setPriceMode] = useState<DirectoryPriceMode>('group')
  const [category, setCategory] = useState<ModelCategory>('text')

  const availableGroups = useMemo(
    () =>
      Object.keys(usableGroup || {}).filter(
        (group) => !EXCLUDED_GROUPS.includes(group)
      ),
    [usableGroup]
  )
  const [selectedGroup, setSelectedGroup] = useState<string>(
    availableGroups[0] || ''
  )
  const effectiveGroup = selectedGroup || availableGroups[0] || ''

  const vendorModels = useMemo(() => {
    const result = [...(grouped[vendorKey] || [])]
    result.sort((a, b) => {
      const dateDelta = dateSuffix(b.model_name) - dateSuffix(a.model_name)
      return dateDelta || a.model_name.localeCompare(b.model_name)
    })
    return result
  }, [grouped, vendorKey])

  const categoryCounts = useMemo(
    () =>
      vendorModels.reduce<Record<ModelCategory, number>>(
        (counts, model) => {
          counts[modelCategory(model)] += 1
          return counts
        },
        { text: 0, image: 0, video: 0 }
      ),
    [vendorModels]
  )

  const filteredModels = useMemo(
    () => vendorModels.filter((model) => modelCategory(model) === category),
    [category, vendorModels]
  )

  const pageCount = Math.max(1, Math.ceil(filteredModels.length / PAGE_SIZE))
  const currentPage = Math.min(page, pageCount)
  const paged = useMemo(
    () =>
      filteredModels.slice(
        (currentPage - 1) * PAGE_SIZE,
        currentPage * PAGE_SIZE
      ),
    [filteredModels, currentPage]
  )

  const resetPage = () => setPage(1)
  const unitLabel = tokenUnit === 'K' ? '1K' : '1M'
  const vendorName = catalog.find((item) => item.key === vendorKey)?.name || ''

  return (
    <div className='relative overflow-hidden'>
      <div
        aria-hidden
        className='pointer-events-none absolute inset-x-0 top-0 h-[520px] opacity-20 dark:opacity-[0.10]'
        style={{
          background: [
            'radial-gradient(ellipse 60% 50% at 20% 20%, oklch(0.72 0.18 250 / 80%) 0%, transparent 70%)',
            'radial-gradient(ellipse 50% 40% at 80% 15%, oklch(0.65 0.15 200 / 60%) 0%, transparent 70%)',
          ].join(', '),
          maskImage: 'linear-gradient(to bottom, black 40%, transparent 100%)',
          WebkitMaskImage:
            'linear-gradient(to bottom, black 40%, transparent 100%)',
        }}
      />

      <PageTransition className='relative mx-auto w-full max-w-[1240px] px-6 pt-20 pb-24'>
        <header className='mx-auto max-w-3xl text-center'>
          <h1 className='text-[clamp(2.25rem,5.5vw,3.75rem)] leading-[1.12] font-bold tracking-tight'>
            {t('Model Square')}
          </h1>
          <p className='text-muted-foreground/80 mt-4 text-base sm:text-lg'>
            {t('This site currently has {{count}} models enabled', {
              count: models.length,
            })}
          </p>
          <p className='text-muted-foreground/60 mx-auto mt-2 max-w-2xl text-sm leading-relaxed sm:text-base'>
            {t(
              'Discover curated AI models, compare pricing and capabilities, and choose the right model for every scenario.'
            )}
          </p>
        </header>

        <div className='mt-12'>
          <VendorTabs
            models={models}
            vendors={vendors}
            selectedKey={vendorKey}
            onSelect={(key) => {
              setVendorKey(key)
              setCategory('text')
              resetPage()
            }}
          />
        </div>

        <section className='pt-16'>
          <div className='flex flex-col gap-6 sm:flex-row sm:items-end sm:justify-between'>
            <div>
              <span className='text-primary text-sm font-semibold'>
                {vendorName}
              </span>
              <h2 className='mt-2 text-3xl font-bold tracking-tight sm:text-4xl'>
                {t('Model Pricing')}
              </h2>
              <p className='text-muted-foreground mt-3 max-w-2xl text-sm sm:text-base'>
                {t(
                  'Prices are displayed in RMB by default, making it easy to compare official and group prices.'
                )}
              </p>
            </div>

            <div>
              <div className='text-muted-foreground mb-2 text-xs font-semibold'>
                {t('Price display')}
              </div>
              <div className='bg-muted flex rounded-xl border p-1'>
                {(['group', 'official'] as const).map((mode) => (
                  <button
                    key={mode}
                    type='button'
                    aria-pressed={priceMode === mode}
                    onClick={() => setPriceMode(mode)}
                    className={cn(
                      'rounded-lg px-4 py-2 text-sm font-semibold transition-all',
                      priceMode === mode
                        ? 'bg-background text-foreground shadow-sm'
                        : 'text-muted-foreground hover:text-foreground'
                    )}
                  >
                    {t(mode === 'group' ? 'Group Price' : 'Official Price')}
                  </button>
                ))}
              </div>
            </div>
          </div>
        </section>

        {availableGroups.length > 0 && (
          <section className='mt-10'>
            <h3 className='mb-3 text-sm font-semibold'>
              {t('Select a price group')}
            </h3>
            <div
              className='grid grid-cols-1 gap-3 sm:grid-cols-2'
              style={{
                gridTemplateColumns:
                  availableGroups.length > 2
                    ? 'repeat(auto-fit, minmax(240px, 1fr))'
                    : undefined,
              }}
            >
              {availableGroups.map((group) => {
                const ratio = groupRatio[group] ?? 1
                const discount = Number((ratio * 10).toFixed(1)).toString()
                const active = group === effectiveGroup

                return (
                  <button
                    key={group}
                    type='button'
                    aria-pressed={active}
                    onClick={() => {
                      setSelectedGroup(group)
                      setPriceMode('group')
                      resetPage()
                    }}
                    className={cn(
                      'relative flex min-h-[112px] flex-col items-start rounded-xl border px-5 py-4 text-left transition-all',
                      active
                        ? 'border-primary/60 bg-primary/10 shadow-sm'
                        : 'bg-card hover:border-primary/30 hover:bg-muted/30'
                    )}
                  >
                    {active && (
                      <CheckCircle2 className='text-primary absolute top-4 right-4 size-5' />
                    )}
                    <span className='flex items-center gap-2.5 pr-8'>
                      <span className='text-xl font-bold'>{group}</span>
                      <span className='bg-primary/10 text-primary rounded-full px-2.5 py-0.5 text-xs font-bold'>
                        {t('{{discount}} tenths', { discount })}
                      </span>
                    </span>
                    <span className='text-muted-foreground mt-3 text-sm'>
                      {t(
                        '{{ratio}}x ratio · approximately {{discount}} tenths of the official price',
                        { ratio, discount }
                      )}
                    </span>
                  </button>
                )
              })}
            </div>
          </section>
        )}

        <section className='mt-8'>
          <h3 className='mb-3 text-sm font-semibold'>
            {t('Select a model type')}
          </h3>
          <div className='grid grid-cols-1 gap-3 sm:grid-cols-3'>
            {MODEL_CATEGORIES.map((item) => {
              const Icon = item.icon
              const active = category === item.key
              return (
                <button
                  key={item.key}
                  type='button'
                  aria-pressed={active}
                  onClick={() => {
                    setCategory(item.key)
                    resetPage()
                  }}
                  className={cn(
                    'flex min-h-[84px] items-center gap-4 rounded-xl border px-5 text-left transition-all',
                    active
                      ? 'border-primary/60 bg-primary/10 text-primary shadow-sm'
                      : 'bg-card hover:border-primary/30 hover:bg-muted/30'
                  )}
                >
                  <span className='bg-background flex size-10 shrink-0 items-center justify-center rounded-lg border shadow-sm'>
                    <Icon className='size-5' />
                  </span>
                  <span>
                    <span className='block text-base font-bold'>
                      {t(item.label)}
                    </span>
                    <span className='text-muted-foreground mt-0.5 block text-xs tabular-nums'>
                      {t('{{count}} models', {
                        count: categoryCounts[item.key],
                      })}
                    </span>
                  </span>
                </button>
              )
            })}
          </div>
        </section>

        <div className='mt-10 mb-3 flex items-center justify-between'>
          <span className='text-muted-foreground text-xs'>
            {t('{{count}} models', { count: filteredModels.length })}
          </span>
          <span className='text-muted-foreground text-xs'>
            {category === 'image'
              ? t('Unit: RMB / image')
              : t('Unit: RMB / {{unit}} tokens', { unit: unitLabel })}
          </span>
        </div>

        {paged.length > 0 ? (
          <DirectoryTable
            models={paged}
            selectedGroup={effectiveGroup}
            groupRatio={groupRatio}
            priceMode={priceMode}
            category={category}
            tokenUnit={tokenUnit}
            priceRate={priceRate}
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
              onClick={() => setPage((value) => Math.max(1, value - 1))}
              className='text-muted-foreground hover:text-foreground text-sm disabled:opacity-30'
            >
              ← {t('Prev')}
            </button>
            <span className='text-muted-foreground text-xs tabular-nums'>
              {currentPage} / {pageCount}
            </span>
            <button
              type='button'
              disabled={currentPage >= pageCount}
              onClick={() => setPage((value) => Math.min(pageCount, value + 1))}
              className='text-muted-foreground hover:text-foreground text-sm disabled:opacity-30'
            >
              {t('Next')} →
            </button>
          </div>
        )}
      </PageTransition>
    </div>
  )
}
