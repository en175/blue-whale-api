export interface PricingRule {
  id: number
  model: string
  input_price: number
  output_price: number
  unit: string
  ratio: number
  is_active: boolean
  created_at: string
  updated_at: string
}
