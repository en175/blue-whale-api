import { describe, test, expect } from 'bun:test';
import { STATIC_MODELS, L } from './live-models.data';

describe('live-models.data', () => {
  test('STATIC_MODELS 非空且同时含国内与海外渠道', () => {
    expect(STATIC_MODELS.length).toBeGreaterThan(0);
    const domestic = STATIC_MODELS.filter((m) => m.region === 'domestic');
    const overseas = STATIC_MODELS.filter((m) => m.region === 'overseas');
    expect(domestic.length).toBeGreaterThan(0);
    expect(overseas.length).toBeGreaterThan(0);
  });

  test('国内渠道(region=domestic)model_name 全部以 doubao- 前缀', () => {
    for (const m of STATIC_MODELS.filter((x) => x.region === 'domestic')) {
      expect(m.model_name).toMatch(/^doubao-/);
    }
  });

  test('海外渠道(region=overseas)model_name 全部以 dreamina- 前缀', () => {
    for (const m of STATIC_MODELS.filter((x) => x.region === 'overseas')) {
      expect(m.model_name).toMatch(/^dreamina-/);
    }
  });

  test('国内与海外渠道的 model_name 无交集', () => {
    const domestic = new Set(
      STATIC_MODELS.filter((m) => m.region === 'domestic').map(
        (m) => m.model_name
      )
    );
    for (const m of STATIC_MODELS.filter((x) => x.region === 'overseas')) {
      expect(domestic.has(m.model_name)).toBe(false);
    }
  });

  test('L 文案均非空', () => {
    expect(L.empty.length).toBeGreaterThan(0);
    expect(L.section.domestic.length).toBeGreaterThan(0);
    expect(L.section.overseas.length).toBeGreaterThan(0);
  });
});
