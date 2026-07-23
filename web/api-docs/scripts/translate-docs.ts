/**
 * Documentation Translation Script
 * Automatically translates Chinese markdown documents to English and Japanese using OpenAI API
 */

import * as fs from 'fs';
import * as path from 'path';
import { execSync } from 'child_process';

// ============================================================================
// Configuration
// ============================================================================

const DOCS_DIR = path.join(process.cwd(), 'content', 'docs');
const OPENAI_API_KEY = process.env.OPENAI_API_KEY || '';
const OPENAI_BASE_URL =
  process.env.OPENAI_BASE_URL || 'https://api.openai.com/v1';
const OPENAI_MODEL = process.env.OPENAI_MODEL || 'gemini-2.5-flash';
const MAX_RETRIES = parseInt(process.env.MAX_RETRIES || '3', 10);
const RETRY_DELAY = parseInt(process.env.RETRY_DELAY || '2', 10);
const RETRY_BACKOFF = parseFloat(process.env.RETRY_BACKOFF || '2.0');
const MAX_WORKERS = parseInt(process.env.MAX_WORKERS || '3', 10);
const FORCE_TRANSLATE = process.env.FORCE_TRANSLATE?.toLowerCase() === 'true';
const INCREMENTAL_TRANSLATE =
  process.env.INCREMENTAL_TRANSLATE?.toLowerCase() !== 'false';

const LANGUAGES = {
  en: { name: 'English', nativeName: '英文', dir: 'en' },
  ja: { name: 'Japanese', nativeName: '日文', dir: 'ja' },
} as const;

const GLOSSARY = `
| 中文 | English | 说明 | Description |
|------|---------|------|-------------|
| 倍率 | Ratio | 用于计算价格的乘数因子 | Multiplier factor used for price calculation |
| 令牌 | Token | API访问凭证，也指模型处理的文本单元 | API access credentials or text units processed by models |
| 渠道 | Channel | API服务提供商的接入通道 | Access channel for API service providers |
| 分组 | Group | 用户或令牌的分类，影响价格倍率 | Classification of users or tokens, affecting price ratios |
| 额度 | Quota | 用户可用的服务额度 | Available service quota for users |
`;

// ============================================================================
// Type Definitions
// ============================================================================

type LanguageCode = keyof typeof LANGUAGES;

interface OpenAIResponse {
  choices: Array<{ message: { content: string } }>;
}

interface TranslationStats {
  total: number;
  translated: number;
  skipped: number;
  failed: number;
}

interface FileChange {
  oldContent: string;
  newContent: string;
  hasChanges: boolean;
}

interface TranslationResult {
  translated: number;
  skipped: number;
  failed: number;
}

// ============================================================================
// Validation (deferred)
// ============================================================================
// NOTE: Do not exit at import time. This file is also imported by tooling/scripts.
function assertOpenAIKey() {
  if (!OPENAI_API_KEY) {
    console.error('❌ Error: OPENAI_API_KEY environment variable is not set');
    process.exit(1);
  }
}

// ============================================================================
// Translation Prompt Generation
// ============================================================================

const BASE_TRANSLATION_RULES = `
翻译要求：
1. 保持 Markdown 格式完整，包括标题、列表、代码块、链接等
2. 代码块内容不要翻译
3. 专业术语使用行业标准翻译
4. 保持技术准确性和专业性
5. 图片路径、链接路径保持不变（如果路径中包含中文目录，保持原样）
6. Front matter (YAML 头部) 中的内容需要翻译
7. 保持原文的语气和风格
8. 对于特殊的专有名词（如产品名 "New API"、"Cherry Studio" 等），保持不变
9. 如果文本中出现占位符（例如 @@FD_PROTECT_0@@），必须原样保留，不得翻译、改写、换行或添加空格`;

// ============================================================================
// MDX Protection (Prevent AI from translating API/OpenAPI paths)
// ============================================================================

const PROTECT_PREFIX = '@@FD_PROTECT_';
const PROTECT_SUFFIX = '@@';

