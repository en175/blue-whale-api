import {
  mkdirSync, readdirSync, readFileSync, writeFileSync, unlinkSync, existsSync,
} from 'node:fs';
import path from 'node:path';
import sharp from 'sharp';
import { extractReferencedAssetPaths, verifyAssetRefs } from './lib/asset-references';

const IMAGE_EXT = new Set(['.png', '.jpg', '.jpeg']);
const QUALITY = 80;
const DRY_RUN = process.argv.includes('--dry-run');

interface ConvertItem { old: string; target: string; abs: string; targetAbs: string; exists: boolean }

function collect(): ConvertItem[] {
  const refs = new Set([
    ...extractReferencedAssetPaths(path.join(process.cwd(), 'content')),
    ...extractReferencedAssetPaths(path.join(process.cwd(), 'src')),
  ]);
  const publicRoot = path.join(process.cwd(), 'public');
  const out: ConvertItem[] = [];
  for (const ref of refs) {
    const ext = path.extname(ref).toLowerCase();
    if (!IMAGE_EXT.has(ext)) continue;
    const abs = path.join(publicRoot, ref.slice(1));
    if (!existsSync(abs)) continue;
    const target = ref.slice(0, -ext.length) + '.webp';
    const targetAbs = path.join(publicRoot, target.slice(1));
    out.push({ old: ref, target, abs, targetAbs, exists: existsSync(targetAbs) });
  }
  return out;
}

async function main(): Promise<void> {
  const items = collect();
  console.log(`Found ${items.length} referenced PNG/JPG.`);

  const mapping = new Map<string, string>();
  for (const it of items) {
    if (!it.exists) {
      console.log(`convert  ${it.old} -> ${it.target}`);
      if (!DRY_RUN) {
        mkdirSync(path.dirname(it.targetAbs), { recursive: true });
        await sharp(it.abs).webp({ quality: QUALITY }).toFile(it.targetAbs);
        unlinkSync(it.abs);
      }
    } else {
      console.log(`reuse   ${it.old} -> existing ${it.target}`);
      // 目标 webp 已存在：只改引用，同时删掉原图，避免泄漏孤儿
      if (!DRY_RUN) unlinkSync(it.abs);
    }
    mapping.set(it.old, it.target);
  }
  if (DRY_RUN) { console.log('DRY RUN — nothing changed.'); return; }

  let rewritten = 0;
  for (const root of [path.join(process.cwd(), 'content'), path.join(process.cwd(), 'src')]) {
    const walk = (dir: string): void => {
      for (const entry of readdirSync(dir, { withFileTypes: true })) {
        const p = path.join(dir, entry.name);
        if (entry.isDirectory()) { walk(p); continue; }
        if (!/\.(mdx?|tsx?|ts)$/i.test(entry.name)) continue;
        let content = readFileSync(p, 'utf8');
        let changed = false;
        for (const [oldRef, newRef] of mapping) {
          if (content.includes(oldRef)) { content = content.split(oldRef).join(newRef); changed = true; }
        }
        if (changed) { writeFileSync(p, content); rewritten++; }
      }
    };
    walk(root);
  }
  console.log(`Rewrote ${rewritten} files.`);

  const allRefs = [
    ...extractReferencedAssetPaths(path.join(process.cwd(), 'content')),
    ...extractReferencedAssetPaths(path.join(process.cwd(), 'src')),
  ];
  const missing = verifyAssetRefs(allRefs, path.join(process.cwd(), 'public'));
  if (missing.length) { console.error('MISSING REFS:', missing); process.exit(1); }
  console.log('Verify OK — all asset refs resolve.');
}

main().catch((e) => { console.error(e); process.exit(1); });
