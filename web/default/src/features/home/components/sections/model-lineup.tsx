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
import { Link } from '@tanstack/react-router'
import { ArrowRight } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { AnimateInView } from '@/components/animate-in-view'
import { usePricingData } from '@/features/pricing/hooks/use-pricing-data'

import { IconCard } from '../icon-card'

/**
 * Horizontally scrolling wall of currently enabled models.
 *
 * Data comes from the shared /api/pricing endpoint (via usePricingData), so any
 * model enabled on the backend appears here without a frontend change. Cards link
 * to the pricing page for details. Intentionally bare (no section/heading) so it
 * can be injected into an admin-authored custom homepage via the
 * `<div data-home-model-lineup></div>` placeholder.
 */
export function ModelLineupWall() {
  const { t } = useTranslation()
  const { models, isLoading } = usePricingData()

  if (isLoading) {
    return <div className='text-muted-foreground text-sm'>{t('Loading...')}</div>
  }

  return (
    <div className='hover-scrollbar flex items-center gap-3 overflow-x-auto pb-4'>
      {models.map((model) => (
        <Link
          key={model.model_name}
          to='/pricing'
          className='shrink-0 outline-none'
          aria-label={model.model_name}
        >
          <IconCard
            iconName={model.icon || model.vendor_icon || model.model_name}
            label={model.model_name}
            size={36}
          />
        </Link>
      ))}
      <Link
        to='/pricing'
        className='border-border/50 bg-muted/30 text-muted-foreground hover:text-foreground flex h-[112px] shrink-0 items-center gap-2 rounded-2xl border px-6 text-sm font-medium transition-colors'
      >
        {t('View all models')}
        <ArrowRight className='size-4' />
      </Link>
    </div>
  )
}

/**
 * Default homepage section wrapper around {@link ModelLineupWall}: a heading plus
 * the scrolling wall. Used on the built-in homepage when no admin custom content
 * overrides it.
 */
export function ModelLineup() {
  const { t } = useTranslation()

  return (
    <section className='relative z-10 px-6 py-24 md:py-32'>
      <div className='mx-auto max-w-6xl'>
        <AnimateInView className='mb-12 max-w-lg'>
          <p className='text-muted-foreground mb-3 text-xs font-medium tracking-widest uppercase'>
            {t('Model Lineup')}
          </p>
          <h2 className='text-2xl leading-tight font-bold tracking-tight md:text-3xl'>
            {t('A broad lineup of models, always up to date')}
          </h2>
        </AnimateInView>

        <ModelLineupWall />
      </div>
    </section>
  )
}
