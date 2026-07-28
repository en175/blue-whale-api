import { useTranslation } from 'react-i18next'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'

import type { AdjustLog } from '../lib/types'

function formatDate(iso: string, locale: string) {
  return new Date(iso).toLocaleString(locale)
}

interface AdjustLogsTableProps {
  logs: AdjustLog[]
  total: number
  page: number
  pageSize: number
  onPageChange: (p: number) => void
}

export function AdjustLogsTable(props: AdjustLogsTableProps) {
  const { t, i18n } = useTranslation()
  const totalPages = Math.max(1, Math.ceil(props.total / props.pageSize))

  const columns = [
    { key: 'id', label: 'ID', format: (l: AdjustLog) => `#${l.id}` },
    { key: 'username', label: 'Username', format: (l: AdjustLog) => l.username },
    { key: 'amount', label: 'Amount', format: (l: AdjustLog) =>
      l.amount > 0
        ? <Badge variant="default" className="bg-green-600 hover:bg-green-600">+{l.amount.toLocaleString()}</Badge>
        : <Badge variant="secondary" className="bg-red-100 text-red-700 dark:bg-red-900 dark:text-red-200">{l.amount.toLocaleString()}</Badge>
    },
    { key: 'reason', label: 'Reason', format: (l: AdjustLog) => l.reason },
    { key: 'operator', label: 'Operator', format: (l: AdjustLog) => l.operator },
    { key: 'created_at', label: 'Time', format: (l: AdjustLog) => formatDate(l.created_at, i18n.language) },
  ]

  return (
    <div className="space-y-4">
      <h2 className="text-lg font-semibold">{t('Adjust Logs')}</h2>
      <Table>
        <TableHeader>
          <TableRow>
            {columns.map((col) => (
              <TableHead key={col.key}>{t(col.label)}</TableHead>
            ))}
          </TableRow>
        </TableHeader>
        <TableBody>
          {props.logs.map((log) => (
            <TableRow key={log.id}>
              {columns.map((col) => (
                <TableCell key={col.key}>{col.format(log)}</TableCell>
              ))}
            </TableRow>
          ))}
          {props.logs.length === 0 && (
            <TableRow>
              <TableCell colSpan={columns.length} className="text-muted-foreground py-8 text-center">
                {t('No adjust logs')}
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
