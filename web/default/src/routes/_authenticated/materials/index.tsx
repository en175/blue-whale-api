import { createFileRoute } from '@tanstack/react-router'

import { Materials } from '@/features/materials'

export const Route = createFileRoute('/_authenticated/materials/')({
  component: Materials,
})
