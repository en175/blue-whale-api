// 模型列表改为静态内置（来源：网关 GET /api/docs/config 的 data.models）
// 按能力分区：文本（text）/ 图片（image）/ 视频（video，再分国内渠道与海外渠道）
// 国内渠道: 方舟 doubao-*;海外渠道: BytePlus dreamina-*
// 网关模型有增删时,需手动同步并重新构建部署
export type DocModel = {
  model_name: string;
  description: string;
  tag: string;
  category: 'text' | 'image' | 'video';
  // 仅视频模型区分渠道:国内(方舟) / 海外(BytePlus)
  region?: 'domestic' | 'overseas';
  sort_order: number;
};

export const STATIC_MODELS: DocModel[] = [
  // 文本模型
  {
    model_name: 'doubao-seed-2-1-pro-260915',
    description: '豆包 Seed 2.1 Pro',
    tag: '',
    category: 'text',
    sort_order: 1,
  },
  {
    model_name: 'deepseek-v4-1-flash-260910',
    description: 'DeepSeek V4.1 Flash',
    tag: '',
    category: 'text',
    sort_order: 2,
  },
  {
    model_name: 'kimi-k3-260716',
    description: 'Kimi K3',
    tag: '',
    category: 'text',
    sort_order: 3,
  },
  {
    model_name: 'glm-5-2-260617',
    description: 'GLM-5.2',
    tag: '',
    category: 'text',
    sort_order: 4,
  },
  {
    model_name: 'deepseek-v4-pro-ga-260813',
    description: 'DeepSeek V4 Pro',
    tag: '',
    category: 'text',
    sort_order: 5,
  },
  {
    model_name: 'deepseek-v4-flash-ga-260731',
    description: 'DeepSeek V4 Flash',
    tag: '',
    category: 'text',
    sort_order: 6,
  },
  // 图片模型
  {
    model_name: 'doubao-seedream-5-0-pro-260628',
    description: 'Seedream 5.0 Pro',
    tag: '',
    category: 'image',
    sort_order: 1,
  },
  {
    model_name: 'doubao-seedream-5-0-260128',
    description: 'Seedream 5.0',
    tag: '',
    category: 'image',
    sort_order: 2,
  },
  // 视频模型 · 国内渠道(方舟)
  {
    model_name: 'doubao-seedance-2-5-260628',
    description: '30秒超长叙事，全模态参考扩容',
    tag: '全面开放',
    category: 'video',
    region: 'domestic',
    sort_order: 1,
  },
  {
    model_name: 'doubao-seedance-2-0-260128',
    description: '超强的参考一致性，极致拟真的视听稳定性',
    tag: '',
    category: 'video',
    region: 'domestic',
    sort_order: 2,
  },
  {
    model_name: 'doubao-seedance-2-0-mini-260615',
    description: '音视图文参考，高性价比',
    tag: '',
    category: 'video',
    region: 'domestic',
    sort_order: 3,
  },
  {
    model_name: 'doubao-seedance-2-0-fast-260128',
    description: '音视图文参考，更快，继承2.0核心优势',
    tag: '',
    category: 'video',
    region: 'domestic',
    sort_order: 4,
  },
  // 视频模型 · 海外渠道(BytePlus)
  {
    model_name: 'dreamina-seedance-2-0-mini-260615',
    description: 'Audio/video/image reference, cost-effective',
    tag: '',
    category: 'video',
    region: 'overseas',
    sort_order: 1,
  },
  {
    model_name: 'dreamina-seedance-2-0-260128',
    description:
      'Strong reference consistency, realistic audio-visual stability',
    tag: '',
    category: 'video',
    region: 'overseas',
    sort_order: 2,
  },
  {
    model_name: 'dreamina-seedance-2-0-fast-260128',
    description:
      'Audio/video/image reference, faster, inherits 2.0 core advantages',
    tag: '',
    category: 'video',
    region: 'overseas',
    sort_order: 3,
  },
];

export const L = {
  empty: '暂无对外开放的模型',
  section: {
    text: '文本模型',
    image: '图片模型',
    videoDomestic: '视频模型 · 国内可用(方舟渠道)',
    videoOverseas: '视频模型 · 海外可用(BytePlus 渠道)',
  },
} as const;
