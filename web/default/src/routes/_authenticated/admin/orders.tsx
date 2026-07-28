import { createFileRoute } from '@tanstack/react-router'
import { OrdersPage } from '@/features/admin/orders'

export const Route = createFileRoute('/_authenticated/admin/orders')({
  component: OrdersPage,
})
