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
}

/** 左侧素材组列表：数据来自父级（本地归属表查询），支持删除。 */
export function MaterialGroupList({
  groups,
  isLoading,
  groupType,
  selectedGroupId,
  onSelect,
}: MaterialGroupListProps) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [search, setSearch] = useState('')
  const [deleteTarget, setDeleteTarget] = useState<MaterialGroup | null>(null)
  const [isDeleting, setIsDeleting] = useState(false)

  const keyword = search.trim().toLowerCase()
  const filteredGroups = keyword
    ? groups.filter(
        (group) =>
          (group.name || '').toLowerCase().includes(keyword) ||
          group.seedance_asset_group_id.toLowerCase().includes(keyword)
      )
    : groups

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
        {t('No material groups')}
      </div>
    )
  }

  return (
    <>
      <div className='relative mb-2'>
        <Search className='text-muted-foreground pointer-events-none absolute top-1/2 left-2.5 size-4 -translate-y-1/2' />
        <Input
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          placeholder={t('Filter by name...')}
          className='h-8 pl-8'
          aria-label={t('Filter by name...')}
        />
      </div>
      {!filteredGroups.length ? (
        <div className='text-muted-foreground flex h-32 items-center justify-center rounded-lg border text-sm'>
          {t('No material groups')}
        </div>
      ) : (
        <div className='space-y-1.5'>
          {filteredGroups.map((group) => {
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
