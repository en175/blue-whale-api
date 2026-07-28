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

import { pricingFormSchema, type PricingFormValues } from '../lib/schema'
import type { PricingRule } from '../lib/types'

interface PricingFormDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  onSubmit: (values: PricingFormValues) => void
  rule?: PricingRule | null
  submitting?: boolean
}

export function PricingFormDialog(props: PricingFormDialogProps) {
  const { t } = useTranslation()
  const isEdit = !!props.rule

  const form = useForm<z.infer<typeof pricingFormSchema>>({
    resolver: zodResolver(pricingFormSchema),
    defaultValues: props.rule
      ? {
          model: props.rule.model,
          input_price: props.rule.input_price,
          output_price: props.rule.output_price,
          unit: props.rule.unit,
          ratio: props.rule.ratio,
        }
      : {
          model: '',
          input_price: 0,
          output_price: 0,
          unit: '1K tokens',
          ratio: 1.0,
        },
  })

  async function handleSubmit(values: PricingFormValues) {
    await props.onSubmit(values)
    form.reset()
  }

  return (
    <Dialog open={props.open} onOpenChange={props.onOpenChange}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>
            {isEdit ? t('Edit Pricing Rule') : t('Create Pricing Rule')}
          </DialogTitle>
        </DialogHeader>
        <Form {...form}>
          <form
            onSubmit={form.handleSubmit(handleSubmit)}
            className="space-y-4"
          >
            <FormField
              control={form.control}
              name="model"
              render={(fieldProps) => (
                <FormItem>
                  <FormLabel>{t('Model')}</FormLabel>
                  <FormControl>
                    <Input
                      disabled={fieldProps.field.disabled}
                      name={fieldProps.field.name}
                      value={fieldProps.field.value}
                      onChange={fieldProps.field.onChange}
                      onBlur={fieldProps.field.onBlur}
                      ref={fieldProps.field.ref}
                      placeholder="gpt-4o"
                    />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />
            <div className="grid grid-cols-2 gap-4">
              <FormField
                control={form.control}
                name="input_price"
                render={(fieldProps) => (
                  <FormItem>
                    <FormLabel>{t('Input Price')}</FormLabel>
                    <FormControl>
                      <Input
                        type="number"
                        step="0.0001"
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
                name="output_price"
                render={(fieldProps) => (
                  <FormItem>
                    <FormLabel>{t('Output Price')}</FormLabel>
                    <FormControl>
                      <Input
                        type="number"
                        step="0.0001"
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
                name="unit"
                render={(fieldProps) => (
                  <FormItem>
                    <FormLabel>{t('Unit')}</FormLabel>
                    <FormControl>
                      <Input
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
                name="ratio"
                render={(fieldProps) => (
                  <FormItem>
                    <FormLabel>{t('Ratio')}</FormLabel>
                    <FormControl>
                      <Input
                        type="number"
                        step="0.1"
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
