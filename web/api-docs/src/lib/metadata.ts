import type { Metadata } from 'next';

const logoUrl =
  'https://bluewhale-assets.tos-cn-beijing.volces.com/blue-whale-api/blue_whale_v2.svg';

export function createMetadata(override: Metadata): Metadata {
  return {
    ...override,
    icons: {
      icon: { url: logoUrl, type: 'image/svg+xml' },
      shortcut: logoUrl,
      apple: '/assets/logo.png',
    },
    openGraph: {
      title: override.title ?? undefined,
      description: override.description ?? undefined,
      url: 'https://blue-whale.example.com',
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

export const baseUrl =
  process.env.NODE_ENV === 'development' ||
  !process.env.VERCEL_PROJECT_PRODUCTION_URL
    ? new URL('http://localhost:3011')
    : new URL(`https://${process.env.VERCEL_PROJECT_PRODUCTION_URL}`);
