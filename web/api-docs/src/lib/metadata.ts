import type { Metadata } from 'next';

export function createMetadata(override: Metadata): Metadata {
  return {
    ...override,
    icons: {
      icon: '/favicon.ico',
      shortcut: '/favicon.ico',
      apple: '/assets/logo.png',
    },
    openGraph: {
      title: override.title ?? undefined,
      description: override.description ?? undefined,
      url: 'https://blue-whale.example.com',
      images: '/assets/logo.png',
      siteName: 'Blue Whale API',
      type: 'website',
      ...override.openGraph,
    },
    twitter: {
      card: 'summary_large_image',
      title: override.title ?? undefined,
      description: override.description ?? undefined,
      images: '/assets/logo.png',
      ...override.twitter,
    },
  };
}

export const baseUrl =
  process.env.NODE_ENV === 'development' ||
  !process.env.VERCEL_PROJECT_PRODUCTION_URL
    ? new URL('http://localhost:3011')
    : new URL(`https://${process.env.VERCEL_PROJECT_PRODUCTION_URL}`);
