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
import type { TierFormValues } from '../lib/schema'
import type { TierPreviewItem, TopupTier } from '../lib/types'

let idSeq = 10
let tiers: TopupTier[] = [
  {
    id: 1, price_yuan: 10, points: 100, bonus_points: 0, sort_order: 1,
    is_active: true, effective_at: '2026-01-01T00:00:00Z', expires_at: '2027-01-01T00:00:00Z',
    created_at: '2026-01-01T00:00:00Z', updated_at: '2026-01-01T00:00:00Z',
    charge_count: 356, total_revenue: 3560,
  },
  {
    id: 2, price_yuan: 50, points: 550, bonus_points: 50, sort_order: 2,
    is_active: true, effective_at: '2026-01-01T00:00:00Z', expires_at: '2027-01-01T00:00:00Z',
    created_at: '2026-01-01T00:00:00Z', updated_at: '2026-01-01T00:00:00Z',
    charge_count: 189, total_revenue: 9450,
  },
  {
    id: 3, price_yuan: 100, points: 1200, bonus_points: 200, sort_order: 3,
    is_active: true, effective_at: '2026-01-01T00:00:00Z', expires_at: '2027-01-01T00:00:00Z',
    created_at: '2026-01-01T00:00:00Z', updated_at: '2026-01-01T00:00:00Z',
    charge_count: 87, total_revenue: 8700,
  },
  {
    id: 4, price_yuan: 200, points: 2500, bonus_points: 500, sort_order: 4,
    is_active: false, effective_at: '2026-01-01T00:00:00Z', expires_at: '2027-01-01T00:00:00Z',
    created_at: '2026-01-01T00:00:00Z', updated_at: '2026-01-01T00:00:00Z',
    charge_count: 0, total_revenue: 0,
  },
]

const paymentMethods = ['WeChat Pay', 'Alipay', 'Blue Whale Pay']

function wrap<T>(data: T) { return { success: true, message: '', data } }
function delay(ms = 200) { return new Promise((r) => setTimeout(r, ms)) }

export async function fetchTiers(params?: {
  status?: string
  page?: number
  pageSize?: number
}) {
  await delay()
  let filtered = [...tiers]
  if (params?.status === 'active') filtered = filtered.filter((t) => t.is_active)
  if (params?.status === 'inactive') filtered = filtered.filter((t) => !t.is_active)
  filtered.sort((a, b) => a.sort_order - b.sort_order)
  return wrap({ tiers: filtered, total: filtered.length })
}

export async function createTier(values: TierFormValues) {
  await delay()
  const tier: TopupTier = {
    id: ++idSeq, ...values,
    is_active: true, charge_count: 0, total_revenue: 0,
    created_at: new Date().toISOString(), updated_at: new Date().toISOString(),
  }
  tiers.push(tier)
  return wrap(tier)
}

export async function updateTier(id: number, values: Partial<TierFormValues>) {
  await delay()
  const idx = tiers.findIndex((t) => t.id === id)
  if (idx === -1) return { success: false, message: 'Not found', data: null }
  tiers[idx] = { ...tiers[idx], ...values, updated_at: new Date().toISOString() }
  return wrap(tiers[idx])
}

export async function deleteTier(id: number) {
  await delay()
  const t = tiers.find((t) => t.id === id)
  if (t?.charge_count) return { success: false, message: 'Tier has charge records, cannot delete', data: null }
  tiers = tiers.filter((t) => t.id !== id)
  return wrap(null)
}

export async function toggleTierStatus(id: number) {
  await delay()
  const t = tiers.find((t) => t.id === id)
  if (!t) return { success: false, message: 'Not found', data: null }
  t.is_active = !t.is_active
  t.updated_at = new Date().toISOString()
  return wrap(t)
}

export async function updateSortOrder(items: { id: number; sort_order: number }[]) {
  await delay()
  for (const item of items) {
    const t = tiers.find((t) => t.id === item.id)
    if (t) t.sort_order = item.sort_order
  }
  return wrap(null)
}

export async function fetchTierPreview() {
  await delay()
  const active = tiers.filter((t) => t.is_active)
  const preview: TierPreviewItem[] = active.map((t) => ({
    id: t.id, price_yuan: t.price_yuan, points: t.points,
    bonus_points: t.bonus_points, payment_methods: paymentMethods,
  }))
  return wrap(preview)
}
