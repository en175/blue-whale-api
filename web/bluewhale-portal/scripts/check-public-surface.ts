import { existsSync, readdirSync, readFileSync } from 'node:fs'
import { join } from 'node:path'

import { publicVideoScope } from '../src/lib/public-video-scope'

const forbiddenTerms = [
  { label: 'Webhook', pattern: /webhook/i },
  { label: 'callback', pattern: /callback/i },
  { label: 'idempotency', pattern: /idempotency/i },
  { label: 'Authorization', pattern: /authorization/i },
  { label: 'Bearer Token', pattern: /bearer token/i },
  { label: 'GPT-4', pattern: /gpt-4/i },
  { label: 'Claude', pattern: /claude/i },
  { label: 'Gemini', pattern: /gemini/i },
  { label: '管理 API', pattern: /管理\s*api/i },
  { label: 'Management API', pattern: /management\s*api/i },
  { label: '充值', pattern: /充值/i },
  { label: '免费额度', pattern: /免费额度/i },
  { label: '115.190.244.91', pattern: /115\.190\.244\.91/ }
]

const publicApiPaths: ReadonlySet<string> = new Set(
  publicVideoScope.operations.map((operation) => operation.path)
)

export function findForbiddenPublicContent(content: string): string[] {
  const matches = forbiddenTerms
    .filter((term) => term.pattern.test(content))
    .map((term) => term.label)

  const apiPaths = content.match(/\/v\d+(?:\/[A-Za-z0-9_.{}-]+)+/g) ?? []
  for (const path of apiPaths) {
    if (!publicApiPaths.has(path)) {
      matches.push('非公开 API 路径: ' + path)
    }
  }

  return [...new Set(matches)]
}

function collectFiles(directory: string): string[] {
  if (!existsSync(directory)) {
    return []
  }

  return readdirSync(directory, { withFileTypes: true }).flatMap((entry) => {
    const path = join(directory, entry.name)
    return entry.isDirectory() ? collectFiles(path) : [path]
  })
}

function main() {
  const files = ['src', 'content', 'public', '.next/server/app'].flatMap((directory) =>
    collectFiles(join(process.cwd(), directory))
  )
  const matches = files.flatMap((file) =>
    findForbiddenPublicContent(readFileSync(file, 'utf8')).map((term) => file + ': ' + term)
  )

  if (matches.length > 0) {
    throw new Error('公开面包含禁止内容:\\n' + matches.join('\\n'))
  }

  console.log('公开面检查通过：' + files.length + ' 个文件')
}

if (process.argv[1]?.endsWith('check-public-surface.ts')) {
  main()
}
