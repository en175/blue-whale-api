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

import { getRoleLabelKey } from '@/lib/roles'
import { type AdminUserFull } from '../lib/types'

function formatDate(iso: string, locale: string) {
  return new Date(iso).toLocaleDateString(locale)
}

const ROLE_VARIANTS: Record<string, 'default' | 'secondary' | 'outline'> = {
  'Super Admin': 'default',
  Admin: 'secondary',
  User: 'outline',
  Guest: 'secondary',
}

interface UserTableProps {
  users: AdminUserFull[]
  search: string
  onSearchChange: (v: string) => void
  onView: (user: AdminUserFull) => void
  onToggle: (user: AdminUserFull) => void
  onAdjust: (user: AdminUserFull) => void
  total: number
  page: number
  pageSize: number
  onPageChange: (p: number) => void
}

export function UserTable(props: UserTableProps) {
  const { t, i18n } = useTranslation()
  const totalPages = Math.max(1, Math.ceil(props.total / props.pageSize))

  const columns = [
    { key: 'id', label: 'ID', format: (u: AdminUserFull) => `#${u.id}` },
    { key: 'username', label: 'Username', format: (u: AdminUserFull) => u.username },
    { key: 'display_name', label: 'Display Name', format: (u: AdminUserFull) => u.display_name },
    { key: 'email', label: 'Email', format: (u: AdminUserFull) => u.email },
    { key: 'role', label: 'Role', format: (u: AdminUserFull) => {
      const label = t(getRoleLabelKey(u.role))
      return <Badge variant={ROLE_VARIANTS[label] ?? 'outline'}>{label}</Badge>
    }},
    { key: 'points_balance', label: 'Points Balance', format: (u: AdminUserFull) => u.points_balance.toLocaleString() },
    { key: 'status', label: 'Status', format: (u: AdminUserFull) =>
      u.is_active
        ? <Badge variant="default">{t('Active')}</Badge>
        : <Badge variant="secondary">{t('Inactive')}</Badge>
    },
    { key: 'last_login', label: 'Last Login', format: (u: AdminUserFull) => formatDate(u.last_login_at, i18n.language) },
  ]

  return (
    <div className="space-y-4">
      <div className="flex items-center gap-2">
        <Search className="text-muted-foreground size-4" />
        <Input
          placeholder={t('Search users...')}
          value={props.search}
          onChange={(e) => props.onSearchChange(e.target.value)}
          className="max-w-xs"
        />
        <span className="text-muted-foreground ml-auto text-sm">
          {t('{{total}} users', { total: props.total })}
        </span>
      </div>
      <Table>
        <TableHeader>
          <TableRow>
            {columns.map((col) => (
              <TableHead key={col.key}>{t(col.label)}</TableHead>
            ))}
            <TableHead className="w-32 text-right">{t('Actions')}</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {props.users.map((user) => (
            <TableRow key={user.id}>
              {columns.map((col) => (
                <TableCell key={col.key}>{col.format(user)}</TableCell>
              ))}
              <TableCell className="text-right">
                <div className="flex items-center justify-end gap-1">
                  <Button variant="outline" size="sm" onClick={() => props.onView(user)}>
                    {t('Detail')}
                  </Button>
                  <Button variant="outline" size="sm" onClick={() => props.onAdjust(user)}>
                    {t('Adjust')}
                  </Button>
                  <Button variant="outline" size="sm" onClick={() => props.onToggle(user)}>
                    {user.is_active ? t('Disable') : t('Enable')}
                  </Button>
                </div>
              </TableCell>
            </TableRow>
          ))}
          {props.users.length === 0 && (
            <TableRow>
              <TableCell colSpan={columns.length + 1} className="text-muted-foreground py-8 text-center">
                {t('No users found')}
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
