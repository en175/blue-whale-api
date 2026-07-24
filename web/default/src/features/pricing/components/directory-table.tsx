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
import { useTranslation } from 'react-i18next'

import { DEFAULT_TOKEN_UNIT } from '../constants'
import { getDisplayGroupRatio } from '../lib/model-helpers'
import { computeSavingPct, getOfficialPrice } from '../lib/official-price'
import { formatPrice } from '../lib/price'
import type { PricingModel, PriceType, TokenUnit } from '../types'

// ----------------------------------------------------------------------------
// DIRECTORY 视图：精简六列价格表
// 模型 ID / 输入价格 / 输出价格 / 缓存创建 / 缓存读取 / 节省幅度
// ----------------------------------------------------------------------------

export interface DirectoryTableProps {
  models: PricingModel[]
  selectedGroup?: string
  tokenUnit?: TokenUnit
  priceRate: number
  usdExchangeRate: number
  onModelClick: (modelName: string) => void
}

function hasRatio(value: number | null | undefined): boolean {
  return value != null && Number.isFinite(Number(value))
}

export function DirectoryTable({
  models,
  selectedGroup,
  tokenUnit = DEFAULT_TOKEN_UNIT,
  priceRate,
  usdExchangeRate,
  onModelClick,
}: DirectoryTableProps) {
  const { t } = useTranslation()

  return (
    <div className='overflow-x-auto border-y'>
      <table className='w-full min-w-[860px] border-collapse text-left'>
        <colgroup>
          <col className='w-[36%]' />
          <col className='w-[13%]' />
          <col className='w-[13%]' />
          <col className='w-[13%]' />
          <col className='w-[13%]' />
          <col className='w-[12%]' />
        </colgroup>
        <thead>
          <tr className='h-12 border-b text-xs font-medium text-muted-foreground'>
            <th className='px-4 font-medium'>{t('Model ID')}</th>
            <th className='px-4 text-right font-medium'>{t('Input Price')}</th>
            <th className='px-4 text-right font-medium'>{t('Output Price')}</th>
            <th className='px-4 text-right font-medium'>{t('Cache Create')}</th>
            <th className='px-4 text-right font-medium'>{t('Cache Read')}</th>
            <th className='px-4 text-right font-medium'>{t('Savings')}</th>
          </tr>
        </thead>
        <tbody>
          {models.map((model) => (
            <DirectoryRow
              key={model.model_name}
              model={model}
              selectedGroup={selectedGroup}
              tokenUnit={tokenUnit}
              priceRate={priceRate}
              usdExchangeRate={usdExchangeRate}
              onModelClick={onModelClick}
            />
          ))}
        </tbody>
      </table>
    </div>
  )
}

function DirectoryRow({
  model,
  selectedGroup,
  tokenUnit,
  priceRate,
  usdExchangeRate,
  onModelClick,
}: {
  model: PricingModel
  selectedGroup?: string
  tokenUnit: TokenUnit
  priceRate: number
  usdExchangeRate: number
  onModelClick: (modelName: string) => void
}) {
  const fmt = (type: PriceType) =>
    formatPrice(
      model,
      type,
      tokenUnit,
      false,
      priceRate,
      usdExchangeRate,
      selectedGroup
    )

  const cacheRead = hasRatio(model.cache_ratio) ? fmt('cache') : '—'
  const cacheCreate = hasRatio(model.create_cache_ratio)
    ? fmt('create_cache')
    : '—'

  // 节省幅度：相对官方 input 价。ourInputUSD = model_ratio*2*groupRatio (USD/1M)
  const official = getOfficialPrice(model.model_name)
  const groupRatio = getDisplayGroupRatio(model, selectedGroup)
  const ourInputUSD = model.model_ratio * 2 * groupRatio
  const saving = official ? computeSavingPct(ourInputUSD, official) : null

  return (
    <tr className='group h-[68px] border-b transition-colors last:border-0 hover:bg-muted/40'>
      <td className='px-4'>
        <button
          type='button'
          onClick={() => onModelClick(model.model_name)}
          className='text-foreground font-mono text-[13px] underline-offset-4 group-hover:underline'
        >
          {model.model_name}
        </button>
      </td>
      <td className='px-4 text-right font-mono text-[13px] tabular-nums'>
        {fmt('input')}
      </td>
      <td className='px-4 text-right font-mono text-[13px] tabular-nums'>
        {fmt('output')}
      </td>
      <td className='text-muted-foreground px-4 text-right font-mono text-[13px] tabular-nums'>
        {cacheCreate}
      </td>
      <td className='text-muted-foreground px-4 text-right font-mono text-[13px] tabular-nums'>
        {cacheRead}
      </td>
      <td className='px-4 text-right font-mono text-[13px] font-medium tabular-nums'>
        {saving !== null ? (
          <span className='text-emerald-600 dark:text-emerald-400'>
            {saving}%
          </span>
        ) : (
          <span className='text-muted-foreground/40'>—</span>
        )}
      </td>
    </tr>
  )
}
