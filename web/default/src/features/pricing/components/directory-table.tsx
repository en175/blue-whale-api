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

import { DEFAULT_TOKEN_UNIT, QUOTA_TYPE_VALUES } from '../constants'
import { formatRmbFixedPrice, formatRmbGroupPrice } from '../lib/price'
import type { PricingModel, PriceType, TokenUnit } from '../types'

// ----------------------------------------------------------------------------
// DIRECTORY 视图：模型 ID / 输入 / 输出 / 缓存创建 / 缓存读取 / 节省幅度
// ----------------------------------------------------------------------------

export type DirectoryPriceMode = 'group' | 'official'

export interface DirectoryTableProps {
  models: PricingModel[]
  selectedGroup: string
  groupRatio: Record<string, number>
  priceMode: DirectoryPriceMode
  tokenUnit?: TokenUnit
  priceRate: number
  onModelClick: (modelName: string) => void
}

function hasRatio(value: number | null | undefined): boolean {
  return value != null && Number.isFinite(Number(value))
}

export function DirectoryTable({
  models,
  selectedGroup,
  groupRatio,
  priceMode,
  tokenUnit = DEFAULT_TOKEN_UNIT,
  priceRate,
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
          <tr className='text-muted-foreground h-12 border-b text-xs font-medium'>
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
              groupRatio={groupRatio}
              priceMode={priceMode}
              tokenUnit={tokenUnit}
              priceRate={priceRate}
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
  groupRatio,
  priceMode,
  tokenUnit,
  priceRate,
  onModelClick,
}: {
  model: PricingModel
  selectedGroup: string
  groupRatio: Record<string, number>
  priceMode: DirectoryPriceMode
  tokenUnit: TokenUnit
  priceRate: number
  onModelClick: (modelName: string) => void
}) {
  const { t } = useTranslation()
  const isRequest = model.quota_type === QUOTA_TYPE_VALUES.REQUEST

  const fmtGroup = (type: PriceType) =>
    formatRmbGroupPrice(
      model,
      selectedGroup,
      type,
      tokenUnit,
      priceRate,
      groupRatio
    )

  // model_ratio/model_price 是后端保存的官方基准价；分组价在此基础上乘分组倍率。
  const officialRatio = { [selectedGroup]: 1 }
  const officialInput = isRequest
    ? formatRmbFixedPrice(model, selectedGroup, priceRate, officialRatio)
    : formatRmbGroupPrice(
        model,
        selectedGroup,
        'input',
        tokenUnit,
        priceRate,
        officialRatio
      )
  const officialOutput = isRequest
    ? null
    : formatRmbGroupPrice(
        model,
        selectedGroup,
        'output',
        tokenUnit,
        priceRate,
        officialRatio
      )
  const officialCacheCreate =
    !isRequest && hasRatio(model.create_cache_ratio)
      ? formatRmbGroupPrice(
          model,
          selectedGroup,
          'create_cache',
          tokenUnit,
          priceRate,
          officialRatio
        )
      : null
  const officialCacheRead =
    !isRequest && hasRatio(model.cache_ratio)
      ? formatRmbGroupPrice(
          model,
          selectedGroup,
          'cache',
          tokenUnit,
          priceRate,
          officialRatio
        )
      : null

  const groupedFixed = isRequest
    ? formatRmbFixedPrice(model, selectedGroup, priceRate, groupRatio)
    : null
  const groupedInput = isRequest ? groupedFixed : fmtGroup('input')
  const groupedOutput = isRequest ? null : fmtGroup('output')
  const groupedCacheCreate =
    !isRequest && hasRatio(model.create_cache_ratio)
      ? fmtGroup('create_cache')
      : null
  const groupedCacheRead =
    !isRequest && hasRatio(model.cache_ratio) ? fmtGroup('cache') : null

  const selectedRatio = groupRatio[selectedGroup] ?? 1
  const saving =
    selectedRatio < 1 ? Math.round((1 - selectedRatio) * 100) : null

  const renderPriceCell = (
    grouped: string | null,
    officialText: string | null
  ) => {
    const primary = priceMode === 'official' ? officialText : grouped
    const comparison = priceMode === 'group' ? officialText : null

    return (
      <td className='px-4 text-right align-middle'>
        <div className='font-mono text-[13px] tabular-nums'>
          {primary || '—'}
        </div>
        {comparison && (
          <div className='text-muted-foreground/50 mt-0.5 font-mono text-[11px] tabular-nums line-through'>
            {t('Official {{price}}', { price: comparison })}
          </div>
        )}
      </td>
    )
  }

  return (
    <tr className='group hover:bg-muted/40 h-[72px] border-b transition-colors last:border-0'>
      <td className='px-4'>
        <button
          type='button'
          onClick={() => onModelClick(model.model_name)}
          className='text-foreground font-mono text-sm font-semibold underline-offset-4 group-hover:underline'
        >
          {model.model_name}
        </button>
        {isRequest && (
          <span className='text-muted-foreground mt-1 block text-[11px]'>
            {t('Per Request')}
          </span>
        )}
      </td>
      {renderPriceCell(groupedInput, officialInput)}
      {renderPriceCell(groupedOutput, officialOutput)}
      {renderPriceCell(groupedCacheCreate, officialCacheCreate)}
      {renderPriceCell(groupedCacheRead, officialCacheRead)}
      <td className='px-4 text-right font-mono text-[13px] font-medium tabular-nums'>
        {priceMode === 'group' && saving !== null ? (
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
