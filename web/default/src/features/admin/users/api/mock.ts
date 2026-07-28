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
import type { AdjustLog, AdminUserFull } from '../lib/types'

const users: AdminUserFull[] = [
  { id: 1, username: 'admin', display_name: 'Admin', email: 'admin@example.com', role: 100, points_balance: 0, total_charged: 0, total_consumed: 0, is_active: true, created_at: '2026-01-01T00:00:00Z', last_login_at: '2026-07-28T10:30:00Z' },
  { id: 2, username: 'zhangsan', display_name: '张三', email: 'zhangsan@example.com', role: 1, points_balance: 5000, total_charged: 10000, total_consumed: 5000, is_active: true, created_at: '2026-03-15T08:00:00Z', last_login_at: '2026-07-28T09:00:00Z' },
  { id: 3, username: 'lisi', display_name: '李四', email: 'lisi@example.com', role: 1, points_balance: 200, total_charged: 5000, total_consumed: 4800, is_active: true, created_at: '2026-04-20T12:00:00Z', last_login_at: '2026-07-27T18:00:00Z' },
  { id: 4, username: 'wangwu', display_name: '王五', email: 'wangwu@example.com', role: 1, points_balance: 0, total_charged: 1000, total_consumed: 1000, is_active: false, created_at: '2026-05-10T14:00:00Z', last_login_at: '2026-06-15T11:00:00Z' },
  { id: 5, username: 'opadmin', display_name: '运营小王', email: 'op@example.com', role: 10, points_balance: 0, total_charged: 0, total_consumed: 0, is_active: true, created_at: '2026-02-01T09:00:00Z', last_login_at: '2026-07-28T07:00:00Z' },
  { id: 6, username: 'guest_01', display_name: '游客01', email: 'guest01@example.com', role: 0, points_balance: 50, total_charged: 0, total_consumed: 0, is_active: true, created_at: '2026-07-01T10:00:00Z', last_login_at: '2026-07-20T08:00:00Z' },
  { id: 7, username: 'test_user', display_name: '测试用户', email: 'test@example.com', role: 1, points_balance: 10000, total_charged: 50000, total_consumed: 40000, is_active: true, created_at: '2026-01-20T08:00:00Z', last_login_at: '2026-07-28T11:00:00Z' },
  { id: 8, username: 'vip_user', display_name: 'VIP会员', email: 'vip@example.com', role: 1, points_balance: 50000, total_charged: 100000, total_consumed: 50000, is_active: true, created_at: '2026-02-10T08:00:00Z', last_login_at: '2026-07-28T12:00:00Z' },
]

function wrap<T>(data: T, total?: number) { return { success: true, message: '', data, total: total ?? (Array.isArray(data) ? data.length : undefined) } }
function delay(ms = 200) { return new Promise((r) => setTimeout(r, ms)) }

export async function fetchUsers(params?: { search?: string; role?: string; page?: number; pageSize?: number }) {
  await delay()
  let filtered = [...users]
  if (params?.search) {
    const q = params.search.toLowerCase()
    filtered = filtered.filter((u) => u.username.toLowerCase().includes(q) || u.display_name.includes(q) || u.email.toLowerCase().includes(q))
  }
  if (params?.role && params.role !== 'all') {
    filtered = filtered.filter((u) => u.role === Number(params.role))
  }
  const page = params?.page ?? 1
  const pageSize = params?.pageSize ?? 20
  const start = (page - 1) * pageSize
  return wrap(filtered.slice(start, start + pageSize), filtered.length)
}

export async function fetchUserDetail(id: number) {
  await delay()
  const u = users.find((u) => u.id === id)
  return u ? wrap(u) : { success: false, message: 'Not found', data: null }
}

export async function toggleUserStatus(id: number) {
  await delay()
  const u = users.find((u) => u.id === id)
  if (!u) return { success: false, message: 'Not found', data: null }
  u.is_active = !u.is_active
  return wrap(u)
}

let logSeq = 100
const adjustLogs: AdjustLog[] = [
  { id: 101, user_id: 2, username: 'zhangsan', amount: 1000, reason: '活动赠送', operator: 'admin', created_at: '2026-07-20T10:00:00Z' },
  { id: 102, user_id: 3, username: 'lisi', amount: -200, reason: '异常消费扣除', operator: 'admin', created_at: '2026-07-20T14:00:00Z' },
  { id: 103, user_id: 7, username: 'test_user', amount: 5000, reason: '批量导入', operator: 'admin', created_at: '2026-07-15T09:00:00Z' },
  { id: 104, user_id: 4, username: 'wangwu', amount: 500, reason: '客服补偿', operator: 'opadmin', created_at: '2026-07-10T16:00:00Z' },
  { id: 105, user_id: 8, username: 'vip_user', amount: 10000, reason: 'VIP 充值赠送', operator: 'opadmin', created_at: '2026-07-28T08:00:00Z' },
]

export async function adjustUserPoints(userId: number, amount: number, reason: string) {
  await delay(400)
  const u = users.find((u) => u.id === userId)
  if (!u) return { success: false, message: 'User not found', data: null }
  if (u.points_balance + amount < 0) return { success: false, message: 'Insufficient balance', data: null }
  u.points_balance += amount
  if (amount > 0) u.total_charged += amount
  else u.total_consumed += Math.abs(amount)
  const log: AdjustLog = {
    id: ++logSeq,
    user_id: u.id,
    username: u.username,
    amount,
    reason,
    operator: 'admin',
    created_at: new Date().toISOString(),
  }
  adjustLogs.unshift(log)
  return wrap(u)
}

export async function fetchAdjustLogs(params?: { userId?: number; page?: number; pageSize?: number }) {
  await delay()
  let filtered = [...adjustLogs]
  if (params?.userId) filtered = filtered.filter((l) => l.user_id === params.userId)
  filtered.sort((a, b) => b.created_at.localeCompare(a.created_at))
  const page = params?.page ?? 1
  const pageSize = params?.pageSize ?? 20
  const start = (page - 1) * pageSize
  return wrap(filtered.slice(start, start + pageSize), filtered.length)
}
