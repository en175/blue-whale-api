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

import type { PricingRule } from '../lib/types'

interface PricingTableProps {
  rules: PricingRule[]
  onEdit: (rule: PricingRule) => void
  onDelete: (rule: PricingRule) => void
  onToggle: (rule: PricingRule) => void
}

export function PricingTable(props: PricingTableProps) {
  const { t } = useTranslation()

  const columns = [
    { key: 'model', label: 'Model', format: (r: PricingRule) => r.model },
    { key: 'input_price', label: 'Input Price', format: (r: PricingRule) => `$${r.input_price}` },
    { key: 'output_price', label: 'Output Price', format: (r: PricingRule) => `$${r.output_price}` },
    { key: 'ratio', label: 'Ratio', format: (r: PricingRule) => `${r.ratio}x` },
    { key: 'unit', label: 'Unit', format: (r: PricingRule) => r.unit },
    { key: 'status', label: 'Status', format: (r: PricingRule) =>
      r.is_active
        ? <Badge variant="default">{t('Active')}</Badge>
        : <Badge variant="secondary">{t('Inactive')}</Badge>
    },
  ]

  return (
    <Table>
      <TableHeader>
        <TableRow>
          {columns.map((col) => (
            <TableHead key={col.key}>{t(col.label)}</TableHead>
          ))}
          <TableHead className="w-40 text-right">{t('Actions')}</TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {props.rules.map((rule) => (
          <TableRow key={rule.id}>
            {columns.map((col) => (
              <TableCell key={col.key}>{col.format(rule)}</TableCell>
            ))}
            <TableCell className="text-right">
              <div className="flex items-center justify-end gap-1">
                <Button
                  variant="outline"
                  size="sm"
                  onClick={() => props.onToggle(rule)}
                >
                  {rule.is_active ? t('Deactivate') : t('Activate')}
                </Button>
                <Button
                  variant="outline"
                  size="sm"
                  onClick={() => props.onEdit(rule)}
                >
                  {t('Edit')}
                </Button>
                <Button
                  variant="outline"
                  size="sm"
                  onClick={() => props.onDelete(rule)}
                >
                  {t('Delete')}
                </Button>
              </div>
            </TableCell>
          </TableRow>
        ))}
        {props.rules.length === 0 && (
          <TableRow>
            <TableCell colSpan={columns.length + 1} className="text-muted-foreground py-8 text-center">
              {t('No pricing rules')}
            </TableCell>
          </TableRow>
        )}
      </TableBody>
    </Table>
  )
}