function protectMdxSegments(input: string): {
  protectedText: string;
  restore: (translated: string) => string;
  tokens: string[];
} {
  const replacements: Array<{ token: string; value: string }> = [];
  let protectedText = input;

  function protectByRegex(re: RegExp) {
    protectedText = protectedText.replace(re, (match) => {
      const token = `${PROTECT_PREFIX}${replacements.length}${PROTECT_SUFFIX}`;
      replacements.push({ token, value: match });
      return token;
    });
  }

  // Protect the entire <APIPage ... /> component (it often contains OpenAPI JSON paths).
  protectByRegex(/<APIPage\b[\s\S]*?\/>/g);

  // Extra safety: protect any remaining OpenAPI generated JSON path outside APIPage.
  protectByRegex(/openapi\/generated\/[^\s"'<>)}\]]+?\.json/g);

  const tokens = replacements.map((r) => r.token);
  const restore = (translated: string) => {
    // Ensure model didn't drop/alter tokens. If it did, retry to avoid corrupting paths.
    for (const t of tokens) {
      if (!translated.includes(t)) {
        throw new Error(
          `Translation output is missing protected token: ${t}. Refusing to write corrupted content.`
        );
      }
    }
    let out = translated;
    for (const r of replacements) {
      out = out.split(r.token).join(r.value);
    }
    return out;
  };

  return { protectedText, restore, tokens };
}

function getPlainTextTranslationPrompt(
  targetLang: LanguageCode,
  text: string
): string {
  const { nativeName, dir } = LANGUAGES[targetLang];

  return `你是一个专业的技术文档翻译专家。请将以下中文短文本翻译为${nativeName}。
要求：
1. 只输出翻译后的纯文本，不要加引号，不要添加解释，不要换行（保持单行）。
2. 专业术语使用行业标准翻译；专有名词（如 "New API"）保持不变。
3. URL/路径保持不变；如果出现 /zh/ 路径前缀，请替换为 /${dir}/。

术语表（不要放在翻译内容中）：
${GLOSSARY}

原文：
${text}`;
}

function getTranslationPrompt(
  targetLang: LanguageCode,
  content: string
): string {
  const { nativeName, dir } = LANGUAGES[targetLang];

  return `你是一个专业的技术文档翻译专家。请将以下 Markdown 格式的技术文档从中文翻译为${nativeName}。
${BASE_TRANSLATION_RULES}
9. 路径中的语言代码需要替换：将 /zh/ 替换为 /${dir}/（例如：href="/zh/docs/guide" → href="/${dir}/docs/guide"）

术语表（不要放在翻译内容中）：
${GLOSSARY}

请直接返回翻译后的内容，不要添加任何解释或说明。
重要：不要在输出开头添加 \`\`\` 或 \`\`\`yaml 或 \`\`\`markdown 等代码块包裹符号，直接输出翻译后的原始内容即可。

原文：

${content}
`;
}

// ============================================================================
// OpenAI API Integration
// ============================================================================

async function callOpenAI(
  prompt: string,
  targetLang: LanguageCode
): Promise<string> {
  const response = await fetch(`${OPENAI_BASE_URL}/chat/completions`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      Authorization: `Bearer ${OPENAI_API_KEY}`,
    },
    body: JSON.stringify({
      model: OPENAI_MODEL,
      messages: [
        {
          role: 'system',
          content:
            'You are a professional technical documentation translator. Translate accurately while preserving Markdown formatting, code blocks, and technical terms.',
        },
        { role: 'user', content: prompt },
      ],
      temperature: 0.3,
    }),
  });

  if (!response.ok) {
    throw new Error(
      `API request failed: ${response.status} ${response.statusText}`
    );
  }

  const data = (await response.json()) as OpenAIResponse;
  return data.choices[0].message.content.trim();
}

