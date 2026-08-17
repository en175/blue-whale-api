/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { zodResolver } from '@hookform/resolvers/zod'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { Loader2, Plus } from 'lucide-react'
import { useEffect, useState } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import * as z from 'zod'

import { ConfirmDialog } from '@/components/confirm-dialog'
import {
  StaticDataTable,
  StaticRowActions,
  type StaticDataTableColumn,
} from '@/components/data-table'
import {
  sideDrawerContentClassName,
  sideDrawerFooterClassName,
  sideDrawerFormClassName,
  sideDrawerHeaderClassName,
  sideDrawerSwitchItemClassName,
} from '@/components/drawer-layout'
import { SectionPageLayout } from '@/components/layout'
import { StatusBadge } from '@/components/status-badge'
import { Button } from '@/components/ui/button'
import {
  Form,
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Input } from '@/components/ui/input'
import {
  Sheet,
  SheetClose,
  SheetContent,
  SheetDescription,
  SheetFooter,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet'
import { Switch } from '@/components/ui/switch'
import { Textarea } from '@/components/ui/textarea'

import {
  createApiDocModel,
  deleteApiDocModel,
  getApiDocModels,
  getDocsDomain,
  setDocsDomain,
  updateApiDocModel,
} from './api'
import type { ApiDocModel } from './types'

const apiDocModelsQueryKey = ['api-doc-models']

const apiDocModelFormSchema = z.object({
  model_name: z.string().min(1, 'Model name is required'),
  display_name: z.string(),
  description: z.string(),
  param_schema: z.string().refine((value) => {
    const trimmed = value.trim()
    if (!trimmed) return true
    try {
      JSON.parse(trimmed)
      return true
    } catch {
      return false
    }
  }, 'Invalid JSON format'),
  sort_order: z.number().int().min(0),
  enabled: z.boolean(),
})

type ApiDocModelFormValues = z.infer<typeof apiDocModelFormSchema>

export function DocsPage() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()

  const { data: models = [], isLoading } = useQuery({
    queryKey: apiDocModelsQueryKey,
    queryFn: getApiDocModels,
  })

  const [drawerOpen, setDrawerOpen] = useState(false)
  const [currentRow, setCurrentRow] = useState<ApiDocModel | null>(null)
  const [deleteTarget, setDeleteTarget] = useState<ApiDocModel | null>(null)
  const [deleting, setDeleting] = useState(false)
  const [docsDomain, setDocsDomainValue] = useState('')
  const [savingDomain, setSavingDomain] = useState(false)

  useEffect(() => {
    let active = true
    getDocsDomain()
      .then((value) => {
        if (active) setDocsDomainValue(value)
      })
      .catch(() => {
        // Business/HTTP errors are surfaced by the global api interceptor.
      })
    return () => {
      active = false
    }
  }, [])

  const handleSaveDomain = async () => {
    setSavingDomain(true)
    try {
      await setDocsDomain(docsDomain)
      toast.success(t('Saved successfully'))
    } catch {
      // Business/HTTP errors are surfaced by the global api interceptor.
    } finally {
      setSavingDomain(false)
    }
  }

  const openCreate = () => {
    setCurrentRow(null)
    setDrawerOpen(true)
  }

  const openEdit = (row: ApiDocModel) => {
    setCurrentRow(row)
    setDrawerOpen(true)
  }

  const handleDelete = async () => {
    if (!deleteTarget) return
    setDeleting(true)
    try {
      const res = await deleteApiDocModel(deleteTarget.id)
      if (res.success) {
        toast.success(t('Deleted successfully'))
        queryClient.invalidateQueries({ queryKey: apiDocModelsQueryKey })
      }
    } catch {
      // Business/HTTP errors are surfaced by the global api interceptor.
    } finally {
      setDeleting(false)
      setDeleteTarget(null)
    }
  }

  const columns: StaticDataTableColumn<ApiDocModel>[] = [
    {
      id: 'model_name',
      header: t('Model Name'),
      cell: (row) => (
        <StatusBadge
          label={row.model_name}
          variant='neutral'
          copyText={row.model_name}
          size='sm'
          className='font-mono'
        />
      ),
    },
    {
      id: 'display_name',
      header: t('Display Name'),
      cell: (row) => row.display_name || '-',
    },
    {
      id: 'description',
      header: t('Description'),
      cell: (row) => row.description || '-',
    },
    {
      id: 'sort_order',
      header: t('Sort Order'),
      cell: (row) => row.sort_order,
    },
    {
      id: 'enabled',
      header: t('Enabled'),
      cell: (row) => (
        <StatusBadge
          variant={row.enabled ? 'success' : 'neutral'}
          copyable={false}
          size='sm'
        >
          {row.enabled ? t('Enabled') : t('Disabled')}
        </StatusBadge>
      ),
    },
    {
      id: 'actions',
      header: t('Actions'),
      cell: (row) => (
        <StaticRowActions
          editLabel={t('Edit')}
          deleteLabel={t('Delete')}
          menuLabel={t('Open menu')}
          onEdit={() => openEdit(row)}
          onDelete={() => setDeleteTarget(row)}
        />
      ),
    },
  ]

  return (
    <>
      <SectionPageLayout>
        <SectionPageLayout.Title>{t('External Docs')}</SectionPageLayout.Title>
        <SectionPageLayout.Actions>
          <Button onClick={openCreate} size='sm'>
            <Plus className='h-4 w-4' />
            {t('Create Model')}
          </Button>
        </SectionPageLayout.Actions>
        <SectionPageLayout.Content>
          <div className='mb-4 flex flex-col gap-2 sm:max-w-md'>
            <label className='text-sm font-medium'>{t('Docs Domain')}</label>
            <div className='flex items-center gap-2'>
              <Input
                value={docsDomain}
                onChange={(e) => setDocsDomainValue(e.target.value)}
                placeholder='https://docs.example.com'
              />
              <Button
                onClick={handleSaveDomain}
                disabled={savingDomain}
                size='sm'
              >
                {savingDomain && (
                  <Loader2 className='mr-2 h-4 w-4 animate-spin' />
                )}
                {t('Save')}
              </Button>
            </div>
          </div>
          {isLoading ? (
            <div className='flex items-center justify-center py-16'>
              <Loader2 className='text-muted-foreground h-6 w-6 animate-spin' />
            </div>
          ) : (
            <StaticDataTable
              columns={columns}
              data={models}
              getRowKey={(row) => row.id}
              emptyContent={t('No Data')}
            />
          )}
        </SectionPageLayout.Content>
      </SectionPageLayout>

      <ApiDocModelMutateDrawer
        open={drawerOpen}
        onOpenChange={setDrawerOpen}
        currentRow={currentRow}
      />

      <ConfirmDialog
        open={deleteTarget !== null}
        onOpenChange={(open) => !open && setDeleteTarget(null)}
        title={t('Delete Model')}
        desc={t(
          'Are you sure you want to delete model "{{name}}"? This action cannot be undone.',
          { name: deleteTarget?.model_name ?? '' }
        )}
        confirmText={t('Delete')}
        destructive
        isLoading={deleting}
        handleConfirm={handleDelete}
      />
    </>
  )
}

type ApiDocModelMutateDrawerProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  currentRow: ApiDocModel | null
}

function ApiDocModelMutateDrawer({
  open,
  onOpenChange,
  currentRow,
}: ApiDocModelMutateDrawerProps) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const isEditing = Boolean(currentRow)
  const [isSubmitting, setIsSubmitting] = useState(false)

  const form = useForm<ApiDocModelFormValues>({
    resolver: zodResolver(apiDocModelFormSchema),
    defaultValues: {
      model_name: '',
      display_name: '',
      description: '',
      param_schema: '',
      sort_order: 0,
      enabled: true,
    },
  })

  useEffect(() => {
    if (!open) return
    form.reset(
      currentRow
        ? {
            model_name: currentRow.model_name,
            display_name: currentRow.display_name || '',
            description: currentRow.description || '',
            param_schema: currentRow.param_schema || '',
            sort_order: currentRow.sort_order ?? 0,
            enabled: currentRow.enabled,
          }
        : {
            model_name: '',
            display_name: '',
            description: '',
            param_schema: '',
            sort_order: 0,
            enabled: true,
          }
    )
  }, [open, currentRow, form])

  const onSubmit = async (values: ApiDocModelFormValues) => {
    setIsSubmitting(true)
    try {
      const res = currentRow
        ? await updateApiDocModel({ ...currentRow, ...values })
        : await createApiDocModel(values)
      if (res.success) {
        toast.success(t('Saved successfully'))
        queryClient.invalidateQueries({ queryKey: apiDocModelsQueryKey })
        onOpenChange(false)
      }
    } catch {
      // Business/HTTP errors are surfaced by the global api interceptor.
    } finally {
      setIsSubmitting(false)
    }
  }

  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent className={sideDrawerContentClassName('sm:max-w-xl')}>
        <SheetHeader className={sideDrawerHeaderClassName()}>
          <SheetTitle>
            {isEditing ? t('Edit Model') : t('Create Model')}
          </SheetTitle>
          <SheetDescription>
            {isEditing
              ? t("Update model configuration and click save when you're done.")
              : t(
                  'Add a new model to the system by providing the necessary information.'
                )}
          </SheetDescription>
        </SheetHeader>

        <Form {...form}>
          <form
            id='api-doc-model-form'
            onSubmit={form.handleSubmit(onSubmit)}
            className={sideDrawerFormClassName()}
          >
            <FormField
              control={form.control}
              name='model_name'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Model Name')}</FormLabel>
                  <FormControl>
                    <Input placeholder='gpt-4, claude-3-opus, etc.' {...field} />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />

            <FormField
              control={form.control}
              name='display_name'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Display Name')}</FormLabel>
                  <FormControl>
                    <Input {...field} />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />

            <FormField
              control={form.control}
              name='description'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Description')}</FormLabel>
                  <FormControl>
                    <Textarea rows={3} {...field} />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />

            <FormField
              control={form.control}
              name='param_schema'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Parameter Schema')}</FormLabel>
                  <FormControl>
                    <Textarea rows={8} className='font-mono' {...field} />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />

            <FormField
              control={form.control}
              name='sort_order'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Sort Order')}</FormLabel>
                  <FormControl>
                    <Input
                      type='number'
                      min={0}
                      value={field.value}
                      onChange={(e) =>
                        field.onChange(
                          e.target.value === '' ? 0 : Number(e.target.value)
                        )
                      }
                    />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />

            <FormField
              control={form.control}
              name='enabled'
              render={({ field }) => (
                <FormItem className={sideDrawerSwitchItemClassName()}>
                  <div className='flex flex-col gap-0.5'>
                    <FormLabel className='text-base'>{t('Enabled')}</FormLabel>
                  </div>
                  <FormControl>
                    <Switch
                      checked={field.value}
                      onCheckedChange={field.onChange}
                    />
                  </FormControl>
                </FormItem>
              )}
            />
          </form>
        </Form>

        <SheetFooter className={sideDrawerFooterClassName()}>
          <SheetClose
            render={<Button variant='outline' disabled={isSubmitting} />}
          >
            {t('Cancel')}
          </SheetClose>
          <Button
            form='api-doc-model-form'
            type='submit'
            disabled={isSubmitting}
          >
            {isSubmitting && <Loader2 className='mr-2 h-4 w-4 animate-spin' />}
            {t('Save changes')}
          </Button>
        </SheetFooter>
      </SheetContent>
    </Sheet>
  )
}
