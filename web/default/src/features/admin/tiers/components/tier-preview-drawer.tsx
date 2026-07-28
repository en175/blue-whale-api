import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Drawer,
  DrawerContent,
  DrawerHeader,
  DrawerTitle,
  DrawerTrigger,
} from '@/components/ui/drawer'
import { Skeleton } from '@/components/ui/skeleton'

import { fetchTierPreview } from '../api/mock'
import type { TierPreviewItem } from '../lib/types'

export function TierPreviewDrawer() {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)
  const [items, setItems] = useState<TierPreviewItem[]>([])
  const [loading, setLoading] = useState(false)

  async function handleOpen() {
    setOpen(true)
    if (items.length === 0) {
      setLoading(true)
      const res = await fetchTierPreview()
      setItems(res.data ?? [])
      setLoading(false)
    }
  }

  return (
    <Drawer open={open} onOpenChange={setOpen}>
      <DrawerTrigger asChild>
        <Button variant="outline" onClick={handleOpen}>
          {t('Preview')}
        </Button>
      </DrawerTrigger>
      <DrawerContent>
        <div className="mx-auto w-full max-w-md p-6">
          <DrawerHeader>
            <DrawerTitle>{t('User-Facing Tier Preview')}</DrawerTitle>
          </DrawerHeader>
          {loading ? (
            <div className="space-y-3">
              <Skeleton className="h-16 w-full" />
              <Skeleton className="h-16 w-full" />
              <Skeleton className="h-16 w-full" />
            </div>
          ) : items.length === 0 ? (
            <p className="text-muted-foreground text-sm">{t('No active tiers')}</p>
          ) : (
            <div className="space-y-3">
              {items.map((item) => (
                <div
                  key={item.id}
                  className="flex items-center justify-between rounded-lg border p-4"
                >
                  <div>
                    <div className="flex items-center gap-2">
                      <span className="text-lg font-bold">
                        ¥{item.price_yuan}
                      </span>
                      <Badge variant="secondary">
                        {t('{{points}} pts', { points: item.points })}
                      </Badge>
                    </div>
                    {item.bonus_points > 0 && (
                      <p className="text-muted-foreground mt-1 text-xs">
                        {t('+{{bonus}} bonus', { bonus: item.bonus_points })}
                      </p>
                    )}
                    <div className="mt-1 flex gap-1">
                      {item.payment_methods.map((pm) => (
                        <Badge key={pm} variant="outline" className="text-[10px]">
                          {pm}
                        </Badge>
                      ))}
                    </div>
                  </div>
                </div>
              ))}
            </div>
          )}
        </div>
      </DrawerContent>
    </Drawer>
  )
}
