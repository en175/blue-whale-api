import { describe, test, expect } from 'bun:test';
import { extractAssetRefsFromText } from './asset-references';

describe('extractAssetRefsFromText', () => {
  test('extracts markdown image refs', () => {
    const refs = extractAssetRefsFromText('![x](/assets/guide/pricing.png)');
    expect(refs).toContain('/assets/guide/pricing.png');
  });

  test('extracts src attr refs', () => {
    const refs = extractAssetRefsFromText('src="/assets/aionui/add-model-1.png"');
    expect(refs).toContain('/assets/aionui/add-model-1.png');
  });

  test('ignores external github user-attachments urls', () => {
    const refs = extractAssetRefsFromText(
      'src="https://github.com/user-attachments/assets/03e974ba-4605-4bfd-83ef-dd32a30974de"'
    );
    expect(refs).not.toContain('/assets/03e974ba-4605-4bfd-83ef-dd32a30974de');
    expect(refs).toHaveLength(0);
  });

  test('ignores url-preceded assets path', () => {
    expect(extractAssetRefsFromText('http://x.com/assets/foo.png')).toHaveLength(0);
  });

  test('matches line-start refs', () => {
    expect(extractAssetRefsFromText('/assets/start.png')).toContain('/assets/start.png');
  });
});