async function translateContent(
  content: string,
  targetLang: LanguageCode
): Promise<string> {
  const { nativeName } = LANGUAGES[targetLang];
  let retryCount = 0;
  let lastError: Error | null = null;

  while (retryCount <= MAX_RETRIES) {
    try {
      if (retryCount > 0) {
        console.log(
          `   ⟳ Retry ${retryCount}/${MAX_RETRIES} for ${nativeName}...`
        );
      }

      // Protect critical MDX segments (e.g. <APIPage document={...} />) so AI won't translate paths.
      const { protectedText, restore } = protectMdxSegments(content);

      const prompt = getTranslationPrompt(targetLang, protectedText);
      const translated = await callOpenAI(prompt, targetLang);
      return restore(translated);
    } catch (error) {
      lastError = error as Error;
      retryCount++;

      if (retryCount <= MAX_RETRIES) {
        const delay = RETRY_DELAY * Math.pow(RETRY_BACKOFF, retryCount - 1);
        console.log(
          `   ⚠ Translation failed: ${lastError.message}, retrying in ${delay.toFixed(1)}s...`
        );
        await new Promise((resolve) => setTimeout(resolve, delay * 1000));
      } else {
        console.error(
          `   ✗ Translation failed after ${MAX_RETRIES} retries: ${lastError.message}`
        );
        throw lastError;
      }
    }
  }

  throw lastError!;
}

async function translatePlainText(
  text: string,
  targetLang: LanguageCode
): Promise<string> {
  let retryCount = 0;
  let lastError: Error | null = null;

  while (retryCount <= MAX_RETRIES) {
    try {
      const prompt = getPlainTextTranslationPrompt(targetLang, text);
      return await callOpenAI(prompt, targetLang);
    } catch (error) {
      lastError = error as Error;
      retryCount++;
      if (retryCount <= MAX_RETRIES) {
        const delay = RETRY_DELAY * Math.pow(RETRY_BACKOFF, retryCount - 1);
        await new Promise((resolve) => setTimeout(resolve, delay * 1000));
      } else {
        throw lastError;
      }
    }
  }

  throw lastError!;
}

// ============================================================================
// Git Integration
// ============================================================================

function detectManualTranslations(): Set<string> {
  const manualTranslations = new Set<string>();

  try {
    const output = execSync('git diff --name-only HEAD~1 HEAD', {
      cwd: process.cwd(),
      encoding: 'utf-8',
    });

    const changedFiles = output.trim().split('\n').filter(Boolean);
    const languageDirs = Object.values(LANGUAGES).map((l) => `/${l.dir}/`);

    for (const filePath of changedFiles) {
      if (languageDirs.some((dir) => filePath.includes(dir))) {
        manualTranslations.add(path.join(process.cwd(), filePath));
      }
    }

    if (manualTranslations.size > 0) {
      console.log(
        `\n📝 Detected ${manualTranslations.size} manually translated file(s)`
      );
    }
  } catch (error) {
    console.log('ℹ Could not detect manual translations (not in git repo?)');
  }

  return manualTranslations;
}

function getFileChanges(filePath: string): FileChange {
  try {
    const relativePath = path.relative(process.cwd(), filePath);
    const oldContent = execSync(`git show HEAD:${relativePath}`, {
      cwd: process.cwd(),
      encoding: 'utf-8',
    });

    const newContent = fs.readFileSync(filePath, 'utf-8');

    return {
      oldContent,
      newContent,
      hasChanges: oldContent !== newContent,
    };
  } catch (error) {
    // File is new or not in git
    return {
      oldContent: '',
      newContent: fs.readFileSync(filePath, 'utf-8'),
      hasChanges: true,
    };
  }
}

// ============================================================================
// File System Operations
// ============================================================================

function ensureDirectoryExists(filePath: string): void {
  const dir = path.dirname(filePath);
  if (!fs.existsSync(dir)) {
    fs.mkdirSync(dir, { recursive: true });
  }
}

