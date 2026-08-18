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
import { Textarea } from '@/components/ui/textarea'

import { createMaterialGroup } from '../api'
import type { MaterialGroupType } from '../types'

interface CreateGroupDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  defaultGroupType: MaterialGroupType
  onCreated: () => void
}

/** 新建素材组：上游 CreateAssetGroup，成功后归属记录到当前用户。 */
export function CreateGroupDialog({
  open,
  onOpenChange,
  defaultGroupType,
  onCreated,
}: CreateGroupDialogProps) {
  const { t } = useTranslation()
  const [name, setName] = useState('')
  const [description, setDescription] = useState('')
  const [groupType, setGroupType] = useState<MaterialGroupType>(defaultGroupType)
  const [isSubmitting, setIsSubmitting] = useState(false)

  const reset = () => {
    setName('')
    setDescription('')
    setGroupType(defaultGroupType)
  }

  const handleSubmit = async () => {
    if (!name.trim()) {
      toast.error(t('Group Name') + ' ' + t('is required'))
      return
    }
    setIsSubmitting(true)
    try {
      await createMaterialGroup({
        name: name.trim(),
        description: description.trim(),
        group_type: groupType,
      })
      toast.success(t('Create') + ' ' + t('Group Name'))
      reset()
      onOpenChange(false)
      onCreated()
    } catch {
      toast.error(t('Create Material Group') + ' ' + t('failed'))
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
          <DialogTitle>{t('New Material Group')}</DialogTitle>
          <DialogDescription>{t('Group Type')}</DialogDescription>
        </DialogHeader>
        <div className='space-y-4'>
          <div className='space-y-1.5'>
            <Label htmlFor='group-name'>{t('Group Name')}</Label>
            <Input
              id='group-name'
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder={t('Group Name')}
            />
          </div>
          <div className='space-y-1.5'>
            <Label htmlFor='group-type'>{t('Group Type')}</Label>
            <NativeSelect
              id='group-type'
              value={groupType}
              onChange={(e) => setGroupType(e.target.value as MaterialGroupType)}
            >
              <NativeSelectOption value='AIGC'>
                {t('AIGC Materials')}
              </NativeSelectOption>
              <NativeSelectOption value='LivenessFace'>
                {t('Real Person Materials')}
              </NativeSelectOption>
            </NativeSelect>
          </div>
          <div className='space-y-1.5'>
            <Label htmlFor='group-desc'>{t('Description')}</Label>
            <Textarea
              id='group-desc'
              value={description}
              onChange={(e) => setDescription(e.target.value)}
              placeholder={t('Description')}
              rows={3}
            />
          </div>
        </div>
        <DialogFooter>
          <Button variant='ghost' onClick={() => onOpenChange(false)}>
            {t('Cancel')}
          </Button>
          <Button onClick={handleSubmit} disabled={isSubmitting}>
            {isSubmitting ? t('Creating') : t('Create')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
