import { zodResolver } from '@hookform/resolvers/zod'
import { z } from 'zod'
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

import type { AdminUserFull } from '../lib/types'

const adjustSchema = z.object({
  amount: z.number().refine((v) => v !== 0, { message: 'Amount cannot be zero' }),
  reason: z.string().min(1, 'Reason is required'),
})

type AdjustFormValues = z.infer<typeof adjustSchema>

interface AdjustDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  user: AdminUserFull | null
  onSubmit: (values: { userId: number; amount: number; reason: string }) => void
  submitting?: boolean
}

export function AdjustDialog(props: AdjustDialogProps) {
  const { t } = useTranslation()

  const form = useForm<z.infer<typeof adjustSchema>>({
    resolver: zodResolver(adjustSchema),
    defaultValues: { amount: 0, reason: '' },
  })

  function handleSubmit(values: AdjustFormValues) {
    if (!props.user) return
    props.onSubmit({ userId: props.user.id, amount: values.amount, reason: values.reason })
  }

  if (!props.user) return null

  return (
    <Dialog open={props.open} onOpenChange={props.onOpenChange}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>
            {t('Adjust Points — {{name}}', { name: props.user.display_name || props.user.username })}
          </DialogTitle>
        </DialogHeader>
        <Form {...form}>
          <form onSubmit={form.handleSubmit(handleSubmit)} className="space-y-4">
            <div className="text-muted-foreground text-sm">
              {t('Current Balance')}: <span className="font-bold text-foreground">{props.user.points_balance.toLocaleString()}</span>
            </div>
            <FormField
              control={form.control}
              name="amount"
              render={(fieldProps) => (
                <FormItem>
                  <FormLabel>{t('Amount (+ add, − deduct)')}</FormLabel>
                  <FormControl>
                    <Input
                      type="number"
                      disabled={fieldProps.field.disabled || props.submitting}
                      name={fieldProps.field.name}
                      value={fieldProps.field.value ?? ''}
                      onChange={(e) =>
                        fieldProps.field.onChange(
                          e.target.value ? Number(e.target.value) : 0,
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
              name="reason"
              render={(fieldProps) => (
                <FormItem>
                  <FormLabel>{t('Reason')}</FormLabel>
                  <FormControl>
                    <Input
                      disabled={fieldProps.field.disabled || props.submitting}
                      name={fieldProps.field.name}
                      value={fieldProps.field.value}
                      onChange={fieldProps.field.onChange}
                      onBlur={fieldProps.field.onBlur}
                      ref={fieldProps.field.ref}
                      placeholder={t('e.g. event reward, customer support')}
                    />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />
            <DialogFooter>
              <Button type="button" variant="outline" onClick={() => props.onOpenChange(false)} disabled={props.submitting}>
                {t('Cancel')}
              </Button>
              <Button type="submit" disabled={props.submitting}>
                {props.submitting ? t('Saving...') : t('Confirm Adjust')}
              </Button>
            </DialogFooter>
          </form>
        </Form>
      </DialogContent>
    </Dialog>
  )
}
