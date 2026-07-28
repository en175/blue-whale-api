import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Badge } from '@/components/ui/badge'
import {
  Drawer,
  DrawerContent,
  DrawerHeader,
  DrawerTitle,
} from '@/components/ui/drawer'
import { Skeleton } from '@/components/ui/skeleton'

import { ORDER_STATUS_LABELS, type Order } from '../lib/types'
import { fetchOrderDetail } from '../api/mock'

function formatDate(iso: string | null, locale: string) {
  return iso ? new Date(iso).toLocaleString(locale) : '—'
}

const STATUS_VARIANTS: Record<string, 'default' | 'secondary' | 'outline'> = {
  completed: 'default',
  pending: 'secondary',
  failed: 'outline',
  refunded: 'outline',
}

interface OrderDetailDrawerProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  orderId: number | null
}

export function OrderDetailDrawer(props: OrderDetailDrawerProps) {
  const { t, i18n } = useTranslation()
  const [order, setOrder] = useState<Order | null>(null)
  const [loading, setLoading] = useState(false)

  useEffect(() => {
    if (props.open && props.orderId) {
      setLoading(true)
      fetchOrderDetail(props.orderId).then((res) => {
        setOrder(res.data ?? null)
        setLoading(false)
      })
    }
  }, [props.open, props.orderId])

  return (
    <Drawer open={props.open} onOpenChange={props.onOpenChange}>
      <DrawerContent>
        <div className="mx-auto w-full max-w-lg p-6">
          <DrawerHeader>
            <DrawerTitle>{t('Order Detail')}</DrawerTitle>
          </DrawerHeader>
          {loading ? (
            <div className="space-y-3">
              <Skeleton className="h-6 w-60" />
              <Skeleton className="h-6 w-40" />
              <Skeleton className="h-6 w-32" />
              <Skeleton className="h-6 w-48" />
            </div>
          ) : order ? (
            <div className="space-y-4">
              <div className="grid grid-cols-2 gap-4">
                <div>
                  <div className="text-muted-foreground text-xs">{t('Order No.')}</div>
                  <div className="font-mono text-sm font-medium">{order.order_no}</div>
                </div>
                <div>
                  <div className="text-muted-foreground text-xs">{t('Status')}</div>
                  <Badge variant={STATUS_VARIANTS[order.status] ?? 'outline'}>
                    {t(ORDER_STATUS_LABELS[order.status] ?? order.status)}
                  </Badge>
                </div>
                <div>
                  <div className="text-muted-foreground text-xs">{t('Username')}</div>
                  <div className="font-medium">{order.username}</div>
                </div>
                <div>
                  <div className="text-muted-foreground text-xs">{t('Amount')}</div>
                  <div className="font-medium">¥{order.price_yuan.toLocaleString()}</div>
                </div>
                <div>
                  <div className="text-muted-foreground text-xs">{t('Points')}</div>
                  <div className="font-medium">{order.points.toLocaleString()}{order.bonus_points > 0 ? ` (+${order.bonus_points})` : ''}</div>
                </div>
                <div>
                  <div className="text-muted-foreground text-xs">{t('Payment')}</div>
                  <div className="font-medium">{order.payment_method}</div>
                </div>
                <div>
                  <div className="text-muted-foreground text-xs">{t('Created')}</div>
                  <div className="font-medium">{formatDate(order.created_at, i18n.language)}</div>
                </div>
                <div>
                  <div className="text-muted-foreground text-xs">{t('Paid At')}</div>
                  <div className="font-medium">{formatDate(order.paid_at, i18n.language)}</div>
                </div>
              </div>
            </div>
          ) : (
            <p className="text-muted-foreground text-sm">{t('Order not found')}</p>
          )}
        </div>
      </DrawerContent>
    </Drawer>
  )
}
