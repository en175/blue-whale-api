import { generateLLMsFullText } from '@/lib/llms';
import { i18n } from '@/lib/i18n';

export const revalidate = false;

export async function GET() {
  return new Response(await generateLLMsFullText(i18n.defaultLanguage), {
    headers: {
      'Content-Type': 'text/plain; charset=utf-8',
    },
  });
}
