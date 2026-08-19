import type { ColumnDef } from '@tanstack/react-table'
import { useTranslation } from 'react-i18next'

import {
  BadgeCell,
  DataTableColumnHeader,
  StaticRowActions,
} from '@/components/data-table'
import { Badge } from '@/components/ui/badge'

import type { MaterialAsset } from '../types'
import { UrlCell } from './url-cell'

const ASSET_STATUS_VARIANT: Record<string, 'default' | 'secondary' | 'destructive'> = {
  Active: 'default',
  Processing: 'secondary',
  Failed: 'destructive',
}

export interface AssetColumnOptions {
  onRename: (asset: MaterialAsset) => void
  onDelete: (asset: MaterialAsset) => void
}

/** 素材表格列定义：id/名称/类型/状态/地址/创建时间/操作。 */
export function useAssetColumns({
  onRename,
  onDelete,
}: AssetColumnOptions): ColumnDef<MaterialAsset>[] {
  const { t } = useTranslation()

  return [
    {
      accessorKey: 'Id',
      header: ({ column }) => (
        <DataTableColumnHeader column={column} title='ID' />
      ),
      cell: ({ row }) => (
        <span className='font-mono text-xs'>{row.original.Id}</span>
      ),
    },
    {
      accessorKey: 'Name',
      header: ({ column }) => (
        <DataTableColumnHeader column={column} title={t('Name')} />
      ),
      cell: ({ row }) => (
        <span className='font-medium'>{row.original.Name}</span>
      ),
    },
    {
      accessorKey: 'AssetType',
      header: ({ column }) => (
        <DataTableColumnHeader column={column} title={t('Type')} />
      ),
      cell: ({ row }) => row.original.AssetType,
    },
    {
      accessorKey: 'Status',
      header: ({ column }) => (
        <DataTableColumnHeader column={column} title={t('Status')} />
      ),
      cell: ({ row }) => (
        <BadgeCell>
          <Badge
            variant={ASSET_STATUS_VARIANT[row.original.Status] ?? 'secondary'}
          >
            {row.original.Status}
          </Badge>
        </BadgeCell>
      ),
    },
    {
      accessorKey: 'URL',
      header: ({ column }) => (
        <DataTableColumnHeader column={column} title={t('URL')} />
      ),
      // 仅 URL 列限宽：单元格内容硬 max-w-44 截断，其余列按内容自适应。
      cell: ({ row }) => <UrlCell url={row.original.URL} />,
    },
    {
      accessorKey: 'CreateTime',
      header: ({ column }) => (
        <DataTableColumnHeader column={column} title={t('Created Time')} />
      ),
      cell: ({ row }) => (
        <span className='whitespace-nowrap'>{row.original.CreateTime}</span>
      ),
    },
    {
      id: 'actions',
      header: () => <div className='text-right'>{t('Actions')}</div>,
      cell: ({ row }) => (
        <StaticRowActions
          editLabel={t('Rename')}
          deleteLabel={t('Delete')}
          menuLabel={t('Actions')}
          onEdit={() => onRename(row.original)}
          onDelete={() => onDelete(row.original)}
        />
      ),
    },
  ]
}
