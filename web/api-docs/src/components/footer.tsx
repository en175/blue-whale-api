import Link from 'next/link';
import { getLocalePath } from '@/lib/i18n';

interface FooterProps {
  lang: string;
}

// ============================================
// 社交链接：Blue Whale 不外跳 git，留空
// ============================================
const socialLinks: { name: string; href: string; icon: React.ReactNode }[] = [];

// 外部相关/友情链接（Blue Whale 待补充，暂留空）
const relatedProjects: { label: string; href: string }[] = [];
const friendshipLinks: { label: string; href: string }[] = [];

// ============================================
// 内部链接路径（仅 label 需翻译）
// ============================================
const internalPaths = {
  apiDocs: 'docs/api',
} as const;

// ============================================
// 翻译（仅各语言不同的文本）
// ============================================
interface FooterTranslation {
  sections: {
    docs: {
      title: string;
      apiDocs: string;
    };
    relatedProjects: string;
    friendshipLinks: string;
  };
  copyright: string;
}

const translations: Record<string, FooterTranslation> = {
  zh: {
    sections: {
      docs: {
        title: '文档',
        apiDocs: 'API 文档',
      },
      relatedProjects: '相关项目',
      friendshipLinks: '友情链接',
    },
    copyright:
      '© 2023-2026 Blue Whale API 文档. All Rights Reserved.',
  },
  en: {
    sections: {
      docs: {
        title: 'Docs',
        apiDocs: 'API Docs',
      },
      relatedProjects: 'Related Projects',
      friendshipLinks: 'Friendship Links',
    },
    copyright:
      '© 2023-2026 Blue Whale API Docs. All Rights Reserved.',
  },
  ja: {
    sections: {
      docs: {
        title: 'ドキュメント',
        apiDocs: 'APIドキュメント',
      },
      relatedProjects: '関連プロジェクト',
      friendshipLinks: '友好リンク',
    },
    copyright:
      '© 2023-2026 Blue Whale API Docs. All Rights Reserved.',
  },
};

// ============================================
// 组装 sections（空 links 的 section 自动过滤）
// ============================================
function buildSections(t: FooterTranslation) {
  return [
    {
      title: t.sections.docs.title,
      links: [
        { label: t.sections.docs.apiDocs, href: internalPaths.apiDocs },
      ],
    },
    {
      title: t.sections.relatedProjects,
      links: relatedProjects.map((p) => ({ ...p, external: true })),
    },
    {
      title: t.sections.friendshipLinks,
      links: friendshipLinks.map((p) => ({ ...p, external: true })),
    },
  ].filter((s) => s.links.length > 0);
}

// ============================================
// Footer Component
// ============================================
export function Footer({ lang }: FooterProps) {
  const t = translations[lang] || translations.en;
  const sections = buildSections(t);

  return (
    <footer className="border-fd-border bg-fd-card/30 mt-auto border-t backdrop-blur-sm">
      <div className="mx-auto max-w-[1400px] px-6 py-12">
        {/* Top: Links Grid */}
        <div className="grid grid-cols-2 gap-x-8 gap-y-10 pb-10 md:grid-cols-3 lg:grid-cols-4 lg:gap-x-12">
          {sections.map((section) => (
            <div key={section.title}>
              <h3 className="text-fd-foreground mb-4 text-sm font-semibold">
                {section.title}
              </h3>
              <ul className="space-y-3">
                {section.links.map((link) => (
                  <li key={link.href}>
                    {'external' in link && link.external ? (
                      <a
                        href={link.href}
                        target="_blank"
                        rel="noopener noreferrer"
                        className="text-fd-muted-foreground hover:text-fd-foreground text-sm transition-colors"
                      >
                        {link.label}
                      </a>
                    ) : (
                      <Link
                        href={getLocalePath(lang, link.href)}
                        className="text-fd-muted-foreground hover:text-fd-foreground text-sm transition-colors"
                      >
                        {link.label}
                      </Link>
                    )}
                  </li>
                ))}
              </ul>
            </div>
          ))}
        </div>

        {/* Bottom: Copyright and Social */}
        <div className="border-fd-border flex flex-col items-start justify-between gap-4 border-t pt-8 sm:flex-row sm:items-center">
          <div className="text-fd-muted-foreground flex flex-col gap-2 text-xs">
            <p>{t.copyright}</p>
          </div>
          <div className="flex items-center gap-4">
            {socialLinks.map((social) => {
              const isExternal = social.href.startsWith('http');
              const Component = isExternal ? 'a' : Link;
              return (
                <Component
                  key={social.name}
                  href={
                    isExternal ? social.href : getLocalePath(lang, social.href)
                  }
                  {...(isExternal && {
                    target: '_blank',
                    rel: 'noopener noreferrer',
                  })}
                  className="text-fd-muted-foreground hover:text-fd-foreground transition-colors"
                  aria-label={social.name}
                >
                  {social.icon}
                </Component>
              );
            })}
          </div>
        </div>
      </div>
    </footer>
  );
}
