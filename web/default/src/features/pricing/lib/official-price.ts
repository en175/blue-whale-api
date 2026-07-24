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
// DIRECTORY 视图用：官方公开定价（USD / 1M tokens），用于计算"节省幅度"列。
// 仅覆盖有公开定价的主流模型；未收录模型该列显示 —。
// 价格源：各厂商官网定价页。后端无官方价字段时以此为准（Phase 2 数据源）。

export interface OfficialPrice {
  input: number
  output: number
  createCache?: number
  readCache?: number
}

// 按 model_name 前缀匹配，顺序即优先级。
const OFFICIAL_PRICES: Array<{ pattern: RegExp; price: OfficialPrice }> = [
  // Anthropic Claude
  {
    pattern: /^claude-opus-4-[78]/i,
    price: { input: 5, output: 25, createCache: 6.25, readCache: 0.5 },
  },
  {
    pattern: /^claude-opus-4-6/i,
    price: { input: 5, output: 25, createCache: 6.25, readCache: 0.5 },
  },
  {
    pattern: /^claude-sonnet-5/i,
    price: { input: 2, output: 10, createCache: 2.5, readCache: 0.2 },
  },
  {
    pattern: /^claude-sonnet-4-6/i,
    price: { input: 3, output: 15, createCache: 3.75, readCache: 0.3 },
  },
  {
    pattern: /^claude-haiku-4-5/i,
    price: { input: 1, output: 5, createCache: 1.25, readCache: 0.1 },
  },
  // DeepSeek
  {
    pattern: /^deepseek-v4-pro/i,
    price: { input: 2, output: 8, createCache: 2.5, readCache: 0.2 },
  },
  {
    pattern: /^deepseek-v4-flash/i,
    price: { input: 0.5, output: 2, createCache: 0.625, readCache: 0.05 },
  },
  // 智谱 GLM
  { pattern: /^glm-5-2/i, price: { input: 0.5, output: 0.5 } },
  { pattern: /^glm-4-plus/i, price: { input: 0.5, output: 2 } },
]

export function getOfficialPrice(modelName: string): OfficialPrice | null {
  const entry = OFFICIAL_PRICES.find((item) => item.pattern.test(modelName))
  return entry ? entry.price : null
}

/**
 * 节省幅度（%），相对官方 input 价。
 * 高于或等于官方价时不显示折扣，返回 null（避免误导性的负折扣或 0）。
 *
 * ourInputPriceUSD 为按当前分组倍率折算后的 input 价（USD / 1M tokens）。
 */
export function computeSavingPct(
  ourInputPriceUSD: number,
  official: OfficialPrice
): number | null {
  if (!official.input || official.input <= 0) return null
  if (!Number.isFinite(ourInputPriceUSD) || ourInputPriceUSD <= 0) return null
  const pct = Math.round((1 - ourInputPriceUSD / official.input) * 100)
  return pct > 0 ? pct : null
}