function collectMarkdownFiles(dir: string): string[] {
  const files: string[] = [];

  function walkDir(currentDir: string) {
    const entries = fs.readdirSync(currentDir, { withFileTypes: true });

    for (const entry of entries) {
      const fullPath = path.join(currentDir, entry.name);

      if (entry.isDirectory()) {
        walkDir(fullPath);
      } else if (entry.isFile()) {
        if (/\.(md|mdx)$/i.test(entry.name)) {
          files.push(fullPath);
        } else if (entry.name === 'meta.json') {
          files.push(fullPath);
        }
      }
    }
  }

  walkDir(dir);
  return files;
}

// ============================================================================
// Translation Logic
// ============================================================================

async function translateToLanguage(
  content: string,
  langCode: string,
  langInfo: (typeof LANGUAGES)[LanguageCode],
  targetFile: string,
  prefix: string,
  isIncremental: boolean
): Promise<boolean> {
  const translationType = isIncremental ? 'Incremental' : 'Full';
  const emoji = isIncremental ? '🔄' : '🌐';

  console.log(
    `${prefix} ${emoji} ${translationType} translation to ${langInfo.nativeName}...`
  );

  try {
    const translatedContent = await translateContent(
      content,
      langCode as LanguageCode
    );

    ensureDirectoryExists(targetFile);
    fs.writeFileSync(targetFile, translatedContent, 'utf-8');

    const status = isIncremental ? 'Updated' : 'Saved';
    console.log(`${prefix} ✓ ${status} ${langInfo.nativeName} translation`);
    return true;
  } catch (error) {
    console.error(
      `${prefix} ✗ Failed to translate ${langInfo.nativeName}: ${(error as Error).message}`
    );
    return false;
  }
}

async function translateFile(
  sourceFile: string,
  fileIndex: number,
  totalFiles: number,
  manualTranslations: Set<string>
): Promise<TranslationResult> {
  const prefix = `[${fileIndex}/${totalFiles}]`;
  const result: TranslationResult = { translated: 0, skipped: 0, failed: 0 };

  console.log(
    `\n${prefix} 📄 Processing: ${path.relative(process.cwd(), sourceFile)}`
  );

  const isMetaJson = path.basename(sourceFile) === 'meta.json';

  // Read source file
  let content: string;
  try {
    content = fs.readFileSync(sourceFile, 'utf-8');
  } catch (error) {
    console.error(
      `${prefix} ✗ Failed to read file: ${(error as Error).message}`
    );
    result.failed = Object.keys(LANGUAGES).length;
    return result;
  }

  // Calculate relative path
  const zhDir = path.join(DOCS_DIR, 'zh');
  const relPath = path.relative(zhDir, sourceFile);

  if (relPath.startsWith('..')) {
    console.error(`${prefix} ✗ File is not in zh directory`);
    result.failed = Object.keys(LANGUAGES).length;
    return result;
  }

  // Check for file changes (incremental translation)
  let hasChanges = true;
  if (INCREMENTAL_TRANSLATE && !FORCE_TRANSLATE) {
    const fileChange = getFileChanges(sourceFile);
    hasChanges = fileChange.hasChanges;
  }

  // Check if any target translation is missing
  const missingTranslations: string[] = [];
  for (const [langCode, langInfo] of Object.entries(LANGUAGES)) {
    const targetFile = path.join(DOCS_DIR, langInfo.dir, relPath);
    if (!fs.existsSync(targetFile)) {
      missingTranslations.push(langInfo.nativeName);
    }
  }

  // Skip only if no changes AND all translations exist
  if (
    INCREMENTAL_TRANSLATE &&
    !hasChanges &&
    missingTranslations.length === 0 &&
    !FORCE_TRANSLATE
  ) {
    console.log(
      `${prefix} ⏭  No changes and all translations exist, skipping...`
    );
    result.skipped = Object.keys(LANGUAGES).length;
    return result;
  }

  // Log if we're filling missing translations
  if (missingTranslations.length > 0 && !hasChanges) {
    console.log(
      `${prefix} 📝 Filling missing translations: ${missingTranslations.join(', ')}`
    );
  }

  // Translate to each target language
  for (const [langCode, langInfo] of Object.entries(LANGUAGES)) {
    const targetFile = path.join(DOCS_DIR, langInfo.dir, relPath);

    // Check manual translation
    if (manualTranslations.has(targetFile)) {
      console.log(
        `${prefix} ⏭  Skipping ${langInfo.nativeName} (manual translation detected)`
      );
      result.skipped++;
      continue;
    }

    const targetExists = fs.existsSync(targetFile);

    // Skip if exists and not forcing
    if (targetExists && !FORCE_TRANSLATE && !INCREMENTAL_TRANSLATE) {
      console.log(
        `${prefix} ⏭  Skipping ${langInfo.nativeName} (already exists)`
      );
      result.skipped++;
      continue;
    }

    // Determine translation type
    const isIncremental = INCREMENTAL_TRANSLATE && targetExists && hasChanges;
    const shouldTranslate =
      FORCE_TRANSLATE || !targetExists || (INCREMENTAL_TRANSLATE && hasChanges);

    if (!shouldTranslate) {
      result.skipped++;
      continue;
    }

    if (FORCE_TRANSLATE && targetExists && !isIncremental) {
      console.log(
        `${prefix} 🔄 Force re-translating ${langInfo.nativeName}...`
      );
    }

    let success: boolean;

    if (isMetaJson) {
      // Safe JSON translation: only translate values, keep JSON structure untouched.
      try {
        const meta = JSON.parse(content) as Record<string, unknown>;
        const out: Record<string, unknown> = { ...meta };

        for (const key of ['title', 'description']) {
          const v = out[key];
          if (typeof v === 'string' && v.trim().length > 0) {
            out[key] = await translatePlainText(v, langCode as LanguageCode);
          }
        }

        ensureDirectoryExists(targetFile);
        fs.writeFileSync(targetFile, JSON.stringify(out, null, 2), 'utf-8');
        console.log(`${prefix} ✓ Saved ${langInfo.nativeName} meta.json`);
        success = true;
      } catch (error) {
        console.error(
          `${prefix} ✗ Failed to translate meta.json (${langInfo.nativeName}): ${(error as Error).message}`
        );
        success = false;
      }
    } else {
      // Perform markdown translation
      success = await translateToLanguage(
        content,
        langCode,
        langInfo,
        targetFile,
        prefix,
        isIncremental
      );
    }

    if (success) {
      result.translated++;
    } else {
      result.failed++;
    }
  }

  return result;
}

