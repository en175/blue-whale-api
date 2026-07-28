import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'

import { Button } from '@/components/ui/button'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'

import {
  createTier,
  deleteTier,
  fetchTiers,
  toggleTierStatus,
  updateSortOrder,
  updateTier,
} from '../api/mock'
import type { TierFormValues } from '../lib/schema'
import type { TopupTier } from '../lib/types'
import { TierFormDialog } from './tier-form-dialog'
import { TierPreviewDrawer } from './tier-preview-drawer'
import { TierTable } from './tier-table'

export function TiersPage() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [statusFilter, setStatusFilter] = useState('all')
  const [formOpen, setFormOpen] = useState(false)
  const [editingTier, setEditingTier] = useState<TopupTier | null>(null)

  const params: Record<string, string> = {}
  if (statusFilter !== 'all') params.status = statusFilter

  const { data: tiers = [] } = useQuery({
    queryKey: ['admin', 'tiers', params],
    queryFn: () => fetchTiers(params as { status?: string }),
    select: (res) => res.data?.tiers ?? [],
  })

  const invalidate = () => queryClient.invalidateQueries({ queryKey: ['admin', 'tiers'] })

  const onError = () => toast.error(t('Operation failed'))

  const createMutation = useMutation({
    mutationFn: (values: TierFormValues) => createTier(values),
    onSuccess: () => { setFormOpen(false); invalidate(); toast.success(t('Tier created')) },
    onError,
  })

  const updateMutation = useMutation({
    mutationFn: ({ id, values }: { id: number; values: TierFormValues }) => updateTier(id, values),
    onSuccess: () => { setFormOpen(false); setEditingTier(null); invalidate(); toast.success(t('Tier updated')) },
    onError,
  })

  const deleteMutation = useMutation({
    mutationFn: (id: number) => deleteTier(id),
    onSuccess: (res) => {
      if (!res.success) { toast.error(res.message); return }
      invalidate()
      toast.success(t('Tier deleted'))
    },
    onError,
  })

  const toggleMutation = useMutation({
    mutationFn: (id: number) => toggleTierStatus(id),
    onSuccess: () => { invalidate(); toast.success(t('Status updated')) },
    onError,
  })

  const reorderMutation = useMutation({
    mutationFn: (reordered: TopupTier[]) => {
      const items = reordered.map((t, i) => ({ id: t.id, sort_order: i + 1 }))
      return updateSortOrder(items)
    },
    onSuccess: () => { invalidate(); toast.success(t('Sort order saved')) },
    onError,
  })

  function handleSubmit(values: TierFormValues) {
    if (editingTier) {
      updateMutation.mutate({ id: editingTier.id, values })
    } else {
      createMutation.mutate(values)
    }
  }

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-bold tracking-tight">
          {t('Top-up Tiers')}
        </h1>
        <div className="flex items-center gap-3">
          <Select value={statusFilter} onValueChange={(v) => v && setStatusFilter(v)}>
            <SelectTrigger className="w-36">
              <SelectValue placeholder={t('All')} />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="all">{t('All')}</SelectItem>
              <SelectItem value="active">{t('Active')}</SelectItem>
              <SelectItem value="inactive">{t('Inactive')}</SelectItem>
            </SelectContent>
          </Select>
          <TierPreviewDrawer />
          <Button
            onClick={() => {
              setEditingTier(null)
              setFormOpen(true)
            }}
          >
            {t('Create Tier')}
          </Button>
        </div>
      </div>

      <TierTable
        tiers={tiers}
        onEdit={(tier) => { setEditingTier(tier); setFormOpen(true) }}
        onDelete={(tier) => { if (confirm(t('Delete this tier?'))) deleteMutation.mutate(tier.id) }}
        onToggleStatus={(tier) => toggleMutation.mutate(tier.id)}
        onReorder={(reordered) => reorderMutation.mutate(reordered)}
      />

      <TierFormDialog
        open={formOpen}
        onOpenChange={(open) => {
          setFormOpen(open)
          if (!open) setEditingTier(null)
        }}
        onSubmit={handleSubmit}
        tier={editingTier}
        submitting={createMutation.isPending || updateMutation.isPending}
      />
    </div>
  )
}
