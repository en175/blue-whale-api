'use client';

import { useEffect, useState } from 'react';

type DocModel = {
  model_name: string;
  display_name: string;
  description: string;
  tag: string;
  sort_order: number;
};

const L = {
  zh: {
    empty: '暂无对外开放的模型',
    loading: '加载中…',
    error: '模型列表加载失败',
  },
  en: {
    empty: 'No publicly available models',
    loading: 'Loading…',
    error: 'Failed to load model list',
  },
} as const;

export function LiveModels({ lang = 'zh' }: { lang?: 'zh' | 'en' }) {
  const t = L[lang];
  const [models, setModels] = useState<DocModel[] | null>(null);
  const [error, setError] = useState('');

  useEffect(() => {
    const base = (process.env.NEXT_PUBLIC_API_BASE_URL || '').replace(/\/$/, '');
    fetch(`${base}/api/docs/config`)
      .then((r) => {
        if (!r.ok) throw new Error(`HTTP ${r.status}`);
        return r.json();
      })
      .then((json) => setModels((json?.data?.models as DocModel[]) ?? []))
      .catch((e) => setError(String(e)));
  }, []);

  if (error) return <p className="text-sm">{t.error}</p>;
  if (models === null) return <p className="text-sm">{t.loading}</p>;
  if (models.length === 0) return <p className="text-sm">{t.empty}</p>;

  return (
    <div className="grid gap-4 sm:grid-cols-2">
      {models.map((m) => (
        <div key={m.model_name} className="rounded-lg border p-4">
          <div className="flex items-center gap-2">
            <span className="font-semibold">{m.display_name || m.model_name}</span>
            {m.tag ? (
              <span className="rounded-full bg-primary/10 px-2 py-0.5 text-xs font-medium text-primary">
                {m.tag}
              </span>
            ) : null}
          </div>
          {m.description ? (
            <p className="mt-1 text-sm text-muted-foreground">{m.description}</p>
          ) : null}
        </div>
      ))}
    </div>
  );
}
