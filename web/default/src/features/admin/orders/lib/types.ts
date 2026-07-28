export interface Order {
  id: number
  order_no: string
  user_id: number
  username: string
  tier_id: number
  price_yuan: number
  points: number
  bonus_points: number
  payment_method: string
  status: 'pending' | 'completed' | 'failed' | 'refunded'
  created_at: string
  paid_at: string | null
}

export const ORDER_STATUS_LABELS: Record<string, string> = {
  pending: 'Pending',
  completed: 'Completed',
  failed: 'Failed',
  refunded: 'Refunded',
}
