import { createFileRoute } from '@tanstack/react-router'
import { PricingPage } from '@/features/admin/pricing'

export const Route = createFileRoute('/_authenticated/admin/pricing')({
  component: PricingPage,
})
