import { createFileRoute } from '@tanstack/react-router'
import { SettingsPage } from '@/features/admin/settings'

export const Route = createFileRoute('/_authenticated/admin/settings')({
  component: SettingsPage,
})
