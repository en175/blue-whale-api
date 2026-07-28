import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useQuery } from '@tanstack/react-query'

import { fetchOrders } from '../api/mock'
import type { Order } from '../lib/types'
import { OrderDetailDrawer } from './order-detail-drawer'
import { OrderTable } from './order-table'

const PAGE_SIZE = 10

export function OrdersPage() {
  const { t } = useTranslation()
  const [search, setSearch] = useState('')
  const [statusFilter, setStatusFilter] = useState('all')
  const [page, setPage] = useState(1)
  const [detailOrderId, setDetailOrderId] = useState<number | null>(null)
  const [detailOpen, setDetailOpen] = useState(false)

  const { data, isLoading } = useQuery({
    queryKey: ['admin', 'orders', { search, status: statusFilter, page, pageSize: PAGE_SIZE }],
    queryFn: () => fetchOrders({ search, status: statusFilter, page, pageSize: PAGE_SIZE }),
    select: (res) => ({ orders: res.data ?? [], total: res.total ?? 0 }),
  })

  const orders = data?.orders ?? []
  const total = data?.total ?? 0

  function handleSearch(v: string) {
    setSearch(v)
    setPage(1)
  }

  function handleStatusChange(v: string) {
    setStatusFilter(v)
    setPage(1)
  }

  function handlePageChange(p: number) {
    setPage(p)
  }

  function handleView(order: Order) {
    setDetailOrderId(order.id)
    setDetailOpen(true)
  }

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-bold tracking-tight">
          {t('Order Management')}
        </h1>
      </div>

      <OrderTable
        orders={isLoading ? [] : orders}
        search={search}
        statusFilter={statusFilter}
        onSearchChange={handleSearch}
        onStatusChange={handleStatusChange}
        onView={handleView}
        total={total}
        page={page}
        pageSize={PAGE_SIZE}
        onPageChange={handlePageChange}
      />

      <OrderDetailDrawer
        open={detailOpen}
        onOpenChange={setDetailOpen}
        orderId={detailOrderId}
      />
    </div>
  )
}
