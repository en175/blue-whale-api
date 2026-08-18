'use client';

type DocModel = {
  model_name: string;
  display_name: string;
  description: string;
  tag: string;
  sort_order: number;
};

// 模型列表改为静态内置（来源：网关 GET /api/docs/config 的 data.models）
// 网关模型有增删时，需手动同步此数组并重新构建部署
const STATIC_MODELS: DocModel[] = [
  {
    model_name: 'doubao-seedance-2-5-260628',
    display_name: 'Doubao-Seedance-2.5',
    description: '30秒超长叙事，全模态参考扩容',
    tag: '全面开放',
    sort_order: 1,
  },
  {
    model_name: 'doubao-seedance-2-0-260128',
    display_name: 'Doubao-Seedance-2.0',
    description: '超强的参考一致性，极致拟真的视听稳定性',
    tag: '',
    sort_order: 2,
  },
  {
    model_name: 'doubao-seedance-2-0-mini-260615',
    display_name: 'Doubao-Seedance-2.0-mini',
    description: '音视图文参考，高性价比',
    tag: '',
    sort_order: 3,
  },
  {
    model_name: 'doubao-seedance-2-0-fast-260128',
    display_name: 'Doubao-Seedance-2.0-fast',
    description: '音视图文参考，更快，继承2.0核心优势',
    tag: '',
    sort_order: 4,
  },
];

const L = {
  zh: {
    empty: '暂无对外开放的模型',
  },
  en: {
    empty: 'No publicly available models',
  },
} as const;

export function LiveModels({ lang = 'zh' }: { lang?: 'zh' | 'en' }) {
  const t = L[lang];
  const models = STATIC_MODELS;
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
