/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'

import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'

import { fetchDashboardStats } from '../api/mock'

function StatCard(props: {
  title: string
  value: string | number
  loading?: boolean
}) {
  return (
    <Card>
      <CardHeader className="pb-2">
        <CardTitle className="text-sm font-medium text-muted-foreground">
          {props.title}
        </CardTitle>
      </CardHeader>
      <CardContent>
        {props.loading ? (
          <Skeleton className="h-8 w-24" />
        ) : (
          <div className="text-2xl font-bold">{props.value}</div>
        )}
      </CardContent>
    </Card>
  )
}

export function Dashboard() {
  const { t } = useTranslation()
  const { data, isLoading } = useQuery({
    queryKey: ['admin', 'dashboard'],
    queryFn: fetchDashboardStats,
  })

  const stats = data?.data

  return (
    <div className="space-y-6">
      <h1 className="text-2xl font-bold tracking-tight">
        {t('Dashboard')}
      </h1>

      <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-5">
        <StatCard
          title={t('Total Users')}
          value={stats?.total_users?.toLocaleString() ?? '-'}
          loading={isLoading}
        />
        <StatCard
          title={t('Today Recharge')}
          value={`¥${stats?.today_recharge_amount?.toLocaleString() ?? '-'}`}
          loading={isLoading}
        />
        <StatCard
          title={t('Today Recharge Orders')}
          value={stats?.today_recharge_count ?? '-'}
          loading={isLoading}
        />
        <StatCard
          title={t('Today Points Consumed')}
          value={stats?.today_points_consumed?.toLocaleString() ?? '-'}
          loading={isLoading}
        />
        <StatCard
          title={t('Pending Orders')}
          value={stats?.pending_orders ?? '-'}
          loading={isLoading}
        />
      </div>
    </div>
  )
}
