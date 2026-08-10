import { documentationPages } from '../lib/docs-content'
import type { PortalLanguage } from '../lib/public-video-scope'

type DocsPageProps = {
  language: PortalLanguage
  slug: string
}

export function DocsPage({ language, slug }: DocsPageProps) {
  const page = documentationPages.find((item) => item.slug === slug) ?? documentationPages[0]

  return (
    <main className='docs-shell'>
      <header className='docs-header'>
        <a className='brand' href={'/' + language} aria-label='BLUE WHALE API 首页'>
          <span className='brand-mark' aria-hidden='true'>
            ≋
          </span>
          <span>BLUE WHALE API</span>
        </a>
        <a className='docs-home-link' href={'/' + language}>
          返回官网
        </a>
      </header>
      <div className='docs-layout'>
        <aside className='docs-sidebar' aria-label='文档导航'>
          <p>DOCUMENTATION</p>
          <nav>
            {documentationPages.map((item) => (
              <a
                className={item.slug === page.slug ? 'active' : undefined}
                href={'/' + language + '/docs/' + item.slug}
                key={item.slug}
              >
                {item.label}
              </a>
            ))}
          </nav>
        </aside>
        <article className='docs-article'>
          <p className='eyebrow'>{page.eyebrow}</p>
          <h1>{page.title}</h1>
          <p className='docs-summary'>{page.summary}</p>
          {page.sections.map((section) => (
            <section className='docs-section' key={section.heading}>
              <h2>{section.heading}</h2>
              <p>{section.body}</p>
            </section>
          ))}
        </article>
      </div>
    </main>
  )
}
