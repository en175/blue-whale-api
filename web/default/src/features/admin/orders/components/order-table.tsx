import { Search } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'

import { ORDER_STATUS_LABELS, type Order } from '../lib/types'

function formatDate(iso: string | null, locale: string) {
  return iso ? new Date(iso).toLocaleString(locale) : '—'
}

const STATUS_VARIANTS: Record<string, 'default' | 'secondary' | 'outline'> = {
  completed: 'default',
  pending: 'secondary',
  failed: 'outline',
  refunded: 'outline',
}

interface OrderTableProps {
  orders: Order[]
  search: string
  statusFilter: string
  onSearchChange: (v: string) => void
  onStatusChange: (v: string) => void
  onView: (order: Order) => void
  total: number
  page: number
  pageSize: number
  onPageChange: (p: number) => void
}

export function OrderTable(props: OrderTableProps) {
  const { t, i18n } = useTranslation()
  const totalPages = Math.max(1, Math.ceil(props.total / props.pageSize))

  const columns = [
    { key: 'order_no', label: 'Order No.', format: (o: Order) => <span className="font-mono text-xs">{o.order_no}</span> },
    { key: 'username', label: 'Username', format: (o: Order) => o.username },
    { key: 'price_yuan', label: 'Amount', format: (o: Order) => `¥${o.price_yuan.toLocaleString()}` },
    { key: 'points', label: 'Points', format: (o: Order) => `${o.points.toLocaleString()}${o.bonus_points > 0 ? ` (+${o.bonus_points})` : ''}` },
    { key: 'payment_method', label: 'Payment', format: (o: Order) => o.payment_method },
    { key: 'status', label: 'Status', format: (o: Order) =>
      <Badge variant={STATUS_VARIANTS[o.status] ?? 'outline'}>{t(ORDER_STATUS_LABELS[o.status] ?? o.status)}</Badge>
    },
    { key: 'created_at', label: 'Created', format: (o: Order) => formatDate(o.created_at, i18n.language) },
  ]

  return (
    <div className="space-y-4">
      <div className="flex items-center gap-2">
        <Search className="text-muted-foreground size-4" />
        <Input
          placeholder={t('Search orders...')}
          value={props.search}
          onChange={(e) => props.onSearchChange(e.target.value)}
          className="max-w-xs"
        />
        <Select value={props.statusFilter} onValueChange={(v) => v && props.onStatusChange(v)}>
          <SelectTrigger className="w-32">
            <SelectValue placeholder={t('All')} />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="all">{t('All')}</SelectItem>
            <SelectItem value="pending">{t('Pending')}</SelectItem>
            <SelectItem value="completed">{t('Completed')}</SelectItem>
            <SelectItem value="failed">{t('Failed')}</SelectItem>
            <SelectItem value="refunded">{t('Refunded')}</SelectItem>
          </SelectContent>
        </Select>
        <span className="text-muted-foreground ml-auto text-sm">
          {t('{{total}} orders', { total: props.total })}
        </span>
      </div>
      <Table>
        <TableHeader>
          <TableRow>
            {columns.map((col) => (
              <TableHead key={col.key}>{t(col.label)}</TableHead>
            ))}
            <TableHead className="w-24 text-right">{t('Actions')}</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {props.orders.map((order) => (
            <TableRow key={order.id}>
              {columns.map((col) => (
                <TableCell key={col.key}>{col.format(order)}</TableCell>
              ))}
              <TableCell className="text-right">
                <Button variant="outline" size="sm" onClick={() => props.onView(order)}>
                  {t('Detail')}
                </Button>
              </TableCell>
            </TableRow>
          ))}
          {props.orders.length === 0 && (
            <TableRow>
              <TableCell colSpan={columns.length + 1} className="text-muted-foreground py-8 text-center">
                {t('No orders found')}
              </TableCell>
            </TableRow>
          )}
        </TableBody>
      </Table>
      {totalPages > 1 && (
        <div className="flex items-center justify-end gap-2">
          <Button variant="outline" size="sm" disabled={props.page <= 1} onClick={() => props.onPageChange(props.page - 1)}>
            {t('Previous')}
          </Button>
          <span className="text-sm">{props.page} / {totalPages}</span>
          <Button variant="outline" size="sm" disabled={props.page >= totalPages} onClick={() => props.onPageChange(props.page + 1)}>
            {t('Next')}
          </Button>
        </div>
      )}
    </div>
  )
}
