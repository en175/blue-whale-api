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
import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'

import { getLobeIcon } from '@/lib/lobe-icon'
import { cn } from '@/lib/utils'

import type { PricingModel, PricingVendor } from '../types'

// ----------------------------------------------------------------------------
// DIRECTORY 视图：竞品式厂商导航（固定目录，未启用厂商灰禁）
// ----------------------------------------------------------------------------

interface VendorCatalogItem {
  key: string
  name: string
  fallbackIcon: string
  match: (model: PricingModel, vendor: PricingVendor | undefined) => boolean
}

/** 固定厂商目录，顺序对标竞品布局。未启用厂商灰禁显示。 */
const VENDOR_CATALOG: VendorCatalogItem[] = [
  {
    key: 'doubao',
    name: '豆包',
    fallbackIcon: 'Doubao.Color',
    // seedance/seedream 是字节（豆包）的视频/图像模型族，但上游模型名常不带 doubao 前缀，
    // 这里一并归入豆包，避免它们在 DIRECTORY 视图被静默丢弃。
    match: (m) => /^(doubao|seedance|seedream)/i.test(m.model_name),
  },
  {
    key: 'claude',
    name: 'Claude',
    fallbackIcon: 'Claude.Color',
    match: (m, v) =>
      /claude/i.test(m.model_name) || /anthropic/i.test(v?.name || ''),
  },
  {
    key: 'chatgpt',
    name: 'ChatGPT',
    fallbackIcon: 'OpenAI',
    match: (m, v) =>
      /^(gpt|o\d|chatgpt)/i.test(m.model_name) || /openai/i.test(v?.name || ''),
  },
  {
    key: 'grok',
    name: 'Grok',
    fallbackIcon: 'Grok',
    match: (m, v) => /^grok/i.test(m.model_name) || /xai/i.test(v?.name || ''),
  },
  {
    key: 'gemini',
    name: 'Gemini',
    fallbackIcon: 'Gemini.Color',
    match: (m, v) =>
      /^gemini/i.test(m.model_name) || /google/i.test(v?.name || ''),
  },
  {
    key: 'zhipu',
    name: '智谱',
    fallbackIcon: 'Zhipu.Color',
    match: (m, v) =>
      /^glm/i.test(m.model_name) || /zhipu|智谱/i.test(v?.name || ''),
  },
  {
    key: 'kimi',
    name: 'Kimi',
    fallbackIcon: 'Kimi.Color',
    match: (m, v) =>
      /^(kimi|moonshot)/i.test(m.model_name) || /moonshot/i.test(v?.name || ''),
  },
  {
    key: 'deepseek',
    name: 'DeepSeek',
    fallbackIcon: 'DeepSeek.Color',
    match: (m, v) =>
      /^deepseek/i.test(m.model_name) || /deepseek/i.test(v?.name || ''),
  },
  {
    key: 'minimax',
    name: 'MiniMax',
    fallbackIcon: 'Minimax.Color',
    match: (m, v) =>
      /^(minimax|abab)/i.test(m.model_name) || /minimax/i.test(v?.name || ''),
  },
]

export function getVendorCatalog() {
  return VENDOR_CATALOG
}

/** 按厂商目录分组模型；不属于任何目录的模型被忽略（DIRECTORY 只列主流厂商）。 */
export function groupModelsByVendor(
  models: PricingModel[],
  vendors: PricingVendor[]
): Record<string, PricingModel[]> {
  const vendorMap = new Map(vendors.map((v) => [v.id, v]))
  return models.reduce<Record<string, PricingModel[]>>((acc, model) => {
    const vendor = model.vendor_id ? vendorMap.get(model.vendor_id) : undefined
    for (const item of VENDOR_CATALOG) {
      if (item.match(model, vendor)) {
        ;(acc[item.key] ||= []).push(model)
        break
      }
    }
    return acc
  }, {})
}

export interface VendorTabsProps {
  models: PricingModel[]
  vendors: PricingVendor[]
  selectedKey: string
  onSelect: (key: string) => void
}

export function VendorTabs({
  models,
  vendors,
  selectedKey,
  onSelect,
}: VendorTabsProps) {
  const { t } = useTranslation()
  const grouped = useMemo(
    () => groupModelsByVendor(models, vendors),
    [models, vendors]
  )
  const vendorMap = useMemo(
    () => new Map(vendors.map((vendor) => [vendor.id, vendor])),
    [vendors]
  )

  return (
    <div
      role='tablist'
      aria-label={t('Vendors')}
      className='bg-card flex gap-2 overflow-x-auto rounded-xl border p-2 shadow-sm'
    >
      {VENDOR_CATALOG.filter(
        (item) => (grouped[item.key]?.length || 0) > 0
      ).map((item) => {
        const vendorModels = grouped[item.key] || []
        const count = vendorModels.length
        const vendor = vendorModels
          .map((model) =>
            model.vendor_id ? vendorMap.get(model.vendor_id) : undefined
          )
          .find(Boolean)
        const iconKey = vendor?.icon || item.fallbackIcon
        const active = item.key === selectedKey
        return (
          <button
            key={item.key}
            role='tab'
            type='button'
            aria-selected={active}
            title={t('{{count}} models', { count })}
            onClick={() => onSelect(item.key)}
            className={cn(
              'flex shrink-0 items-center gap-2.5 rounded-lg border px-4 py-2.5 text-base font-semibold transition-colors sm:text-lg',
              active
                ? 'border-primary/50 bg-primary/10 text-primary shadow-sm'
                : 'border-transparent text-muted-foreground hover:border-border hover:bg-muted/50 hover:text-foreground'
            )}
          >
            <span className='flex size-6 shrink-0 items-center justify-center'>
              {getLobeIcon(iconKey, 24)}
            </span>
            <span>{item.name}</span>
            <span className='text-muted-foreground/70 text-xs font-medium tabular-nums'>
              {count}
            </span>
          </button>
        )
      })}
    </div>
  )
}
