import { notFound } from 'next/navigation'

import { DocsPage } from '../../../components/docs-page'
import { getPublicPortalLanguage, publicPortalLanguages } from '../../../lib/public-video-scope'

type DocsIndexPageProps = {
  params: Promise<{ lang: string }>
}

export const dynamicParams = false

export function generateStaticParams() {
  return publicPortalLanguages.map((lang) => ({ lang }))
}

export default async function DocsIndexPage({ params }: DocsIndexPageProps) {
  const { lang } = await params
  const language = getPublicPortalLanguage(lang)

  if (!language) {
    notFound()
  }

  return <DocsPage language={language} slug='quickstart' />
}
