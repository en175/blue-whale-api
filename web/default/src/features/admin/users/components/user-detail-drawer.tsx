import { useTranslation } from 'react-i18next'

import { Badge } from '@/components/ui/badge'
import {
  Drawer,
  DrawerContent,
  DrawerHeader,
  DrawerTitle,
} from '@/components/ui/drawer'
import { Skeleton } from '@/components/ui/skeleton'

import { getRoleLabelKey } from '@/lib/roles'
import { type AdminUserFull } from '../lib/types'
import { fetchUserDetail } from '../api/mock'
import { useEffect, useState } from 'react'

interface UserDetailDrawerProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  userId: number | null
}

function formatDate(iso: string, locale: string) {
  return new Date(iso).toLocaleString(locale)
}

export function UserDetailDrawer(props: UserDetailDrawerProps) {
  const { t, i18n } = useTranslation()
  const [user, setUser] = useState<AdminUserFull | null>(null)
  const [loading, setLoading] = useState(false)

  useEffect(() => {
    if (props.open && props.userId) {
      setLoading(true)
      fetchUserDetail(props.userId).then((res) => {
        setUser(res.data ?? null)
        setLoading(false)
      })
    }
  }, [props.open, props.userId])

  return (
    <Drawer open={props.open} onOpenChange={props.onOpenChange}>
      <DrawerContent>
        <div className="mx-auto w-full max-w-lg p-6">
          <DrawerHeader>
            <DrawerTitle>{t('User Detail')}</DrawerTitle>
          </DrawerHeader>
          {loading ? (
            <div className="space-y-3">
              <Skeleton className="h-6 w-40" />
              <Skeleton className="h-6 w-60" />
              <Skeleton className="h-6 w-32" />
              <Skeleton className="h-6 w-48" />
            </div>
          ) : user ? (
            <div className="space-y-4">
              <div className="grid grid-cols-2 gap-4">
                <div>
                  <div className="text-muted-foreground text-xs">{t('Username')}</div>
                  <div className="font-medium">{user.username}</div>
                </div>
                <div>
                  <div className="text-muted-foreground text-xs">{t('Display Name')}</div>
                  <div className="font-medium">{user.display_name}</div>
                </div>
                <div>
                  <div className="text-muted-foreground text-xs">{t('Email')}</div>
                  <div className="font-medium">{user.email}</div>
                </div>
                <div>
                  <div className="text-muted-foreground text-xs">{t('Role')}</div>
                  <Badge variant="outline">{t(getRoleLabelKey(user.role))}</Badge>
                </div>
                <div>
                  <div className="text-muted-foreground text-xs">{t('Points Balance')}</div>
                  <div className="font-medium">{user.points_balance.toLocaleString()}</div>
                </div>
                <div>
                  <div className="text-muted-foreground text-xs">{t('Status')}</div>
                  <Badge variant={user.is_active ? 'default' : 'secondary'}>
                    {user.is_active ? t('Active') : t('Inactive')}
                  </Badge>
                </div>
                <div>
                  <div className="text-muted-foreground text-xs">{t('Total Charged')}</div>
                  <div className="font-medium">{user.total_charged.toLocaleString()}</div>
                </div>
                <div>
                  <div className="text-muted-foreground text-xs">{t('Total Consumed')}</div>
                  <div className="font-medium">{user.total_consumed.toLocaleString()}</div>
                </div>
                <div>
                  <div className="text-muted-foreground text-xs">{t('Created At')}</div>
                  <div className="font-medium">{formatDate(user.created_at, i18n.language)}</div>
                </div>
                <div>
                  <div className="text-muted-foreground text-xs">{t('Last Login')}</div>
                  <div className="font-medium">{formatDate(user.last_login_at, i18n.language)}</div>
                </div>
              </div>
            </div>
          ) : (
            <p className="text-muted-foreground text-sm">{t('User not found')}</p>
          )}
        </div>
      </DrawerContent>
    </Drawer>
  )
}
