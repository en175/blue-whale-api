import { useState } from 'react'
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
import {
  NativeSelect,
  NativeSelectOption,
} from '@/components/ui/native-select'

import { createMaterialAsset } from '../api'

interface UploadAssetDialogProps {
  groupId: string
  open: boolean
  onOpenChange: (open: boolean) => void
  onUploaded: () => void
}

/** 上传素材：上游 CreateAsset，URL 直传。 */
export function UploadAssetDialog({
  groupId,
  open,
  onOpenChange,
  onUploaded,
}: UploadAssetDialogProps) {
  const { t } = useTranslation()
  const [url, setUrl] = useState('')
  const [name, setName] = useState('')
  const [assetType, setAssetType] = useState('Image')
  const [isSubmitting, setIsSubmitting] = useState(false)

  const reset = () => {
    setUrl('')
    setName('')
    setAssetType('Image')
  }

  const handleSubmit = async () => {
    if (!url.trim() || !name.trim()) {
      toast.error(t('Asset URL') + ' / ' + t('Asset Name') + ' ' + t('is required'))
      return
    }
    setIsSubmitting(true)
    try {
      await createMaterialAsset(groupId, {
        url: url.trim(),
        name: name.trim(),
        asset_type: assetType,
      })
      toast.success(t('Upload Asset'))
      reset()
      onOpenChange(false)
      onUploaded()
    } catch {
      toast.error(t('Upload Asset') + ' ' + t('failed'))
    } finally {
      setIsSubmitting(false)
    }
  }

  return (
    <Dialog
      open={open}
      onOpenChange={(next) => {
        if (!next) reset()
        onOpenChange(next)
      }}
    >
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t('Upload Asset')}</DialogTitle>
          <DialogDescription>{t('Asset URL')}</DialogDescription>
        </DialogHeader>
        <div className='space-y-4'>
          <div className='space-y-1.5'>
            <Label htmlFor='asset-url'>{t('Asset URL')}</Label>
            <Input
              id='asset-url'
              value={url}
              onChange={(e) => setUrl(e.target.value)}
              placeholder='https://'
            />
          </div>
          <div className='space-y-1.5'>
            <Label htmlFor='asset-name'>{t('Asset Name')}</Label>
            <Input
              id='asset-name'
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder={t('Asset Name')}
            />
          </div>
          <div className='space-y-1.5'>
            <Label htmlFor='asset-type'>{t('Asset Type')}</Label>
            <NativeSelect
              id='asset-type'
              value={assetType}
              onChange={(e) => setAssetType(e.target.value)}
            >
              <NativeSelectOption value='Image'>Image</NativeSelectOption>
              <NativeSelectOption value='Video'>Video</NativeSelectOption>
              <NativeSelectOption value='Audio'>Audio</NativeSelectOption>
            </NativeSelect>
          </div>
        </div>
        <DialogFooter>
          <Button variant='ghost' onClick={() => onOpenChange(false)}>
            {t('Cancel')}
          </Button>
          <Button onClick={handleSubmit} disabled={isSubmitting}>
            {isSubmitting ? t('Uploading') : t('Upload')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
