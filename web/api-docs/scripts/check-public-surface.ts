import { existsSync, readdirSync, readFileSync, statSync } from 'node:fs'
import { join } from 'node:path'

const forbiddenTerms = [
  { label: 'GPT-4', pattern: /gpt-4/i },
  { label: 'Claude', pattern: /claude/i },
  { label: 'Gemini', pattern: /gemini/i },
  { label: '充值', pattern: /充值/ },
  { label: '免费额度', pattern: /免费额度/ },
  { label: 'Chat Completions', pattern: /\/v1\/chat\/completions/i },
  { label: 'Embeddings', pattern: /\/v1\/embeddings/i },
  { label: 'Images API', pattern: /\/v1\/images/i },
  { label: 'Audio API', pattern: /\/v1\/audio/i },
  { label: 'Management API', pattern: /management\s*api/i }
]

const allowedApiPaths = new Set([
  '/v1/video/generations',
  '/v1/video/generations/{task_id}',
  '/v1/videos/{task_id}/content'
])

// 将示例里的具体 task_id（如 task_9f3a2b7c）归一化为 {task_id}，以便与白名单模板匹配
// 用负向前瞻排除 {task_id} 占位符本身（避免把 task_id 误替换成 {{task_id}}）
function normalizeApiPath(path: string): string {
  return path.replace(/task_(?!id)[A-Za-z0-9]+/g, '{task_id}')
}

function collectFiles(path: string): string[] {
  if (!existsSync(path)) return []
  if (statSync(path).isFile()) return [path]
  return readdirSync(path, { withFileTypes: true }).flatMap((entry) => {
    const child = join(path, entry.name)
    return entry.isDirectory() ? collectFiles(child) : [child]
  })
}

function scanDirectory(root: string): string[] {
  const files = collectFiles(root).filter((f) => /\.(mdx?|json)$/.test(f))
  return files.flatMap((file) => {
    const content = readFileSync(file, 'utf8')
    const found: string[] = forbiddenTerms
      .filter((term) => term.pattern.test(content))
      .map((term) => `${file}: ${term.label}`)
    const apiPaths = content.match(/\/v\d+(?:\/[A-Za-z0-9_.{}-]+)+/g) ?? []
    for (const path of apiPaths) {
      if (!allowedApiPaths.has(normalizeApiPath(path))) found.push(`${file}: 非公开 API 路径: ${path}`)
    }
    return found
  })
}

function main() {
  // 只扫描对外公开的新增章节（index/quickstart/authentication/models/video/errors/rate-limits/faq）
  // 被导航隐藏的旧章节（api/apps/en）不参与公开面检查
  const zhRoot = join(process.cwd(), 'content', 'docs', 'zh')
  const publicEntries = [
    'index.mdx',
    'quickstart.mdx',
    'authentication.mdx',
    'models.mdx',
    'video',
    'errors.mdx',
    'rate-limits.mdx',
    'faq.mdx'
  ]

  const matches: string[] = []
  for (const entry of publicEntries) {
    const target = join(zhRoot, entry)
    if (!existsSync(target)) continue
    matches.push(...scanDirectory(target))
  }

  if (matches.length > 0) {
    console.error('公开面包含禁止内容:\n' + matches.join('\n'))
    process.exit(1)
  }
  console.log('公开面检查通过：' + publicEntries.length + ' 个入口')
}

main()