// ============================================================================
// Concurrent Processing
// ============================================================================

async function processFiles(
  files: string[],
  manualTranslations: Set<string>
): Promise<TranslationStats> {
  const stats: TranslationStats = {
    total: files.length,
    translated: 0,
    skipped: 0,
    failed: 0,
  };

  if (MAX_WORKERS === 1) {
    console.log('\n🔄 Using sequential mode\n');

    for (let i = 0; i < files.length; i++) {
      const result = await translateFile(
        files[i],
        i + 1,
        files.length,
        manualTranslations
      );
      stats.translated += result.translated;
      stats.skipped += result.skipped;
      stats.failed += result.failed;
    }
  } else {
    console.log(`\n🚀 Using concurrent mode (${MAX_WORKERS} workers)\n`);

    const chunks: string[][] = [];
    for (let i = 0; i < files.length; i += MAX_WORKERS) {
      chunks.push(files.slice(i, i + MAX_WORKERS));
    }

    let processedCount = 0;

    for (const chunk of chunks) {
      const promises = chunk.map((file, idx) =>
        translateFile(
          file,
          processedCount + idx + 1,
          files.length,
          manualTranslations
        )
      );

      const results = await Promise.all(promises);

      for (const result of results) {
        stats.translated += result.translated;
        stats.skipped += result.skipped;
        stats.failed += result.failed;
      }

      processedCount += chunk.length;
    }
  }

  return stats;
}

// ============================================================================
// Main Function
// ============================================================================

