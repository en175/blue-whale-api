import { generateLLMsText } from '@/lib/llms';
import { i18n } from '@/lib/i18n';

export const revalidate = false;

export async function GET(req: Request) {
  const origin = new URL(req.url).origin;

  return new Response(generateLLMsText(origin, i18n.defaultLanguage), {
    headers: {
      'Content-Type': 'text/plain; charset=utf-8',
    },
  });
}
