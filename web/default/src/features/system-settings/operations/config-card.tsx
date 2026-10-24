import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Button } from '@/components/ui/button'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Textarea } from '@/components/ui/textarea'

import { updateSystemOption } from '../api'
import type { SystemOption } from '../types'

interface ConfigCardProps {
  entry: SystemOption
  onSaved: () => void
}

/** 单个 JSON 配置的编辑卡片：格式化 + 校验 + 保存（PUT /api/option）。 */
export function ConfigCard({ entry, onSaved }: ConfigCardProps) {
  const { t } = useTranslation()
  const [value, setValue] = useState(entry.value)
  const [error, setError] = useState('')
  const [saving, setSaving] = useState(false)

  // 只接受「非 null 普通对象」：标量/数组/null 会静默破坏 .config 配置，
  // 后端 updateConfigFromMap 对反序列化失败是静默 continue，前端必须先拦住。
  const isValidJson = () => {
    try {
      const parsed = JSON.parse(value)
      return (
        parsed !== null && typeof parsed === 'object' && !Array.isArray(parsed)
      )
    } catch {
      return false
    }
  }

  const handleFormat = () => {
    if (!isValidJson()) {
      setError(t('Invalid JSON'))
      return
    }
    setError('')
    setValue(JSON.stringify(JSON.parse(value), null, 2))
  }

  const handleSave = async () => {
    if (!isValidJson()) {
      setError(t('Invalid JSON'))
      return
    }
    setError('')
    setSaving(true)
    try {
      const result = await updateSystemOption({ key: entry.key, value })
      if (result.success) {
        toast.success(t('Save'))
        onSaved()
      } else {
        setError(result.message || t('Save failed'))
      }
    } catch {
      setError(t('Save failed'))
    } finally {
      setSaving(false)
    }
  }

  return (
    <div className='space-y-3 rounded-lg border p-4'>
      <div className='flex items-center justify-between gap-2'>
        <div className='font-mono text-sm font-medium'>{entry.key}</div>
        <Button variant='ghost' size='sm' onClick={handleFormat}>
          {t('Format')}
        </Button>
      </div>
      {entry.key === 'material_library.config' && (
        <>
          <Alert>
            <AlertDescription>
              {t(
                'When video generation goes through a TokenHub channel, keep this provider set to tokenhub so asset:// references resolve in the same upstream account.'
              )}
            </AlertDescription>
          </Alert>
          <p className='text-muted-foreground text-xs'>
            {t(
              'An asset group and the video API key must belong to the same upstream account and project.'
            )}
          </p>
        </>
      )}
      <Textarea
        value={value}
        onChange={(e) => {
          setValue(e.target.value)
          if (error) setError('')
        }}
        rows={8}
        className='font-mono text-xs'
        spellCheck={false}
      />
      {error && <p className='text-destructive text-xs'>{error}</p>}
      <div className='flex justify-end'>
        <Button onClick={handleSave} disabled={saving}>
          {saving ? t('Saving') : t('Save')}
        </Button>
      </div>
    </div>
  )
}
