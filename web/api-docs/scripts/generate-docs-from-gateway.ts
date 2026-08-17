import { mkdir, writeFile } from 'node:fs/promises';
import path from 'node:path';

type DocsConfig = { docs_domain: string };

async function fetchDocsConfig(gatewayUrl: string): Promise<DocsConfig> {
  const res = await fetch(`${gatewayUrl.replace(/\/$/, '')}/api/docs/config`);
  if (!res.ok) throw new Error(`docs config fetch failed: ${res.status}`);
  const json = (await res.json()) as { data: DocsConfig };
  return json.data;
}

// 模型列表已改为客户端实时拉取（LiveModels 组件），本脚本只负责把「文档域名」
// 写入 generated/site-config.json，供 metadata.ts 的 baseUrl 读取。
export async function generateDocsFromGateway(): Promise<void> {
  const gatewayUrl = process.env.GATEWAY_URL;
  if (!gatewayUrl) {
    console.warn('⚠ GATEWAY_URL not set; skipping site-config generation (keep static content)');
    return;
  }
  try {
    const config = await fetchDocsConfig(gatewayUrl);
    const cwd = process.cwd();
    await mkdir(path.join(cwd, 'generated'), { recursive: true });
    await writeFile(
      path.join(cwd, 'generated/site-config.json'),
      JSON.stringify({ docs_domain: config.docs_domain }, null, 2),
      'utf8',
    );
    console.log('✅ Generated site-config.json');
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
