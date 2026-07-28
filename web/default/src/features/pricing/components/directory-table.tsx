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
// DIRECTORY 视图：按模型类别渲染不同列结构
//   文字：模型 / 输入 / 输出 / 节省
//   图片：模型 / 价格(元/张) / 节省
//   视频：模型 / 价格(元/百万token) / 节省
// ----------------------------------------------------------------------------

export type DirectoryPriceMode = 'group' | 'official'
export type DirectoryCategory = 'text' | 'image' | 'video'

export interface DirectoryTableProps {
  models: PricingModel[]
  selectedGroup: string
  groupRatio: Record<string, number>
  priceMode: DirectoryPriceMode
  category: DirectoryCategory
  tokenUnit?: TokenUnit
  priceRate: number
  onModelClick: (modelName: string) => void
}

export function DirectoryTable({
  models,
  selectedGroup,
  groupRatio,
  priceMode,
  category,
  tokenUnit = DEFAULT_TOKEN_UNIT,
  priceRate,
  onModelClick,
}: DirectoryTableProps) {
  const { t } = useTranslation()
  // 文字类别双列(输入/输出)，图片/视频单一价格列
  const isText = category === 'text'

  return (
    <div className='overflow-x-auto border-y'>
      <table className='w-full min-w-[640px] border-collapse text-left'>
        <colgroup>
          <col className={isText ? 'w-[40%]' : 'w-[50%]'} />
          {isText ? (
            <>
              <col className='w-[22%]' />
              <col className='w-[22%]' />
            </>
          ) : (
            <col className='w-[30%]' />
          )}
          <col className={isText ? 'w-[16%]' : 'w-[20%]'} />
        </colgroup>
        <thead>
          <tr className='text-muted-foreground h-12 border-b text-xs font-medium'>
            <th className='px-4 font-medium'>{t('Model ID')}</th>
            {isText ? (
              <>
                <th className='px-4 text-right font-medium'>
                  {t('Input Price')}
                </th>
                <th className='px-4 text-right font-medium'>
                  {t('Output Price')}
                </th>
              </>
            ) : (
              <th className='px-4 text-right font-medium'>{t('Price')}</th>
            )}
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
              category={category}
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
  category,
  tokenUnit,
  priceRate,
  onModelClick,
}: {
  model: PricingModel
  selectedGroup: string
  groupRatio: Record<string, number>
  priceMode: DirectoryPriceMode
  category: DirectoryCategory
  tokenUnit: TokenUnit
  priceRate: number
  onModelClick: (modelName: string) => void
}) {
  const { t } = useTranslation()
  const isRequest = model.quota_type === QUOTA_TYPE_VALUES.REQUEST
  const isText = category === 'text'

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

  const groupedFixed = isRequest
    ? formatRmbFixedPrice(model, selectedGroup, priceRate, groupRatio)
    : null
  // 图片(quota_type=1)与视频(quota_type=0, completion_ratio=0)都只展示单一价：
  // 图片走固定按张价，视频走 input token 价(output 无价，不单列)。
  const groupedInput = isRequest ? groupedFixed : fmtGroup('input')
  const groupedOutput = isRequest ? null : fmtGroup('output')

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
    <tr className='group hover:bg-muted/40 h-[72px] border-b transition-colors last:border-b-0'>
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
      {isText && renderPriceCell(groupedOutput, officialOutput)}
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
