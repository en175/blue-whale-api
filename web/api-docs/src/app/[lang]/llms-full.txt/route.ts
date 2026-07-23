import { generateLLMsFullText } from '@/lib/llms';

export const revalidate = false;

export async function GET(
  _req: Request,
  { params }: { params: Promise<{ lang: string }> }
) {
  const { lang } = await params;

  return new Response(await generateLLMsFullText(lang), {
    headers: {
      'Content-Type': 'text/plain; charset=utf-8',
    },
  });
}
