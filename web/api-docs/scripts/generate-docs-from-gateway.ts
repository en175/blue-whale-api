import { mkdir, writeFile } from 'node:fs/promises';
import path from 'node:path';

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
type DocsConfig = { docs_domain: string; models: DocModel[] };

async function fetchDocsConfig(gatewayUrl: string): Promise<DocsConfig> {
  const res = await fetch(`${gatewayUrl.replace(/\/$/, '')}/api/docs/config`);
  if (!res.ok) throw new Error(`docs config fetch failed: ${res.status}`);
  const json = (await res.json()) as { data: DocsConfig };
  return json.data;
}

type Lang = 'zh' | 'en';

const L = {
  zh: {
    title: '模型列表',
    description: '当前开放的模型与参数一览',
    heading: '模型列表',
    fieldHeader: '| 字段 | 类型 | 必填 | 默认 | 说明 |',
    headerHeader: '| 名称 | 必填 | 说明 |',
    yes: '是',
    no: '否',
    params: '请求参数',
    headers: '请求头',
    auth: '鉴权',
    example: '调用示例',
    response: '响应示例',
  },
  en: {
    title: 'Models',
    description: 'Currently available models and parameters',
    heading: 'Models',
    fieldHeader: '| Field | Type | Required | Default | Description |',
    headerHeader: '| Name | Required | Description |',
    yes: 'Yes',
    no: 'No',
    params: 'Request Parameters',
    headers: 'Headers',
    auth: 'Authentication',
    example: 'Example',
    response: 'Response',
  },
} as const;

function renderFieldTable(fields: BodyField[], lang: Lang): string {
  if (!fields.length) return '';
  const t = L[lang];
  const rows = fields
    .map((f) => {
      const req = f.required ? t.yes : t.no;
      const def = f.default ?? '';
      const desc = f.description ?? '';
      return `| \`${f.name}\` | ${f.type} | ${req} | ${def} | ${desc} |`;
    })
    .join('\n');
  return `${t.fieldHeader}\n| --- | --- | --- | --- | --- |\n${rows}`;
}

function renderHeaderTable(headers: HeaderField[], lang: Lang): string {
  if (!headers.length) return '';
  const t = L[lang];
  const rows = headers
    .map((h) => {
      const req = h.required ? t.yes : t.no;
      const desc = h.description ?? '';
      return `| \`${h.name}\` | ${req} | ${desc} |`;
    })
    .join('\n');
  return `${t.headerHeader}\n| --- | --- | --- |\n${rows}`;
}

function renderModelSection(m: DocModel, lang: Lang): string {
  const t = L[lang];
  let schema: ParamSchema = {};
  try {
    schema = JSON.parse(m.param_schema || '{}') as ParamSchema;
  } catch {
    schema = {};
  }
  const lines: string[] = [];
  lines.push(`## ${m.display_name || m.model_name}`);
  if (m.description) lines.push(`\n${m.description}\n`);
  if (schema.body_fields?.length) {
    lines.push(`\n### ${t.params}\n`);
    lines.push(renderFieldTable(schema.body_fields, lang));
  }
  if (schema.headers?.length) {
    lines.push(`\n### ${t.headers}\n`);
    lines.push(renderHeaderTable(schema.headers, lang));
  }
  if (schema.auth?.description) {
    lines.push(`\n### ${t.auth}\n`);
    lines.push(schema.auth.description);
  }
  if (schema.curl_example) {
    lines.push(`\n### ${t.example}\n\n\`\`\`bash\n${schema.curl_example}\n\`\`\`\n`);
  }
  if (schema.response_example) {
    lines.push(`\n### ${t.response}\n\n\`\`\`json\n${schema.response_example}\n\`\`\`\n`);
  }
  return lines.join('\n');
}

function renderModelsPage(config: DocsConfig, lang: Lang): string {
  const t = L[lang];
  return `---\ntitle: ${t.title}\ndescription: ${t.description}\n---\n\n# ${t.heading}\n\n${config.models
    .map((m) => renderModelSection(m, lang))
    .join('\n\n')}\n`;
}

export async function generateDocsFromGateway(): Promise<void> {
  const gatewayUrl = process.env.GATEWAY_URL;
  if (!gatewayUrl) {
    console.warn('⚠ GATEWAY_URL not set; skipping docs generation (keep static content)');
    return;
  }
  try {
    const config = await fetchDocsConfig(gatewayUrl);

    const cwd = process.cwd();
    await mkdir(path.join(cwd, 'content/docs/zh'), { recursive: true });
    await mkdir(path.join(cwd, 'content/docs/en'), { recursive: true });
    await mkdir(path.join(cwd, 'generated'), { recursive: true });

    await writeFile(
      path.join(cwd, 'content/docs/zh/models.mdx'),
      renderModelsPage(config, 'zh'),
      'utf8',
    );
    await writeFile(
      path.join(cwd, 'content/docs/en/models.mdx'),
      renderModelsPage(config, 'en'),
      'utf8',
    );
    await writeFile(
      path.join(cwd, 'generated/site-config.json'),
      JSON.stringify({ docs_domain: config.docs_domain }, null, 2),
      'utf8',
    );
    console.log(
      `✅ Generated models.mdx (${config.models.length} models) zh+en + site-config.json`,
    );
  } catch (e) {
    console.error('❌ generate-docs-from-gateway failed:', e);
    console.log('⚠ build continues with static content');
  }
}

// Allow standalone execution (bun/tsx) for manual verification, without
// double-running when imported by prebuild.ts.
if ((import.meta as { main?: boolean }).main) {
  generateDocsFromGateway();
}
