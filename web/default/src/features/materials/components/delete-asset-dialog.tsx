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

import { deleteMaterialAsset } from '../api'
import type { MaterialAsset } from '../types'

interface DeleteAssetDialogProps {
  groupId: string
  asset: MaterialAsset | null
  open: boolean
  onOpenChange: (open: boolean) => void
  onDeleted: () => void
}

/** 删除素材：上游 DeleteAsset。 */
export function DeleteAssetDialog({
  groupId,
  asset,
  open,
  onOpenChange,
  onDeleted,
}: DeleteAssetDialogProps) {
  const { t } = useTranslation()
  const [isDeleting, setIsDeleting] = useState(false)

  const handleDelete = async () => {
    if (!asset) return
    setIsDeleting(true)
    try {
      await deleteMaterialAsset(groupId, asset.Id)
      toast.success(t('Delete Asset'))
      onOpenChange(false)
      onDeleted()
    } catch {
      toast.error(t('Delete Asset') + ' ' + t('failed'))
    } finally {
      setIsDeleting(false)
    }
  }

  return (
    <AlertDialog open={open} onOpenChange={onOpenChange}>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>{t('Are you sure?')}</AlertDialogTitle>
          <AlertDialogDescription>
            {t('Delete Asset')} <span className='font-semibold'>{asset?.Name}</span>
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
  )
}
