import { zodResolver } from '@hookform/resolvers/zod'
import { useTranslation } from 'react-i18next'
import { useForm } from 'react-hook-form'

import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import {
  Form,
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Input } from '@/components/ui/input'

import { z } from 'zod'
import { tierFormSchema, type TierFormValues } from '../lib/schema'
import type { TopupTier } from '../lib/types'

interface TierFormDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  onSubmit: (values: TierFormValues) => void
  tier?: TopupTier | null
  submitting?: boolean
}

export function TierFormDialog(props: TierFormDialogProps) {
  const { t } = useTranslation()
  const isEdit = !!props.tier

  const form = useForm<z.infer<typeof tierFormSchema>>({
    resolver: zodResolver(tierFormSchema),
    defaultValues: props.tier
      ? {
          price_yuan: props.tier.price_yuan,
          points: props.tier.points,
          bonus_points: props.tier.bonus_points,
          sort_order: props.tier.sort_order,
          effective_at: props.tier.effective_at.slice(0, 16),
          expires_at: props.tier.expires_at.slice(0, 16),
        }
      : {
          price_yuan: 0,
          points: 0,
          bonus_points: 0,
          sort_order: 0,
          effective_at: '',
          expires_at: '',
        },
  })

  async function handleSubmit(values: TierFormValues) {
    await props.onSubmit(values)
    form.reset()
  }

  return (
    <Dialog open={props.open} onOpenChange={props.onOpenChange}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>
            {isEdit ? t('Edit Tier') : t('Create Tier')}
          </DialogTitle>
        </DialogHeader>
        <Form {...form}>
          <form
            onSubmit={form.handleSubmit(handleSubmit)}
            className="space-y-4"
          >
            <div className="grid grid-cols-2 gap-4">
              <FormField
                control={form.control}
                name="price_yuan"
                render={(fieldProps) => (
                  <FormItem>
                    <FormLabel>{t('Price (¥)')}</FormLabel>
                    <FormControl>
                      <Input
                        type="number"
                        step="0.01"
                        min="0"
                        disabled={fieldProps.field.disabled}
                        name={fieldProps.field.name}
                        value={fieldProps.field.value ?? ''}
                        onChange={(e) =>
                          fieldProps.field.onChange(
                            e.target.value ? Number(e.target.value) : undefined,
                          )
                        }
                        onBlur={fieldProps.field.onBlur}
                        ref={fieldProps.field.ref}
                      />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
              <FormField
                control={form.control}
                name="points"
                render={(fieldProps) => (
                  <FormItem>
                    <FormLabel>{t('Points')}</FormLabel>
                    <FormControl>
                      <Input
                        type="number"
                        min="0"
                        disabled={fieldProps.field.disabled}
                        name={fieldProps.field.name}
                        value={fieldProps.field.value ?? ''}
                        onChange={(e) =>
                          fieldProps.field.onChange(
                            e.target.value ? Number(e.target.value) : undefined,
                          )
                        }
                        onBlur={fieldProps.field.onBlur}
                        ref={fieldProps.field.ref}
                      />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
            </div>
            <div className="grid grid-cols-2 gap-4">
              <FormField
                control={form.control}
                name="bonus_points"
                render={(fieldProps) => (
                  <FormItem>
                    <FormLabel>{t('Bonus Points')}</FormLabel>
                    <FormControl>
                      <Input
                        type="number"
                        min="0"
                        disabled={fieldProps.field.disabled}
                        name={fieldProps.field.name}
                        value={fieldProps.field.value ?? ''}
                        onChange={(e) =>
                          fieldProps.field.onChange(
                            e.target.value ? Number(e.target.value) : undefined,
                          )
                        }
                        onBlur={fieldProps.field.onBlur}
                        ref={fieldProps.field.ref}
                      />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
              <FormField
                control={form.control}
                name="sort_order"
                render={(fieldProps) => (
                  <FormItem>
                    <FormLabel>{t('Sort Order')}</FormLabel>
                    <FormControl>
                      <Input
                        type="number"
                        min="0"
                        disabled={fieldProps.field.disabled}
                        name={fieldProps.field.name}
                        value={fieldProps.field.value ?? ''}
                        onChange={(e) =>
                          fieldProps.field.onChange(
                            e.target.value ? Number(e.target.value) : undefined,
                          )
                        }
                        onBlur={fieldProps.field.onBlur}
                        ref={fieldProps.field.ref}
                      />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
            </div>
            <div className="grid grid-cols-2 gap-4">
              <FormField
                control={form.control}
                name="effective_at"
                render={(fieldProps) => (
                  <FormItem>
                    <FormLabel>{t('Effective At')}</FormLabel>
                    <FormControl>
                      <Input
                        type="datetime-local"
                        disabled={fieldProps.field.disabled}
                        name={fieldProps.field.name}
                        value={fieldProps.field.value}
                        onChange={fieldProps.field.onChange}
                        onBlur={fieldProps.field.onBlur}
                        ref={fieldProps.field.ref}
                      />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
              <FormField
                control={form.control}
                name="expires_at"
                render={(fieldProps) => (
                  <FormItem>
                    <FormLabel>{t('Expires At')}</FormLabel>
                    <FormControl>
                      <Input
                        type="datetime-local"
                        disabled={fieldProps.field.disabled}
                        name={fieldProps.field.name}
                        value={fieldProps.field.value}
                        onChange={fieldProps.field.onChange}
                        onBlur={fieldProps.field.onBlur}
                        ref={fieldProps.field.ref}
                      />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
            </div>
            <DialogFooter>
              <Button
                type="button"
                variant="outline"
                onClick={() => props.onOpenChange(false)}
              >
                {t('Cancel')}
              </Button>
              <Button type="submit" disabled={props.submitting}>
                {props.submitting ? t('Saving...') : t('Save')}
              </Button>
            </DialogFooter>
          </form>
        </Form>
      </DialogContent>
    </Dialog>
  )
}
