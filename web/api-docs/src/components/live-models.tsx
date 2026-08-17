'use client';

import { useEffect, useState } from 'react';

type BodyField = {
  name: string;
  type: string;
  required: boolean;
  default?: string;
  description?: string;
};
type HeaderField = {
  name: string;
  required: boolean;
  description?: string;
};
type ParamSchema = {
  body_fields?: BodyField[];
  headers?: HeaderField[];
  auth?: { type?: string; description?: string };
  curl_example?: string;
  response_example?: string;
};
type DocModel = {
  model_name: string;
  display_name: string;
  description: string;
  param_schema: string;
  sort_order: number;
};

const L = {
  zh: {
    params: '请求参数',
    headers: '请求头',
    auth: '鉴权',
    example: '调用示例',
    response: '响应示例',
    empty: '暂无对外开放的模型',
    loading: '加载中…',
    error: '模型列表加载失败',
    field: '字段',
    type: '类型',
    required: '必填',
    default: '默认',
    desc: '说明',
    name: '名称',
    yes: '是',
    no: '否',
  },
  en: {
    params: 'Request Parameters',
    headers: 'Headers',
    auth: 'Authentication',
    example: 'Example',
    response: 'Response',
    empty: 'No publicly available models',
    loading: 'Loading…',
    error: 'Failed to load model list',
    field: 'Field',
    type: 'Type',
    required: 'Required',
    default: 'Default',
    desc: 'Description',
    name: 'Name',
    yes: 'Yes',
    no: 'No',
  },
} as const;

function parseSchema(raw: string): ParamSchema {
  try {
    return JSON.parse(raw || '{}') as ParamSchema;
  } catch {
    return {};
  }
}

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
    <div className="space-y-8">
      {models.map((m) => {
        const schema = parseSchema(m.param_schema);
        return (
          <div key={m.model_name}>
            <h2 className="text-lg font-semibold">
              {m.display_name || m.model_name}
            </h2>
            {m.description ? <p className="text-sm">{m.description}</p> : null}

            {schema.body_fields?.length ? (
              <div className="mt-3">
                <h3 className="font-medium">{t.params}</h3>
                <div className="mt-1 overflow-x-auto">
                  <table className="w-full border-collapse text-sm">
                    <thead>
                      <tr className="border-b">
                        <th className="px-2 py-1 text-left">{t.field}</th>
                        <th className="px-2 py-1 text-left">{t.type}</th>
                        <th className="px-2 py-1 text-left">{t.required}</th>
                        <th className="px-2 py-1 text-left">{t.default}</th>
                        <th className="px-2 py-1 text-left">{t.desc}</th>
                      </tr>
                    </thead>
                    <tbody>
                      {schema.body_fields.map((f) => (
                        <tr key={f.name} className="border-b">
                          <td className="px-2 py-1">
                            <code>{f.name}</code>
                          </td>
                          <td className="px-2 py-1">{f.type}</td>
                          <td className="px-2 py-1">{f.required ? t.yes : t.no}</td>
                          <td className="px-2 py-1">{f.default ?? ''}</td>
                          <td className="px-2 py-1">{f.description ?? ''}</td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              </div>
            ) : null}

            {schema.headers?.length ? (
              <div className="mt-3">
                <h3 className="font-medium">{t.headers}</h3>
                <div className="mt-1 overflow-x-auto">
                  <table className="w-full border-collapse text-sm">
                    <thead>
                      <tr className="border-b">
                        <th className="px-2 py-1 text-left">{t.name}</th>
                        <th className="px-2 py-1 text-left">{t.required}</th>
                        <th className="px-2 py-1 text-left">{t.desc}</th>
                      </tr>
                    </thead>
                    <tbody>
                      {schema.headers.map((h) => (
                        <tr key={h.name} className="border-b">
                          <td className="px-2 py-1">
                            <code>{h.name}</code>
                          </td>
                          <td className="px-2 py-1">{h.required ? t.yes : t.no}</td>
                          <td className="px-2 py-1">{h.description ?? ''}</td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              </div>
            ) : null}

            {schema.auth?.description ? (
              <div className="mt-3">
                <h3 className="font-medium">{t.auth}</h3>
                <p className="text-sm">{schema.auth.description}</p>
              </div>
            ) : null}

            {schema.curl_example ? (
              <div className="mt-3">
                <h3 className="font-medium">{t.example}</h3>
                <pre className="mt-1 overflow-x-auto rounded-md border p-3 text-xs">
                  <code>{schema.curl_example}</code>
                </pre>
              </div>
            ) : null}

            {schema.response_example ? (
              <div className="mt-3">
                <h3 className="font-medium">{t.response}</h3>
                <pre className="mt-1 overflow-x-auto rounded-md border p-3 text-xs">
                  <code>{schema.response_example}</code>
                </pre>
              </div>
            ) : null}
          </div>
        );
      })}
    </div>
  );
}
