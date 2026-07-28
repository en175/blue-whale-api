export interface TopupTier {
  id: number
  price_yuan: number
  points: number
  bonus_points: number
  sort_order: number
  is_active: boolean
  effective_at: string
  expires_at: string
  created_at: string
  updated_at: string
  charge_count: number
  total_revenue: number
}

export interface TierListResponse {
  tiers: TopupTier[]
  total: number
}

export interface TierPreviewItem {
  id: number
  price_yuan: number
  points: number
  bonus_points: number
  payment_methods: string[]
}
