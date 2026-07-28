import { z } from 'zod'

export const tierFormSchema = z
  .object({
    price_yuan: z.number().positive('Price must be positive'),
    points: z.number().int().positive('Points must be positive'),
    bonus_points: z.number().int().nonnegative(),
    sort_order: z.number().int().nonnegative(),
    effective_at: z.string().min(1, 'Start date is required'),
    expires_at: z.string().min(1, 'End date is required'),
  })
  .refine((d) => d.points >= d.bonus_points, {
    message: 'Points must be >= bonus points',
    path: ['bonus_points'],
  })
  .refine((d) => d.effective_at < d.expires_at, {
    message: 'Start must be before end',
    path: ['expires_at'],
  })

export type TierFormValues = z.infer<typeof tierFormSchema>
