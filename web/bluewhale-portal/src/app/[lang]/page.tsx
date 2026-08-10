import { notFound } from 'next/navigation'

import { HomePage } from '../../components/home-page'
import { getPublicPortalLanguage, publicPortalLanguages } from '../../lib/public-video-scope'

type LocalePageProps = {
  params: Promise<{ lang: string }>
}

export const dynamicParams = false

export function generateStaticParams() {
  return publicPortalLanguages.map((lang) => ({ lang }))
}

export default async function LocalePage({ params }: LocalePageProps) {
  const { lang } = await params
  const language = getPublicPortalLanguage(lang)

  if (!language) {
    notFound()
  }

  return <HomePage language={language} />
}
