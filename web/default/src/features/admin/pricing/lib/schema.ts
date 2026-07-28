import { z } from 'zod'

export const pricingFormSchema = z.object({
  model: z.string().min(1, 'Model is required'),
  input_price: z.number().positive('Must be positive'),
  output_price: z.number().positive('Must be positive'),
  unit: z.string().min(1, 'Unit is required'),
  ratio: z.number().positive('Must be positive'),
})

export type PricingFormValues = z.infer<typeof pricingFormSchema>
