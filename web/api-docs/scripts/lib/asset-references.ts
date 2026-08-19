import { readdirSync, readFileSync, existsSync } from 'node:fs';
import path from 'node:path';

// 匹配 /assets/<path>；负向后行断言排除 URL 前缀（如 github.com/user-attachments/assets/）；
// 路径字符类排除空白/引号/括号/尖括号，以兼容含非 ASCII 字符（如 em-dash）的文件名
const ASSET_REF_RE = /(?<![\w/])\/assets\/[^\s"'()<>]+/g;

export function extractAssetRefsFromText(text: string): string[] {
  return Array.from(text.matchAll(ASSET_REF_RE), (m) => m[0]);
}

export function extractReferencedAssetPaths(root: string): string[] {
  const refs = new Set<string>();
  const walk = (dir: string): void => {
    for (const entry of readdirSync(dir, { withFileTypes: true })) {
      const p = path.join(dir, entry.name);
      if (entry.isDirectory()) walk(p);
      else if (/\.(mdx?|tsx?|ts)$/i.test(entry.name)) {
        for (const ref of extractAssetRefsFromText(readFileSync(p, 'utf8'))) refs.add(ref);
      }
    }
  };
  walk(root);
  return [...refs];
}

export function verifyAssetRefs(referenced: string[], publicRoot: string): string[] {
  const missing: string[] = [];
  for (const ref of referenced) {
    if (!existsSync(path.join(publicRoot, ref.replace(/^\//, '')))) missing.push(ref);
  }
  return missing;
}
