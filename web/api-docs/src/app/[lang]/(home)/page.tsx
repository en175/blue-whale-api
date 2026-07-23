import Link from 'next/link';
import { BookOpen } from 'lucide-react';
import { Hero } from './page.client';
import { getLocalePath, i18n } from '@/lib/i18n';

const contentMap: Record<
  string,
  {
    badge: string;
    title: string;
    subtitle: string;
    highlight: string;
    getStarted: string;
  }
> = {
  en: {
    badge: 'Blue Whale API',
    title: 'Unified AI model API documentation,',
    subtitle: 'one platform to',
    highlight: 'connect them all',
    getStarted: 'Getting Started',
  },
  zh: {
    badge: 'Blue Whale API',
    title: '统一的 AI 模型接口文档，',
    subtitle: '一个平台',
    highlight: '连接所有模型',
    getStarted: '快速开始',
  },
  ja: {
    badge: 'Blue Whale API',
    title: '統合された AI モデル API ドキュメント、',
    subtitle: 'ひとつのプラットフォームで',
    highlight: 'すべてのモデルを接続',
    getStarted: 'はじめに',
  },
} as const;

export default async function Page({
  params,
}: {
  params: Promise<{ lang: string }>;
}) {
  const { lang } = await params;
  const content = contentMap[lang] || contentMap.en;

  return (
    <main className="text-landing-foreground dark:text-landing-foreground-dark pt-4 pb-6 md:pb-12">
      <div className="relative mx-auto flex h-[70vh] max-h-[900px] min-h-[600px] w-full max-w-[1400px] overflow-hidden rounded-2xl border bg-origin-border">
        <Hero />
        <div className="z-2 flex size-full flex-col px-4 max-md:items-center max-md:text-center md:p-12">
          <p className="border-brand/50 text-brand mt-12 w-fit rounded-full border p-2 text-xs font-medium">
            {content.badge}
          </p>
          <h1 className="leading-tighter my-8 text-4xl font-medium xl:mb-12 xl:text-5xl">
            {content.title}
            <br />
            {content.subtitle}{' '}
            <span className="text-brand">{content.highlight}</span>.
          </h1>
          <div className="flex w-fit flex-row flex-wrap items-center justify-center gap-4">
            <Link
              href={getLocalePath(lang, 'docs')}
              className="bg-brand text-brand-foreground hover:bg-brand-200 inline-flex items-center justify-center gap-2 rounded-full px-5 py-3 font-medium tracking-tight transition-colors max-sm:text-sm"
            >
              <BookOpen className="size-4" />
              {content.getStarted}
            </Link>
          </div>
        </div>
      </div>
    </main>
  );
}

export async function generateStaticParams() {
  return i18n.languages.map((lang) => ({ lang }));
}
