import { createFileRoute } from '@tanstack/react-router'

import { TiersPage } from '@/features/admin/tiers'

export const Route = createFileRoute('/_authenticated/admin/tiers')({
  component: TiersPage,
})
