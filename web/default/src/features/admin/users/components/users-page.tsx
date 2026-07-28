import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'

import { adjustUserPoints, fetchAdjustLogs, fetchUsers, toggleUserStatus } from '../api/mock'
import type { AdminUserFull } from '../lib/types'
import { AdjustDialog } from './adjust-dialog'
import { AdjustLogsTable } from './adjust-logs-table'
import { UserDetailDrawer } from './user-detail-drawer'
import { UserTable } from './user-table'

const PAGE_SIZE = 10

export function UsersPage() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [search, setSearch] = useState('')
  const [page, setPage] = useState(1)
  const [detailUserId, setDetailUserId] = useState<number | null>(null)
  const [detailOpen, setDetailOpen] = useState(false)
  const [adjustUser, setAdjustUser] = useState<AdminUserFull | null>(null)
  const [adjustOpen, setAdjustOpen] = useState(false)
  const [logsPage, setLogsPage] = useState(1)

  const { data: usersData } = useQuery({
    queryKey: ['admin', 'users', { search, page, pageSize: PAGE_SIZE }],
    queryFn: () => fetchUsers({ search, page, pageSize: PAGE_SIZE }),
    select: (res) => ({ users: res.data ?? [], total: res.total ?? 0 }),
  })
  const users = usersData?.users ?? []
  const total = usersData?.total ?? 0

  const { data: logsData } = useQuery({
    queryKey: ['admin', 'adjust-logs', { page: logsPage, pageSize: PAGE_SIZE }],
    queryFn: () => fetchAdjustLogs({ page: logsPage, pageSize: PAGE_SIZE }),
    select: (res) => ({ logs: res.data ?? [], total: res.total ?? 0 }),
  })
  const logs = logsData?.logs ?? []
  const logsTotal = logsData?.total ?? 0

  const invalidateUsers = () => queryClient.invalidateQueries({ queryKey: ['admin', 'users'] })
  const invalidateLogs = () => queryClient.invalidateQueries({ queryKey: ['admin', 'adjust-logs'] })

  const onError = () => toast.error(t('Operation failed'))

  const toggleMutation = useMutation({
    mutationFn: (id: number) => toggleUserStatus(id),
    onSuccess: () => { invalidateUsers(); toast.success(t('Status updated')) },
    onError,
  })

  const adjustMutation = useMutation({
    mutationFn: ({ userId, amount, reason }: { userId: number; amount: number; reason: string }) =>
      adjustUserPoints(userId, amount, reason),
    onSuccess: (res) => {
      if (!res.success) { toast.error(res.message); return }
      setAdjustOpen(false)
      invalidateUsers()
      invalidateLogs()
      toast.success(t('Points adjusted'))
    },
    onError,
  })

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-bold tracking-tight">
          {t('User Management')}
        </h1>
      </div>

      <UserTable
        users={users}
        search={search}
        onSearchChange={(v) => { setSearch(v); setPage(1) }}
        onView={(user) => { setDetailUserId(user.id); setDetailOpen(true) }}
        onToggle={(user) => toggleMutation.mutate(user.id)}
        onAdjust={(user) => { setAdjustUser(user); setAdjustOpen(true) }}
        total={total}
        page={page}
        pageSize={PAGE_SIZE}
        onPageChange={setPage}
      />

      <AdjustLogsTable
        logs={logs}
        total={logsTotal}
        page={logsPage}
        pageSize={PAGE_SIZE}
        onPageChange={setLogsPage}
      />

      <UserDetailDrawer
        open={detailOpen}
        onOpenChange={setDetailOpen}
        userId={detailUserId}
      />

      <AdjustDialog
        open={adjustOpen}
        onOpenChange={setAdjustOpen}
        user={adjustUser}
        onSubmit={(values) => adjustMutation.mutate(values)}
        submitting={adjustMutation.isPending}
      />
    </div>
  )
}
