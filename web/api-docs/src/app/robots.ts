import type { MetadataRoute } from 'next';
import { baseUrl } from '@/lib/metadata';

export default function robots(): MetadataRoute.Robots {
  const sitemapUrl = `${baseUrl.origin}/sitemap.xml`;

  return {
    rules: [
      {
        userAgent: '*',
        allow: '/',
        disallow: [
          '/api/', // API routes (internal endpoints)
          '/og/', // Open Graph image generation
        ],
      },
    ],
    sitemap: sitemapUrl,
    // robots.txt spec: Host should contain only the hostname (no scheme)
    host: baseUrl.hostname,
  };
}
