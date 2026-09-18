import { describe, test, expect } from 'bun:test';
import { STATIC_MODELS, L } from './live-models.data';

describe('live-models.data', () => {
  test('STATIC_MODELS 非空且三类能力均有模型', () => {
    expect(STATIC_MODELS.length).toBeGreaterThan(0);
    const text = STATIC_MODELS.filter((m) => m.category === 'text');
    const image = STATIC_MODELS.filter((m) => m.category === 'image');
    const video = STATIC_MODELS.filter((m) => m.category === 'video');
    expect(text.length).toBeGreaterThan(0);
    expect(image.length).toBeGreaterThan(0);
    expect(video.length).toBeGreaterThan(0);
  });

  test('视频模型同时含国内与海外渠道', () => {
    const video = STATIC_MODELS.filter((m) => m.category === 'video');
    const domestic = video.filter((m) => m.region === 'domestic');
    const overseas = video.filter((m) => m.region === 'overseas');
    expect(domestic.length).toBeGreaterThan(0);
    expect(overseas.length).toBeGreaterThan(0);
  });

  test('视频模型国内渠道(region=domestic)model_name 全部以 doubao- 前缀', () => {
    for (const m of STATIC_MODELS.filter(
      (x) => x.category === 'video' && x.region === 'domestic'
    )) {
      expect(m.model_name).toMatch(/^doubao-/);
    }
  });

  test('视频模型海外渠道(region=overseas)model_name 全部以 dreamina- 前缀', () => {
    for (const m of STATIC_MODELS.filter(
      (x) => x.category === 'video' && x.region === 'overseas'
    )) {
      expect(m.model_name).toMatch(/^dreamina-/);
    }
  });

  test('model_name 全局唯一', () => {
    const seen = new Set(STATIC_MODELS.map((m) => m.model_name));
    expect(seen.size).toBe(STATIC_MODELS.length);
  });

  test('L 文案均非空', () => {
    expect(L.empty.length).toBeGreaterThan(0);
    expect(L.section.text.length).toBeGreaterThan(0);
    expect(L.section.image.length).toBeGreaterThan(0);
    expect(L.section.videoDomestic.length).toBeGreaterThan(0);
    expect(L.section.videoOverseas.length).toBeGreaterThan(0);
  });
});
