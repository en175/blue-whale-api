import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'

import { Button } from '@/components/ui/button'

import {
  createPricingRule,
  deletePricingRule,
  fetchPricingRules,
  togglePricingRule,
  updatePricingRule,
} from '../api/mock'
import type { PricingFormValues } from '../lib/schema'
import type { PricingRule } from '../lib/types'
import { PricingFormDialog } from './pricing-form-dialog'
import { PricingTable } from './pricing-table'

export function PricingPage() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [formOpen, setFormOpen] = useState(false)
  const [editingRule, setEditingRule] = useState<PricingRule | null>(null)

  const { data: rules = [] } = useQuery({
    queryKey: ['admin', 'pricing'],
    queryFn: fetchPricingRules,
    select: (res) => res.data ?? [],
  })

  const invalidate = () => queryClient.invalidateQueries({ queryKey: ['admin', 'pricing'] })

  const onError = () => toast.error(t('Operation failed'))

  const createMutation = useMutation({
    mutationFn: (values: PricingFormValues) => createPricingRule(values),
    onSuccess: () => { setFormOpen(false); invalidate(); toast.success(t('Pricing rule created')) },
    onError,
  })

  const updateMutation = useMutation({
    mutationFn: ({ id, values }: { id: number; values: PricingFormValues }) => updatePricingRule(id, values),
    onSuccess: () => { setFormOpen(false); setEditingRule(null); invalidate(); toast.success(t('Pricing rule updated')) },
    onError,
  })

  const deleteMutation = useMutation({
    mutationFn: (id: number) => deletePricingRule(id),
    onSuccess: () => { invalidate(); toast.success(t('Pricing rule deleted')) },
    onError,
  })

  const toggleMutation = useMutation({
    mutationFn: (id: number) => togglePricingRule(id),
    onSuccess: () => { invalidate(); toast.success(t('Status updated')) },
    onError,
  })

  function handleSubmit(values: PricingFormValues) {
    if (editingRule) {
      updateMutation.mutate({ id: editingRule.id, values })
    } else {
      createMutation.mutate(values)
    }
  }

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-bold tracking-tight">
          {t('Pricing Rules')}
        </h1>
        <Button
          onClick={() => {
            setEditingRule(null)
            setFormOpen(true)
          }}
        >
          {t('Create Pricing Rule')}
        </Button>
      </div>

      <PricingTable
        rules={rules}
        onEdit={(rule) => { setEditingRule(rule); setFormOpen(true) }}
        onDelete={(rule) => { if (confirm(t('Delete this pricing rule?'))) deleteMutation.mutate(rule.id) }}
        onToggle={(rule) => toggleMutation.mutate(rule.id)}
      />

      <PricingFormDialog
        open={formOpen}
        onOpenChange={(open) => {
          setFormOpen(open)
          if (!open) setEditingRule(null)
        }}
        onSubmit={handleSubmit}
        rule={editingRule}
        submitting={createMutation.isPending || updateMutation.isPending}
      />
    </div>
  )
}
