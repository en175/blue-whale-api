import { source } from '@/lib/source';
import { createFromSource } from 'fumadocs-core/search/server';
import { createTokenizer as createMandarinTokenizer } from '@orama/tokenizers/mandarin';
import { createTokenizer as createJapaneseTokenizer } from '@orama/tokenizers/japanese';

export const { GET } = createFromSource(source, {
  localeMap: {
    en: { language: 'english' },
    zh: {
      components: { tokenizer: createMandarinTokenizer() },
      search: { threshold: 0, tolerance: 0 },
    },
    ja: {
      components: { tokenizer: createJapaneseTokenizer() },
      search: { threshold: 0, tolerance: 0 },
    },
  },
});
