import { createFileRoute } from '@tanstack/react-router'
import { UsersPage } from '@/features/admin/users'

export const Route = createFileRoute('/_authenticated/admin/users')({
  component: UsersPage,
})
