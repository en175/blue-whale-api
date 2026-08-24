import { useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  Pagination,
  PaginationContent,
  PaginationItem,
  PaginationLink,
  PaginationNext,
  PaginationPrevious,
} from '@/components/ui/pagination'
import { Skeleton } from '@/components/ui/skeleton'
import { Search, Trash2 } from 'lucide-react'
import { cn } from '@/lib/utils'

import { deleteMaterialGroup } from '../api'
import type { MaterialGroup, MaterialGroupType } from '../types'

interface MaterialGroupListProps {
  groups: MaterialGroup[]
  isLoading: boolean
  groupType: MaterialGroupType
  selectedGroupId: string
  onSelect: (groupId: string) => void
  totalCount: number
  page: number
  pageSize: number
  onPageChange: (page: number) => void
  search: string
  onSearchChange: (search: string) => void
}

/** 左侧素材组列表：数据来自父级（本地归属表分页查询，name 过滤已在服务端生效），支持删除与分页翻页。 */
export function MaterialGroupList({
  groups,
  isLoading,
  groupType,
  selectedGroupId,
  onSelect,
  totalCount,
  page,
  pageSize,
  onPageChange,
  search,
  onSearchChange,
}: MaterialGroupListProps) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [deleteTarget, setDeleteTarget] = useState<MaterialGroup | null>(null)
  const [isDeleting, setIsDeleting] = useState(false)

  const handleDelete = async () => {
    if (!deleteTarget) return
    setIsDeleting(true)
    try {
      await deleteMaterialGroup(deleteTarget.seedance_asset_group_id)
      toast.success(t('Delete Material Group'))
      if (selectedGroupId === deleteTarget.seedance_asset_group_id) {
        onSelect('')
      }
      setDeleteTarget(null)
      // 前缀失效：删除影响所有搜索/分页视图，不仅当前页。
      await queryClient.invalidateQueries({
        queryKey: ['material-groups', groupType],
      })
    } catch {
      toast.error(t('Delete Material Group') + ' ' + t('failed'))
    } finally {
      setIsDeleting(false)
    }
  }

  if (isLoading) {
    return <Skeleton className='h-64 w-full' />
  }

  if (!groups.length) {
    return (
      <div className='text-muted-foreground flex h-32 items-center justify-center rounded-lg border text-sm'>
        {search ? t('No matching groups') : t('No material groups')}
      </div>
    )
  }

  return (
    <>
      <div className='relative mb-2'>
        <Search className='text-muted-foreground pointer-events-none absolute top-1/2 left-2.5 size-4 -translate-y-1/2' />
        <Input
          value={search}
          onChange={(e) => onSearchChange(e.target.value)}
          placeholder={t('Filter by name...')}
          className='h-8 pl-8'
          aria-label={t('Filter by name...')}
        />
      </div>
      <div className='space-y-1.5'>
        {groups.map((group) => {
          const groupId = group.seedance_asset_group_id
          const isSelected = selectedGroupId === groupId
          return (
            <div
              key={groupId}
              className={cn(
                'group flex w-full items-center gap-1 rounded-lg border px-3 py-2 transition-colors',
                isSelected ? 'bg-accent text-accent-foreground' : 'hover:bg-muted'
              )}
            >
              <button
                type='button'
                onClick={() => onSelect(groupId)}
                className='min-w-0 flex-1 text-left'
              >
                <div className='truncate text-sm font-medium'>
                  {group.name || groupId}
                </div>
                <div className='text-muted-foreground mt-0.5 truncate text-xs'>
                  {groupId}
                </div>
              </button>
              <Button
                type='button'
                variant='ghost'
                size='icon-sm'
                onClick={() => setDeleteTarget(group)}
                aria-label={t('Delete Material Group')}
                className='text-muted-foreground shrink-0 opacity-0 group-hover:opacity-100'
              >
                <Trash2 className='size-4' />
              </Button>
            </div>
          )
        })}
      </div>

      {Math.ceil(totalCount / pageSize) > 1 && (
        <Pagination className='mt-3'>
          <PaginationContent>
            <PaginationItem>
              <PaginationPrevious
                text={t('Previous')}
                onClick={() => onPageChange(page - 1)}
                className={page <= 1 ? 'pointer-events-none opacity-50' : ''}
              />
            </PaginationItem>
            <PaginationItem>
              <PaginationLink isActive>{page}</PaginationLink>
            </PaginationItem>
            {page < Math.ceil(totalCount / pageSize) && (
              <PaginationItem>
                <PaginationLink onClick={() => onPageChange(page + 1)}>
                  {page + 1}
                </PaginationLink>
              </PaginationItem>
            )}
            <PaginationItem>
              <PaginationNext
                text={t('Next')}
                onClick={() => onPageChange(page + 1)}
                className={
                  page >= Math.ceil(totalCount / pageSize)
                    ? 'pointer-events-none opacity-50'
                    : ''
                }
              />
            </PaginationItem>
          </PaginationContent>
        </Pagination>
      )}

      <AlertDialog
        open={Boolean(deleteTarget)}
        onOpenChange={(next) => !next && setDeleteTarget(null)}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{t('Are you sure?')}</AlertDialogTitle>
            <AlertDialogDescription>
              {t('Delete Material Group')}{' '}
              <span className='font-semibold'>
                {deleteTarget?.name || deleteTarget?.seedance_asset_group_id}
              </span>
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={isDeleting}>
              {t('Cancel')}
            </AlertDialogCancel>
            <AlertDialogAction
              onClick={handleDelete}
              disabled={isDeleting}
              className='bg-destructive text-destructive-foreground'
            >
              {isDeleting ? t('Deleting') : t('Delete')}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </>
  )
}
