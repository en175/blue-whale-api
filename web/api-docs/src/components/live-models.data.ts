// 模型列表改为静态内置（来源：网关 GET /api/docs/config 的 data.models）
// 国内渠道: 方舟 doubao-seedance-*;海外渠道: BytePlus dreamina-seedance-*
// 网关模型有增删时,需手动同步并重新构建部署
export type DocModel = {
  model_name: string;
  description: string;
  tag: string;
  region: 'domestic' | 'overseas';
  sort_order: number;
};

export const STATIC_MODELS: DocModel[] = [
  // 国内渠道(方舟)
  {
    model_name: 'doubao-seedance-2-5-260628',
    description: '30秒超长叙事，全模态参考扩容',
    tag: '全面开放',
    region: 'domestic',
    sort_order: 1,
  },
  {
    model_name: 'doubao-seedance-2-0-260128',
    description: '超强的参考一致性，极致拟真的视听稳定性',
    tag: '',
    region: 'domestic',
    sort_order: 2,
  },
  {
    model_name: 'doubao-seedance-2-0-mini-260615',
    description: '音视图文参考，高性价比',
    tag: '',
    region: 'domestic',
    sort_order: 3,
  },
  {
    model_name: 'doubao-seedance-2-0-fast-260128',
    description: '音视图文参考，更快，继承2.0核心优势',
    tag: '',
    region: 'domestic',
    sort_order: 4,
  },
  // 海外渠道(BytePlus)
  {
    model_name: 'dreamina-seedance-2-0-mini-260615',
    description: 'Audio/video/image reference, cost-effective',
    tag: '',
    region: 'overseas',
    sort_order: 1,
  },
  {
    model_name: 'dreamina-seedance-2-0-260128',
    description:
      'Strong reference consistency, realistic audio-visual stability',
    tag: '',
    region: 'overseas',
    sort_order: 2,
  },
  {
    model_name: 'dreamina-seedance-2-0-fast-260128',
    description:
      'Audio/video/image reference, faster, inherits 2.0 core advantages',
    tag: '',
    region: 'overseas',
    sort_order: 3,
  },
];

export const L = {
  empty: '暂无对外开放的模型',
  section: {
    domestic: '国内可用(方舟渠道)',
    overseas: '海外可用(BytePlus 渠道)',
  },
} as const;
