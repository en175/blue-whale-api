import { existsSync, readdirSync, readFileSync, statSync } from 'node:fs'
import { dirname, join, relative } from 'node:path'

// i18n 对齐检查：zh 内容更新后跑一次，防止 en 落后
// 1) 文件树对齐：zh/en 下的 mdx/json 必须双向存在
// 2) 结构对齐：mdx 的标题数/卡片/代码块/图标/表格行/引用块逐项一致（不含译文文本差异）
// 3) 链接对齐：mdx 内相对链接目标必须真实存在（防断链）

const docsRoot = join(process.cwd(), 'content', 'docs')

function collectFiles(root: string): string[] {
  if (!existsSync(root)) return []
  if (statSync(root).isFile()) return [root]
  return readdirSync(root, { withFileTypes: true }).flatMap((entry) => {
    const child = join(root, entry.name)
    return entry.isDirectory() ? collectFiles(child) : [child]
  })
}

function rel(root: string, file: string): string {
  return relative(root, file).replace(/\\/g, '/')
}

interface Signature {
  headings: number
  cards: number
  code: number
  icons: number
  tables: number
  quotes: number
}

function signature(file: string): Signature {
  const t = readFileSync(file, 'utf8')
  return {
    headings: (t.match(/^#{1,6} /gm) ?? []).length,
    cards: (t.match(/<Card/g) ?? []).length,
    code: (t.match(/```/g) ?? []).length,
    icons: (t.match(/<\w+ ?\/>/g) ?? []).length,
    tables: (t.match(/^\|/gm) ?? []).length,
    quotes: (t.match(/^>/gm) ?? []).length
  }
}

// 校验 mdx 内相对链接（如 ./query、../models）目标真实存在
function brokenLinks(root: string, f: string): string[] {
  const t = readFileSync(join(root, f), 'utf8')
  const links = t.match(/\]\((\.{1,2}\/[^)#\s]+)(?:#[^)\s]*)?\)/g) ?? []
  const broken: string[] = []
  for (const raw of links) {
    const target = raw.slice(2, raw.lastIndexOf(')')) .split('#')[0]
    const resolved = join(root, dirname(f), target)
    const ok =
      existsSync(`${resolved}.mdx`) ||
      existsSync(join(resolved, 'index.mdx')) ||
      existsSync(resolved)
    if (!ok) broken.push(target)
  }
  return broken
}

function main() {
  const zhRoot = join(docsRoot, 'zh')
  const enRoot = join(docsRoot, 'en')

  const zhFiles = collectFiles(zhRoot)
    .map((f) => rel(zhRoot, f))
    .sort()
  const enFiles = collectFiles(enRoot)
    .map((f) => rel(enRoot, f))
    .sort()
  const enSet = new Set(enFiles)
  const zhSet = new Set(zhFiles)

  const problems: string[] = []
  for (const f of zhFiles) if (!enSet.has(f)) problems.push(`en 缺失: ${f}`)
  for (const f of enFiles) if (!zhSet.has(f)) problems.push(`en 多余: ${f}`)

  for (const f of zhFiles) {
    if (!f.endsWith('.mdx') || !enSet.has(f)) continue
    const a = signature(join(zhRoot, f))
    const b = signature(join(enRoot, f))
    if (JSON.stringify(a) !== JSON.stringify(b)) {
      problems.push(`结构漂移: ${f}\n  zh=${JSON.stringify(a)}\n  en=${JSON.stringify(b)}`)
    }
    for (const target of brokenLinks(zhRoot, f))
      problems.push(`断链(zh): ${f} -> ${target}`)
    for (const target of brokenLinks(enRoot, f))
      problems.push(`断链(en): ${f} -> ${target}`)
  }

  if (problems.length > 0) {
    console.error('i18n 对齐检查未通过:\n' + problems.join('\n'))
    process.exit(1)
  }
  console.log(`i18n 对齐检查通过：${zhFiles.length} 个文件，双向零差异`)
}

main()
