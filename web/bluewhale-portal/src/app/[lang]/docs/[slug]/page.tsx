import { notFound } from 'next/navigation'

import { DocsPage } from '../../../../components/docs-page'
import { documentationPages } from '../../../../lib/docs-content'
import { getPublicPortalLanguage } from '../../../../lib/public-video-scope'

type DocumentPageProps = {
  params: Promise<{ lang: string; slug: string }>
}

export const dynamicParams = false

export function generateStaticParams() {
  return documentationPages.map((page) => ({ lang: 'zh', slug: page.slug }))
}

export default async function DocumentPage({ params }: DocumentPageProps) {
  const { lang, slug } = await params
  const language = getPublicPortalLanguage(lang)

  if (!language || !documentationPages.some((page) => page.slug === slug)) {
    notFound()
  }

  return <DocsPage language={language} slug={slug} />
}
