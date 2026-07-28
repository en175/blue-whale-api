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
import type { DashboardStats } from '../lib/types'

let dashboardStats: DashboardStats = {
  total_users: 2847,
  today_recharge_amount: 15800,
  today_recharge_count: 47,
  today_points_consumed: 235000,
  pending_orders: 12,
}

function wrap<T>(data: T) {
  return { success: true, message: '', data }
}

function delay(ms = 300) {
  return new Promise((resolve) => setTimeout(resolve, ms))
}

export async function fetchDashboardStats(): Promise<{
  success: boolean
  message: string
  data: DashboardStats
}> {
  await delay()
  return wrap({ ...dashboardStats })
}

export function updateDashboardStats(partial: Partial<DashboardStats>) {
  dashboardStats = { ...dashboardStats, ...partial }
}
