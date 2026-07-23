/**
 * Repair translated OpenAPI MDX files.
 *
 * Problem:
 * - Some translated MDX files (en/ja) have `<APIPage document={"openapi/generated/..."} />`
 *   paths that no longer match the actual JSON file location under `openapi/generated/**`.
 * - This causes prerender failures like: "Cannot destructure property 'dereferenced' ... undefined".
 *
 * This script:
 * - Builds an index of all JSON files under `openapi/generated/` (recursive) by basename.
 * - Scans `content/docs/{en,ja}/api/` (recursive) MDX files for `document={"...json"}` paths.
 * - If a referenced JSON doesn't exist, it rewrites it to the unique matching JSON path by basename.
 */

import { readdir, readFile, stat, writeFile } from 'node:fs/promises';
import path from 'node:path';

type Locale = 'en' | 'ja';

function toPosix(p: string): string {
  return p.split(path.sep).join('/');
}

async function walkFiles(
  root: string,
  predicate: (filePath: string) => boolean
): Promise<string[]> {
  const out: string[] = [];
  async function walk(dir: string) {
    let entries: Array<{ name: string; isDirectory: boolean; isFile: boolean }>;
    try {
      entries = (await readdir(dir, { withFileTypes: true })) as any;
    } catch {
      return;
    }
    for (const e of entries as any) {
      const full = path.join(dir, e.name);
      if (e.isDirectory()) await walk(full);
      else if (e.isFile() && predicate(full)) out.push(full);
    }
  }
  await walk(root);
  return out;
}

async function buildOpenApiIndex(): Promise<Map<string, string[]>> {
  const root = path.join(process.cwd(), 'openapi', 'generated');
  const files = await walkFiles(root, (p) => p.toLowerCase().endsWith('.json'));
  const index = new Map<string, string[]>();
  for (const abs of files) {
    const base = path.basename(abs);
    const rel = toPosix(path.relative(process.cwd(), abs));
    const arr = index.get(base) ?? [];
    arr.push(rel);
    index.set(base, arr);
  }
  return index;
}

function extractDocumentPaths(mdx: string): string[] {
  const re = /document=\{"(openapi\/generated\/[^"]+?\.json)"\}/g;
  const out: string[] = [];
  for (let m; (m = re.exec(mdx)); ) {
    out.push(m[1]);
  }
  return out;
}

async function pathExists(relPosix: string): Promise<boolean> {
  const abs = path.join(process.cwd(), ...relPosix.split('/'));
  try {
    const s = await stat(abs);
    return s.isFile();
  } catch {
    return false;
  }
}

async function repairLocale(locale: Locale, index: Map<string, string[]>) {
  const root = path.join(process.cwd(), 'content', 'docs', locale, 'api');
  const mdxFiles = await walkFiles(root, (p) =>
    p.toLowerCase().endsWith('.mdx')
  );

  let changedFiles = 0;
  let fixedRefs = 0;
  let unresolvedRefs = 0;

  for (const absMdx of mdxFiles) {
    const original = await readFile(absMdx, 'utf8');
    const docPaths = extractDocumentPaths(original);
    if (docPaths.length === 0) continue;

    let next = original;
    let touched = false;

    for (const docPath of docPaths) {
      // If it already exists, nothing to do.
      // (Paths are stored as POSIX in MDX)
      // eslint-disable-next-line no-await-in-loop
      if (await pathExists(docPath)) continue;

      const base = path.basename(docPath);
      const candidates = index.get(base) ?? [];

      if (candidates.length === 1) {
        const replacement = candidates[0];
        next = next
          .split(`document={"${docPath}"}`)
          .join(`document={"${replacement}"}`);
        touched = true;
        fixedRefs++;
      } else {
        unresolvedRefs++;
      }
    }

    if (touched && next !== original) {
      await writeFile(absMdx, next, 'utf8');
      changedFiles++;
    }
  }

  return {
    locale,
    changedFiles,
    fixedRefs,
    unresolvedRefs,
    totalFiles: mdxFiles.length,
  };
}

async function main() {
  const index = await buildOpenApiIndex();
  const locales: Locale[] = ['en', 'ja'];
  const results = [];
  for (const locale of locales) {
    // eslint-disable-next-line no-await-in-loop
    results.push(await repairLocale(locale, index));
  }

  // Keep output compact but informative for CI/build logs.
  for (const r of results) {
    console.log(
      `[repair-openapi-mdx] ${r.locale}: scanned=${r.totalFiles}, changed=${r.changedFiles}, fixedRefs=${r.fixedRefs}, unresolvedRefs=${r.unresolvedRefs}`
    );
  }
}

main().catch((err) => {
  console.error('[repair-openapi-mdx] failed:', err);
  process.exit(1);
});
