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
import type { Order } from '../lib/types'

const orders: Order[] = [
  { id: 1, order_no: 'BW202607280001', user_id: 2, username: 'zhangsan', tier_id: 1, price_yuan: 10, points: 100, bonus_points: 0, payment_method: 'WeChat Pay', status: 'completed', created_at: '2026-07-28T10:30:00Z', paid_at: '2026-07-28T10:30:15Z' },
  { id: 2, order_no: 'BW202607280002', user_id: 8, username: 'vip_user', tier_id: 4, price_yuan: 200, points: 2500, bonus_points: 500, payment_method: 'Alipay', status: 'completed', created_at: '2026-07-28T11:00:00Z', paid_at: '2026-07-28T11:00:20Z' },
  { id: 3, order_no: 'BW202607270003', user_id: 3, username: 'lisi', tier_id: 2, price_yuan: 50, points: 550, bonus_points: 50, payment_method: 'Blue Whale Pay', status: 'completed', created_at: '2026-07-27T18:00:00Z', paid_at: '2026-07-27T18:00:12Z' },
  { id: 4, order_no: 'BW202607280004', user_id: 7, username: 'test_user', tier_id: 3, price_yuan: 100, points: 1200, bonus_points: 200, payment_method: 'WeChat Pay', status: 'pending', created_at: '2026-07-28T14:00:00Z', paid_at: null },
  { id: 5, order_no: 'BW202607280005', user_id: 2, username: 'zhangsan', tier_id: 3, price_yuan: 100, points: 1200, bonus_points: 200, payment_method: 'Alipay', status: 'failed', created_at: '2026-07-28T09:00:00Z', paid_at: null },
  { id: 6, order_no: 'BW202607260006', user_id: 8, username: 'vip_user', tier_id: 4, price_yuan: 200, points: 2500, bonus_points: 500, payment_method: 'Blue Whale Pay', status: 'completed', created_at: '2026-07-26T08:00:00Z', paid_at: '2026-07-26T08:00:25Z' },
  { id: 7, order_no: 'BW202607250007', user_id: 3, username: 'lisi', tier_id: 1, price_yuan: 10, points: 100, bonus_points: 0, payment_method: 'WeChat Pay', status: 'refunded', created_at: '2026-07-25T15:00:00Z', paid_at: '2026-07-25T15:00:10Z' },
  { id: 8, order_no: 'BW202607280008', user_id: 7, username: 'test_user', tier_id: 2, price_yuan: 50, points: 550, bonus_points: 50, payment_method: 'WeChat Pay', status: 'completed', created_at: '2026-07-28T12:30:00Z', paid_at: '2026-07-28T12:30:18Z' },
]

function wrap<T>(data: T, total?: number) { return { success: true, message: '', data, total: total ?? (Array.isArray(data) ? data.length : 0) } }
function delay(ms = 200) { return new Promise((r) => setTimeout(r, ms)) }

export async function fetchOrders(params?: { search?: string; status?: string; page?: number; pageSize?: number }) {
  await delay()
  let filtered = [...orders]
  if (params?.search) {
    const q = params.search.toLowerCase()
    filtered = filtered.filter((o) => o.order_no.toLowerCase().includes(q) || o.username.includes(q))
  }
  if (params?.status && params.status !== 'all') {
    filtered = filtered.filter((o) => o.status === params.status)
  }
  filtered.sort((a, b) => b.created_at.localeCompare(a.created_at))
  const page = params?.page ?? 1
  const pageSize = params?.pageSize ?? 20
  const start = (page - 1) * pageSize
  return wrap(filtered.slice(start, start + pageSize), filtered.length)
}

export async function fetchOrderDetail(id: number) {
  await delay()
  const o = orders.find((o) => o.id === id)
  return o ? wrap(o) : { success: false, message: 'Not found', data: null }
}
