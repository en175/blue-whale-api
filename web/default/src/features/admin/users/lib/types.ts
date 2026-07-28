export interface AdminUserFull {
  id: number
  username: string
  display_name: string
  email: string
  role: number
  points_balance: number
  total_charged: number
  total_consumed: number
  is_active: boolean
  created_at: string
  last_login_at: string
}

export interface AdjustLog {
  id: number
  user_id: number
  username: string
  amount: number
  reason: string
  operator: string
  created_at: string
}
