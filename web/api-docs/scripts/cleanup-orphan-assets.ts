import { readdirSync, statSync, unlinkSync, rmdirSync, existsSync } from 'node:fs';
import path from 'node:path';
import { extractReferencedAssetPaths, verifyAssetRefs } from './lib/asset-references';

const DRY_RUN = process.argv.includes('--dry-run');

function main(): void {
  const publicRoot = path.join(process.cwd(), 'public');
  const assetsRoot = path.join(publicRoot, 'assets');
  const referenced = new Set([
    ...extractReferencedAssetPaths(path.join(process.cwd(), 'content')),
    ...extractReferencedAssetPaths(path.join(process.cwd(), 'src')),
  ]);

  let deleted = 0, freedBytes = 0;
  const list: string[] = [];
  const walk = (dir: string): void => {
    for (const entry of readdirSync(dir, { withFileTypes: true })) {
      const p = path.join(dir, entry.name);
      if (entry.isDirectory()) { walk(p); continue; }
      const rel = '/' + path.relative(publicRoot, p).replace(/\\/g, '/');
      if (!referenced.has(rel)) {
        const size = statSync(p).size;
        if (!DRY_RUN) unlinkSync(p);
        deleted++; freedBytes += size; list.push(rel);
      }
    }
    if (!DRY_RUN && existsSync(dir) && dir !== assetsRoot) {
      try { if (readdirSync(dir).length === 0) rmdirSync(dir); } catch { /* keep */ }
    }
  };
  walk(assetsRoot);

  const remaining = ((): number => {
    let n = 0;
    const count = (d: string): void => {
      for (const e of readdirSync(d, { withFileTypes: true })) {
        const p = path.join(d, e.name);
        if (e.isDirectory()) count(p); else n++;
      }
    };
    count(assetsRoot);
    return n;
  })();

  console.log(`${DRY_RUN ? '[DRY RUN] ' : ''}Deleted ${deleted} orphans, freed ${(freedBytes / 1048576).toFixed(1)} MB, remaining ${remaining} files`);
  console.log(list.join('\n'));

  const allRefs = [
    ...extractReferencedAssetPaths(path.join(process.cwd(), 'content')),
    ...extractReferencedAssetPaths(path.join(process.cwd(), 'src')),
  ];
  const missing = verifyAssetRefs(allRefs, publicRoot);
  if (missing.length) { console.error('MISSING REFS:', missing); process.exit(1); }
  console.log('Verify OK — all asset refs resolve.');
}

main();
