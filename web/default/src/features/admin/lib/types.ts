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
export interface AdminUser {
  id: number
  username: string
  display_name: string
  role: number
  permissions: string[]
}

export interface DashboardStats {
  total_users: number
  today_recharge_amount: number
  today_recharge_count: number
  today_points_consumed: number
  pending_orders: number
}

export interface AdminNavItem {
  title: string
  to: string
  icon: string
}
