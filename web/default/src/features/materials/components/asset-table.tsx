import { useQuery, useQueryClient } from '@tanstack/react-query'
import type { OnChangeFn, PaginationState } from '@tanstack/react-table'
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'

import {
  DataTablePage,
  useDataTable,
} from '@/components/data-table'

import { getGroupAssets } from '../api'
import { useAssetColumns } from './columns'
import { DeleteAssetDialog } from './delete-asset-dialog'
import { RenameAssetDialog } from './rename-asset-dialog'
import type { MaterialAsset } from '../types'

interface MaterialAssetTableProps {
  groupId: string
}

/** 右侧素材表格：按选中的素材组 id 查上游素材，服务端分页，支持重命名/删除。 */
export function MaterialAssetTable({ groupId }: MaterialAssetTableProps) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [pagination, setPagination] = useState<PaginationState>({
    pageIndex: 0,
    pageSize: 10,
  })
  const [nameFilter, setNameFilter] = useState('')
  const [renameTarget, setRenameTarget] = useState<MaterialAsset | null>(null)
  const [deleteTarget, setDeleteTarget] = useState<MaterialAsset | null>(null)

  // 切换素材组时重置分页与搜索，避免停留在旧组的高页/残留搜索。
  useEffect(() => {
    setPagination({ pageIndex: 0, pageSize: 10 })
    setNameFilter('')
  }, [groupId])

  // 搜索变化时回到第一页
  const handleNameFilterChange: OnChangeFn<string> = (updater) => {
    const value = typeof updater === 'function' ? updater(nameFilter) : updater
    setNameFilter(value)
    setPagination((prev) => ({ ...prev, pageIndex: 0 }))
  }

  const { data, isLoading, isFetching } = useQuery({
    queryKey: [
      'material-assets',
      groupId,
      pagination.pageIndex + 1,
      pagination.pageSize,
      nameFilter,
    ],
    queryFn: () =>
      getGroupAssets(
        groupId,
        pagination.pageIndex + 1,
        pagination.pageSize,
        nameFilter
      ),
    enabled: Boolean(groupId),
    placeholderData: (previousData) => previousData,
  })

  const assets = data?.Items ?? []
  const columns = useAssetColumns({
    onRename: setRenameTarget,
    onDelete: setDeleteTarget,
  })

  const { table } = useDataTable({
    data: assets,
    columns,
    manualPagination: true,
    pagination,
    onPaginationChange: setPagination,
    totalCount: data?.TotalCount ?? 0,
    // 不启用列过滤，但需显式初始化 columnFilters 为空数组，
    // 否则工具栏访问 columnFilters.length 会因 undefined 崩溃。
    columnFilters: [],
    onColumnFiltersChange: () => {},
    // 名称搜索走后端透传上游，禁用表格本地过滤。
    globalFilter: nameFilter,
    onGlobalFilterChange: handleNameFilterChange,
    globalFilterFn: () => true,
  })

  const refresh = () =>
    queryClient.invalidateQueries({
      queryKey: ['material-assets', groupId],
    })

  if (!groupId) {
    return (
      <div className='text-muted-foreground flex h-40 items-center justify-center rounded-lg border text-sm'>
        {t('Select a material group to view assets')}
      </div>
    )
  }

  return (
    <>
      <DataTablePage
        table={table}
        columns={columns}
        isLoading={isLoading}
        isFetching={isFetching}
        emptyTitle={t('No assets')}
        toolbarProps={{
          searchPlaceholder: t('Filter by name...'),
        }}
      />
      <RenameAssetDialog
        groupId={groupId}
        asset={renameTarget}
        open={Boolean(renameTarget)}
        onOpenChange={(next) => !next && setRenameTarget(null)}
        onRenamed={refresh}
      />
      <DeleteAssetDialog
        groupId={groupId}
        asset={deleteTarget}
        open={Boolean(deleteTarget)}
        onOpenChange={(next) => !next && setDeleteTarget(null)}
        onDeleted={refresh}
      />
    </>
  )
}
