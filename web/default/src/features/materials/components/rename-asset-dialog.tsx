import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'

import { renameMaterialAsset } from '../api'
import type { MaterialAsset } from '../types'

interface RenameAssetDialogProps {
  groupId: string
  asset: MaterialAsset | null
  open: boolean
  onOpenChange: (open: boolean) => void
  onRenamed: () => void
}

/** 重命名素材：上游 UpdateAsset。 */
export function RenameAssetDialog({
  groupId,
  asset,
  open,
  onOpenChange,
  onRenamed,
}: RenameAssetDialogProps) {
  const { t } = useTranslation()
  const [name, setName] = useState('')
  const [isSubmitting, setIsSubmitting] = useState(false)

  useEffect(() => {
    if (open) setName(asset?.Name ?? '')
  }, [open, asset])

  const handleSubmit = async () => {
    if (!name.trim() || !asset) return
    setIsSubmitting(true)
    try {
      await renameMaterialAsset(groupId, asset.Id, name.trim())
      toast.success(t('Rename Asset'))
      onOpenChange(false)
      onRenamed()
    } catch {
      toast.error(t('Rename Asset') + ' ' + t('failed'))
    } finally {
      setIsSubmitting(false)
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t('Rename Asset')}</DialogTitle>
          <DialogDescription>{asset?.Name}</DialogDescription>
        </DialogHeader>
        <div className='space-y-1.5'>
          <Label htmlFor='rename-asset-name'>{t('Asset Name')}</Label>
          <Input
            id='rename-asset-name'
            value={name}
            onChange={(e) => setName(e.target.value)}
            placeholder={t('Asset Name')}
          />
        </div>
        <DialogFooter>
          <Button variant='ghost' onClick={() => onOpenChange(false)}>
            {t('Cancel')}
          </Button>
          <Button onClick={handleSubmit} disabled={isSubmitting}>
            {isSubmitting ? t('Saving') : t('Confirm')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
