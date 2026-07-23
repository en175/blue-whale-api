import { generateLLMsText } from '@/lib/llms';

export const revalidate = false;

export async function GET(
  req: Request,
  { params }: { params: Promise<{ lang: string }> }
) {
  const { lang } = await params;
  const origin = new URL(req.url).origin;

  return new Response(generateLLMsText(origin, lang), {
    headers: {
      'Content-Type': 'text/plain; charset=utf-8',
    },
  });
}
