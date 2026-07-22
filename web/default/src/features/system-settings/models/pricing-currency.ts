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
import { formatPricingNumber } from './pricing-format'

/**
 * 模型定价输入币种开关。内部值始终 $ 锚点（后端美元锚点不动），
 * CNY 仅作用于输入视图层：符号变 ¥、输入值前端 ÷汇率 转 $ 存内部。
 * 见 ADR-0021 决策②。
 */
export type PriceCurrency = 'USD' | 'CNY'

/**
 * $ 内部值 → 显示值。CNY 模式 ×rate；USD 模式透传。
 * 空串 / 非数草稿透传（不换算）。
 */
export function usdToDisplay(
  usd: string,
  currency: PriceCurrency,
  rate: number
): string {
  if (currency === 'USD') return usd
  if (usd === '') return usd
  const num = Number(usd)
  if (!Number.isFinite(num)) return usd
  return formatPricingNumber(num * rate)
}

/**
 * 显示值 → $ 内部值。CNY 模式 ÷rate；USD 模式透传。
 * 空串 / 非数草稿透传（调用方负责 numericDraftRegex 预校验）。
 */
export function displayToUsd(
  display: string,
  currency: PriceCurrency,
  rate: number
): string {
  if (currency === 'USD') return display
  if (display === '') return ''
  const num = Number(display)
  if (!Number.isFinite(num)) return display
  return formatPricingNumber(num / rate)
}

export function priceSymbol(currency: PriceCurrency): string {
  return currency === 'CNY' ? '¥' : '$'
}
