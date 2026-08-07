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
import { useCallback, useEffect, useRef, useState } from 'react'
import { createPortal } from 'react-dom'
import { useTranslation } from 'react-i18next'

import { PublicLayout } from '@/components/layout'
import { Footer } from '@/components/layout/components/footer'
import { RichContent } from '@/components/rich-content'
import { useTheme } from '@/context/theme-provider'
import { isLikelyHtml } from '@/lib/content-format'
import { useAuthStore } from '@/stores/auth-store'

import { CTA, Features, Hero, HowItWorks, ModelLineup, ModelLineupWall, Stats } from './components'
import { useHomePageContent } from './hooks'

/**
 * Renders an admin-authored custom homepage (HTML) and injects the data-driven
 * model lineup into any `<div data-home-model-lineup></div>` placeholder via a
 * portal. The custom HTML is sandboxed into a shadow root by HtmlContent
 * (isolated variant), so the placeholder is located through that shadow root.
 * A portal (rather than a detached createRoot) keeps router / query contexts
 * available to the injected lineup.
 */
function CustomHomeHtml({ content }: { content: string }) {
  const containerRef = useRef<HTMLDivElement>(null)
  const [lineupSlots, setLineupSlots] = useState<Element[]>([])

  useEffect(() => {
    const container = containerRef.current
    if (!container) return
    const host = container.querySelector<HTMLElement>('.custom-home-content')
    const shadow = host?.shadowRoot
    if (!shadow) {
      setLineupSlots([])
      return
    }
    const slots = shadow.querySelectorAll('[data-home-model-lineup]')
    // Clear any static fallback the admin template nested inside the placeholder
    // before the portal renders the real wall; React only manages its own portal
    // children, so clearing the slot here is safe.
    slots.forEach((slot) => slot.replaceChildren())
    setLineupSlots([...slots])
  }, [content])

  return (
    <div ref={containerRef}>
      <RichContent
        mode='html'
        htmlVariant='isolated'
        content={content}
        className='custom-home-content'
      />
      {lineupSlots.map((slot) =>
        createPortal(<ModelLineupWall key={slot} />, slot)
      )}
    </div>
  )
}

export function Home() {
  const { i18n, t } = useTranslation()
  const iframeRef = useRef<HTMLIFrameElement>(null)
  const { resolvedTheme } = useTheme()
  const { auth } = useAuthStore()
  const isAuthenticated = !!auth.user
  const { content, isLoaded, isUrl } = useHomePageContent()

  const syncIframePreferences = useCallback(() => {
    try {
      iframeRef.current?.contentWindow?.postMessage(
        { themeMode: resolvedTheme },
        '*'
      )
      iframeRef.current?.contentWindow?.postMessage(
        { lang: i18n.language },
        '*'
      )
    } catch {
      // Cross-origin frames may reject access while navigating.
    }
  }, [i18n.language, resolvedTheme])

  useEffect(() => {
    if (isUrl) {
      syncIframePreferences()
    }
  }, [isUrl, syncIframePreferences])

  if (!isLoaded) {
    return (
      <PublicLayout showMainContainer={false}>
        <main className='flex min-h-screen items-center justify-center'>
          <div className='text-muted-foreground'>{t('Loading...')}</div>
        </main>
      </PublicLayout>
    )
  }

  if (content) {
    if (isUrl) {
      return (
        <PublicLayout showMainContainer={false}>
          {/*
            allow-top-navigation-by-user-activation: the custom home page URL is
            admin-configured (trusted); this lets its target="_top" nav/menu links
            navigate the top-level window on user click. The default sandbox blocks
            this on desktop, while some mobile browsers allow it via allow-popups,
            causing inconsistent behavior. This token only permits user-activated
            top-level navigation and does NOT grant same-origin access.
          */}
          <iframe
            ref={iframeRef}
            src={content}
            className='h-screen w-full border-none'
            title={t('Custom Home Page')}
            sandbox='allow-forms allow-popups allow-popups-to-escape-sandbox allow-scripts allow-top-navigation-by-user-activation'
            onLoad={syncIframePreferences}
          />
        </PublicLayout>
      )
    }

    const contentIsHtml = isLikelyHtml(content)

    if (contentIsHtml) {
      return (
        <PublicLayout showMainContainer={false}>
          <CustomHomeHtml content={content} />
        </PublicLayout>
      )
    }

    return (
      <PublicLayout>
        <div className='mx-auto max-w-6xl px-4 py-8'>
          <RichContent
            mode='markdown'
            content={content}
            className='custom-home-content'
          />
        </div>
      </PublicLayout>
    )
  }

  return (
    <PublicLayout showMainContainer={false}>
      <Hero isAuthenticated={isAuthenticated} />
      <Stats />
      <Features />
      <ModelLineup />
      <HowItWorks />
      <CTA isAuthenticated={isAuthenticated} />
      <Footer />
    </PublicLayout>
  )
}
