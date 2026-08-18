import type { Metadata } from 'next';
import { readFileSync } from 'node:fs';
import path from 'node:path';

const logoUrl =
  'https://bluewhale-assets.tos-cn-beijing.volces.com/blue-whale-api/blue_whale_v2.svg';

function resolveSiteUrl(): URL {
  const envUrl = process.env.NEXT_PUBLIC_SITE_URL;
  if (envUrl) {
    try {
      return new URL(envUrl);
    } catch {
      // 非法 env 值，回退
    }
  }
  try {
    const raw = readFileSync(
      path.join(process.cwd(), 'generated/site-config.json'),
      'utf8',
    );
    const cfg = JSON.parse(raw) as { docs_domain?: string };
    if (cfg.docs_domain) {
      return new URL(cfg.docs_domain);
    }
  } catch {
    // 无生成文件或非法 JSON，回退本地
  }
  return new URL('http://localhost:3011');
}

export const baseUrl = resolveSiteUrl();

export function createMetadata(override: Metadata): Metadata {
  return {
    ...override,
    icons: {
      icon: [{ url: '/favicon.png', sizes: '256x256', type: 'image/png' }],
      apple: [{ url: '/favicon.png', sizes: '256x256', type: 'image/png' }],
    },
    openGraph: {
      title: override.title ?? undefined,
      description: override.description ?? undefined,
      url: baseUrl.origin,
      images: logoUrl,
      siteName: 'Blue Whale API',
      type: 'website',
      ...override.openGraph,
    },
    twitter: {
      card: 'summary_large_image',
      title: override.title ?? undefined,
      description: override.description ?? undefined,
      images: logoUrl,
      ...override.twitter,
    },
  };
}