async function translateDocs(specificPaths?: string[]) {
  assertOpenAIKey();
  console.log('═══════════════════════════════════════════════');
  console.log('🌐 Starting document translation...');
  console.log('═══════════════════════════════════════════════\n');

  const startTime = Date.now();
  const manualTranslations = detectManualTranslations();

  // Determine files to translate
  let filesToTranslate: string[];

  if (specificPaths && specificPaths.length > 0) {
    filesToTranslate = [];
    const languageDirs = Object.values(LANGUAGES).map((l) => `/${l.dir}/`);

    for (const inputPath of specificPaths) {
      const resolvedPath = path.resolve(inputPath);

      // Check if it's a translated directory (en/ja)
      if (languageDirs.some((dir) => resolvedPath.includes(dir))) {
        console.log(`⏭  Skipping translated path: ${resolvedPath}`);
        continue;
      }

      if (!fs.existsSync(resolvedPath)) {
        console.warn(`⚠ Path not found: ${resolvedPath}`);
        continue;
      }

      const stat = fs.statSync(resolvedPath);

      if (stat.isDirectory()) {
        // It's a directory - collect all markdown files
        console.log(`📁 Collecting files from directory: ${inputPath}`);
        const dirFiles = collectMarkdownFiles(resolvedPath);
        console.log(`   Found ${dirFiles.length} markdown file(s)`);
        filesToTranslate.push(...dirFiles);
      } else if (stat.isFile()) {
        // It's a file
        if (
          !/\.(md|mdx)$/i.test(resolvedPath) &&
          !/meta\.json$/i.test(resolvedPath)
        ) {
          console.warn(`⚠ Not a translatable file: ${resolvedPath}`);
          continue;
        }
        filesToTranslate.push(resolvedPath);
      }
    }
  } else {
    const zhDir = path.join(DOCS_DIR, 'zh');
    filesToTranslate = collectMarkdownFiles(zhDir);
  }

  if (filesToTranslate.length === 0) {
    console.log('ℹ No files to translate');
    return;
  }

  // Display configuration
  console.log(`\n📋 Configuration:`);
  console.log(`   Files: ${filesToTranslate.length}`);
  console.log(`   Model: ${OPENAI_MODEL}`);
  console.log(`   API: ${OPENAI_BASE_URL}`);
  console.log(
    `   Languages: ${Object.values(LANGUAGES)
      .map((l) => l.nativeName)
      .join(', ')}`
  );
  console.log(
    `   Retry: Max ${MAX_RETRIES} times, delay ${RETRY_DELAY}s, backoff ${RETRY_BACKOFF}x`
  );
  console.log(`   Concurrency: ${MAX_WORKERS} worker(s)`);
  console.log(
    `   Incremental translate: ${INCREMENTAL_TRANSLATE ? 'Yes' : 'No'}`
  );
  console.log(`   Force translate: ${FORCE_TRANSLATE ? 'Yes' : 'No'}`);
  console.log(`   Manual translations: ${manualTranslations.size}`);

  // Process files
  const stats = await processFiles(filesToTranslate, manualTranslations);

  const duration = ((Date.now() - startTime) / 1000).toFixed(2);

  // Display results
  console.log('\n═══════════════════════════════════════════════');
  console.log('📊 Translation Statistics:');
  console.log(`   Total files: ${stats.total}`);
  console.log(`   Translations: ${stats.translated}`);
  console.log(`   Skipped: ${stats.skipped}`);
  if (stats.failed > 0) {
    console.log(`   Failed: ${stats.failed}`);
  }
  console.log(`   Duration: ${duration}s`);
  console.log('═══════════════════════════════════════════════');
  console.log('✅ Translation completed!\n');
}

// ============================================================================
// Entry Point
// ============================================================================

if (require.main === module) {
  const args = process.argv.slice(2);
  translateDocs(args.length > 0 ? args : undefined).catch((error) => {
    console.error('\n❌ Translation failed:', error);
    process.exit(1);
  });
}

export { translateDocs };
// Export for testing/tooling (e.g. to verify OpenAPI path protection).
export { protectMdxSegments };
