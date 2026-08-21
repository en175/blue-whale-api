import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { QRCodeSVG } from 'qrcode.react'
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

import { createMaterialGroup, createVisualValidateSession } from '../api'
import type { MaterialGroupType } from '../types'

interface CreateGroupDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  defaultGroupType: MaterialGroupType
  onCreated: () => void
}

/**
 * 新建素材组：表单完全统一（组名/类型/描述），仅提交后的接口与交互不同——
 * - AIGC：上游 CreateAssetGroup，直接创建
 * - 真人（LivenessFace）：发起 CreateVisualValidateSession（回调指向网关），
 *   弹 H5 认证二维码，认证完成后网关后台轮询拿 GroupId 并记录归属，出组
 */
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
  const [h5Link, setH5Link] = useState('')

  const isLiveness = groupType === 'LivenessFace'

  const reset = () => {
    setName('')
    setDescription('')
    setGroupType(defaultGroupType)
    setH5Link('')
  }

  const handleSubmit = async () => {
    if (!name.trim()) {
      toast.error(t('Group Name') + ' ' + t('is required'))
      return
    }
    setIsSubmitting(true)
    try {
      if (isLiveness) {
        // 真人：发起认证会话，进入二维码阶段（认证完成前不创建组）。
        const res = await createVisualValidateSession(name.trim())
        if (!res.h5_link) throw new Error('empty h5 link')
        setH5Link(res.h5_link)
      } else {
        await createMaterialGroup({
          name: name.trim(),
          description: description.trim(),
          group_type: groupType,
        })
        toast.success(t('Create') + ' ' + t('Group Name'))
        reset()
        onOpenChange(false)
        onCreated()
      }
    } catch {
      toast.error(t('Create Material Group') + ' ' + t('failed'))
    } finally {
      setIsSubmitting(false)
    }
  }

  const handleDone = () => {
    reset()
    onOpenChange(false)
    onCreated()
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
          <DialogDescription>
            {isLiveness ? t('Real Person Certification') : t('Group Type')}
          </DialogDescription>
        </DialogHeader>

        {h5Link ? (
          <div className='flex flex-col items-center gap-3 py-2'>
            <div className='rounded-lg border bg-white p-3'>
              <QRCodeSVG value={h5Link} size={192} />
            </div>
            <p className='text-muted-foreground text-center text-xs'>
              {t('Certification H5 link expires soon, please scan promptly')}
            </p>
          </div>
        ) : (
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
                onChange={(e) =>
                  setGroupType(e.target.value as MaterialGroupType)
                }
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
        )}

        <DialogFooter>
          {h5Link ? (
            <Button onClick={handleDone}>{t('Done')}</Button>
          ) : (
            <>
              <Button variant='ghost' onClick={() => onOpenChange(false)}>
                {t('Cancel')}
              </Button>
              <Button onClick={handleSubmit} disabled={isSubmitting}>
                {isSubmitting
                  ? t('Sending')
                  : isLiveness
                    ? t('Start Certification')
                    : t('Create')}
              </Button>
            </>
          )}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
