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
import type { PricingFormValues } from '../lib/schema'
import type { PricingRule } from '../lib/types'

let idSeq = 10
let rules: PricingRule[] = [
  {
    id: 1, model: 'gpt-4o', input_price: 0.015, output_price: 0.06,
    unit: '1K tokens', ratio: 1.0, is_active: true,
    created_at: '2026-01-01T00:00:00Z', updated_at: '2026-01-01T00:00:00Z',
  },
  {
    id: 2, model: 'gpt-4o-mini', input_price: 0.00015, output_price: 0.0006,
    unit: '1K tokens', ratio: 1.0, is_active: true,
    created_at: '2026-01-01T00:00:00Z', updated_at: '2026-01-01T00:00:00Z',
  },
  {
    id: 3, model: 'claude-3-opus', input_price: 0.015, output_price: 0.075,
    unit: '1K tokens', ratio: 1.2, is_active: true,
    created_at: '2026-01-01T00:00:00Z', updated_at: '2026-01-01T00:00:00Z',
  },
  {
    id: 4, model: 'gemini-1.5-pro', input_price: 0.0035, output_price: 0.0105,
    unit: '1K tokens', ratio: 1.0, is_active: false,
    created_at: '2026-01-01T00:00:00Z', updated_at: '2026-01-01T00:00:00Z',
  },
]

function wrap<T>(data: T) { return { success: true, message: '', data } }
function delay(ms = 200) { return new Promise((r) => setTimeout(r, ms)) }

export async function fetchPricingRules() {
  await delay()
  return wrap([...rules].sort((a, b) => a.model.localeCompare(b.model)))
}

export async function createPricingRule(values: PricingFormValues) {
  await delay()
  const rule: PricingRule = {
    id: ++idSeq, ...values,
    is_active: true,
    created_at: new Date().toISOString(), updated_at: new Date().toISOString(),
  }
  rules.push(rule)
  return wrap(rule)
}

export async function updatePricingRule(id: number, values: Partial<PricingFormValues>) {
  await delay()
  const idx = rules.findIndex((r) => r.id === id)
  if (idx === -1) return { success: false, message: 'Not found', data: null }
  rules[idx] = { ...rules[idx], ...values, updated_at: new Date().toISOString() }
  return wrap(rules[idx])
}

export async function deletePricingRule(id: number) {
  await delay()
  rules = rules.filter((r) => r.id !== id)
  return wrap(null)
}

export async function togglePricingRule(id: number) {
  await delay()
  const r = rules.find((r) => r.id === id)
  if (!r) return { success: false, message: 'Not found', data: null }
  r.is_active = !r.is_active
  r.updated_at = new Date().toISOString()
  return wrap(r)
}
