import { mkdir, writeFile } from 'node:fs/promises';
import path from 'node:path';

type BodyField = {
  name: string;
  type: string;
  required: boolean;
  default?: string;
  description?: string;
};
type ParamSchema = {
  body_fields?: BodyField[];
  headers?: { name: string; required: boolean; description?: string }[];
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

function renderFieldTable(fields: BodyField[]): string {
  if (!fields.length) return '';
  const rows = fields
    .map((f) => {
      const req = f.required ? '是' : '否';
      const def = f.default ?? '';
      const desc = f.description ?? '';
      return `| \`${f.name}\` | ${f.type} | ${req} | ${def} | ${desc} |`;
    })
    .join('\n');
  return `| 字段 | 类型 | 必填 | 默认 | 说明 |\n| --- | --- | --- | --- | --- |\n${rows}`;
}

function renderModelSection(m: DocModel): string {
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
    lines.push(`\n### 请求参数\n`);
    lines.push(renderFieldTable(schema.body_fields));
  }
  if (schema.curl_example) {
    lines.push(`\n### 调用示例\n\n\`\`\`bash\n${schema.curl_example}\n\`\`\`\n`);
  }
  return lines.join('\n');
}

export async function generateDocsFromGateway(): Promise<void> {
  const gatewayUrl = process.env.GATEWAY_URL;
  if (!gatewayUrl) {
    console.warn('⚠ GATEWAY_URL not set; skipping docs generation (keep static content)');
    return;
  }
  try {
    const config = await fetchDocsConfig(gatewayUrl);
    const body = `---\ntitle: 模型列表\ndescription: 当前开放的模型与参数一览\n---\n\n# 模型列表\n\n${config.models
      .map(renderModelSection)
      .join('\n\n')}\n`;

    const cwd = process.cwd();
    await mkdir(path.join(cwd, 'content/docs/zh'), { recursive: true });
    await mkdir(path.join(cwd, 'generated'), { recursive: true });
    await writeFile(path.join(cwd, 'content/docs/zh/models.mdx'), body, 'utf8');
    await writeFile(
      path.join(cwd, 'generated/site-config.json'),
      JSON.stringify({ docs_domain: config.docs_domain }, null, 2),
      'utf8',
    );
    console.log(`✅ Generated models.mdx (${config.models.length} models) + site-config.json`);
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
