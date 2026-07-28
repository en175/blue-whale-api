import {
  closestCenter,
  DndContext,
  type DragEndEvent,
  KeyboardSensor,
  PointerSensor,
  useSensor,
  useSensors,
} from '@dnd-kit/core'
import { restrictToVerticalAxis } from '@dnd-kit/modifiers'
import {
  arrayMove,
  SortableContext,
  sortableKeyboardCoordinates,
  useSortable,
  verticalListSortingStrategy,
} from '@dnd-kit/sortable'
import { CSS } from '@dnd-kit/utilities'
import { GripVertical } from 'lucide-react'
import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { cn } from '@/lib/utils'

import type { TopupTier } from '../lib/types'

function formatDate(iso: string, locale: string) {
  return new Date(iso).toLocaleDateString(locale)
}

function DraggableRow(props: { tier: TopupTier; children: ReactNode }) {
  const { t: td } = useTranslation()
  const {
    attributes,
    listeners,
    setNodeRef,
    transform,
    transition,
    isDragging,
  } = useSortable({ id: props.tier.id })

  return (
    <TableRow
      ref={setNodeRef}
      className={cn(isDragging && 'relative z-10 opacity-50')}
      style={{ transform: CSS.Translate.toString(transform), transition }}
    >
      <TableCell className="w-10">
        <Button
          variant="ghost"
          size="icon"
          className="size-8 cursor-grab touch-none"
          {...attributes}
          {...listeners}
        >
          <GripVertical className="size-4" aria-label={td('Drag to reorder')} />
        </Button>
      </TableCell>
      {props.children}
    </TableRow>
  )
}

interface TierTableProps {
  tiers: TopupTier[]
  onEdit: (tier: TopupTier) => void
  onDelete: (tier: TopupTier) => void
  onToggleStatus: (tier: TopupTier) => void
  onReorder: (tiers: TopupTier[]) => void
}

export function TierTable(props: TierTableProps) {
  const { t, i18n } = useTranslation()
  const sensors = useSensors(
    useSensor(PointerSensor, { activationConstraint: { distance: 5 } }),
    useSensor(KeyboardSensor, {
      coordinateGetter: sortableKeyboardCoordinates,
    }),
  )

  function handleDragEnd(event: DragEndEvent) {
    const { active, over } = event
    if (!over || active.id === over.id) return
    const oldIdx = props.tiers.findIndex((ti) => ti.id === active.id)
    const newIdx = props.tiers.findIndex((ti) => ti.id === over.id)
    if (oldIdx === -1 || newIdx === -1) return
    const reordered = arrayMove(props.tiers, oldIdx, newIdx).map(
      (ti, i) => ({ ...ti, sort_order: i + 1 }),
    )
    props.onReorder(reordered)
  }

  const columns = [
    { key: 'price_yuan', label: 'Price (¥)', format: (ti: TopupTier) => `¥${ti.price_yuan}` },
    { key: 'points', label: 'Points', format: (ti: TopupTier) => `${ti.points.toLocaleString()}` },
    { key: 'bonus_points', label: 'Bonus', format: (ti: TopupTier) => `${ti.bonus_points}` },
    { key: 'status', label: 'Status', format: (ti: TopupTier, renderT: typeof t) =>
      ti.is_active
        ? <Badge variant="default">{renderT('Active')}</Badge>
        : <Badge variant="secondary">{renderT('Inactive')}</Badge>
    },
    { key: 'effective_at', label: 'Effective', format: (ti: TopupTier) => formatDate(ti.effective_at, i18n.language) },
    { key: 'expires_at', label: 'Expires', format: (ti: TopupTier) => formatDate(ti.expires_at, i18n.language) },
    { key: 'charge_count', label: 'Orders', format: (ti: TopupTier) => `${ti.charge_count}` },
    { key: 'total_revenue', label: 'Revenue', format: (ti: TopupTier) => `¥${ti.total_revenue.toLocaleString()}` },
  ]

  return (
    <DndContext
      sensors={sensors}
      collisionDetection={closestCenter}
      onDragEnd={handleDragEnd}
      modifiers={[restrictToVerticalAxis]}
    >
      <SortableContext
        items={props.tiers.map((ti) => ti.id)}
        strategy={verticalListSortingStrategy}
      >
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead className="w-10" />
              {columns.map((col) => (
                <TableHead key={col.key}>{t(col.label)}</TableHead>
              ))}
              <TableHead className="w-40 text-right">{t('Actions')}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {props.tiers.map((tier) => (
              <DraggableRow key={tier.id} tier={tier}>
                {columns.map((col) => (
                  <TableCell key={col.key}>{col.format(tier, t)}</TableCell>
                ))}
                <TableCell className="text-right">
                  <div className="flex items-center justify-end gap-1">
                    <Button
                      variant="outline"
                      size="sm"
                      onClick={() => props.onToggleStatus(tier)}
                    >
                      {tier.is_active ? t('Deactivate') : t('Activate')}
                    </Button>
                    <Button
                      variant="outline"
                      size="sm"
                      onClick={() => props.onEdit(tier)}
                    >
                      {t('Edit')}
                    </Button>
                    <Button
                      variant="outline"
                      size="sm"
                      onClick={() => props.onDelete(tier)}
                    >
                      {t('Delete')}
                    </Button>
                  </div>
                </TableCell>
              </DraggableRow>
            ))}
            {props.tiers.length === 0 && (
              <TableRow>
                <TableCell colSpan={columns.length + 2} className="text-muted-foreground py-8 text-center">
                  {t('No tiers found')}
                </TableCell>
              </TableRow>
            )}
          </TableBody>
        </Table>
      </SortableContext>
    </DndContext>
  )
}
