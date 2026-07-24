import { source } from '@/lib/source';
import { createFromSource } from 'fumadocs-core/search/server';
import { createTokenizer as createMandarinTokenizer } from '@orama/tokenizers/mandarin';

export const { GET } = createFromSource(source, {
  localeMap: {
    en: { language: 'english' },
    zh: {
      components: { tokenizer: createMandarinTokenizer() },
      search: { threshold: 0, tolerance: 0 },
    },
  },
});
