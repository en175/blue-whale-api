import type { BaseLayoutProps } from 'fumadocs-ui/layouts/shared';
import { i18n } from '@/lib/i18n';
import Image from 'next/image';
import type { LinkItemType } from 'fumadocs-ui/layouts/docs';
import { ThemeSwitcher } from '@/components/theme-switcher';

// 导航右侧图标链接：Blue Whale 不外跳 git，留空
export const linkItems: LinkItemType[] = [];

// 导航 logo 统一用 TOS blue_whale_v2.svg（与 API 首页系统配置 logo 同源）；
// 该文件为正方形画幅（384×384）、鲸鱼本体仅占约 1/3，
// 渲染时须裁出鲸鱼区域放大（见下方裁切容器），否则显得过小
export const logoUrl =
  'https://bluewhale-assets.tos-cn-beijing.volces.com/blue-whale-api/blue_whale_v2.svg';

// 鲸鱼本体（含尾部星星）在 384 方画幅中的实际边界，
// 按内嵌位图 alpha 通道实测（1536 空间 /4）
const WHALE_BOX = { x: 89.75, y: 122, w: 193.5, h: 120.25 };
const LOGO_H = 20; // 与导航 15px 标题文字视觉对齐

export const logo = (() => {
  const scale = LOGO_H / WHALE_BOX.h;
  const size = 384 * scale;
  return (
    <span
      className="relative inline-block shrink-0 overflow-hidden"
      style={{ width: WHALE_BOX.w * scale, height: LOGO_H }}
    >
      <Image
        alt="Blue Whale API"
        src={logoUrl}
        width={384}
        height={384}
        className="absolute max-w-none"
        style={{
          width: size,
          height: size,
          left: -WHALE_BOX.x * scale,
          top: -WHALE_BOX.y * scale,
        }}
        priority
        unoptimized
      />
    </span>
  );
})();

export function baseOptions(locale: string): BaseLayoutProps {
  return {
    i18n,
    // 自定义主题切换：与门户首页同款 View Transitions 交叉淡化
    themeSwitch: { component: <ThemeSwitcher locale={locale} /> },
    nav: {
      title: (
        <>
          {logo}
          <span className="font-medium in-[header]:text-[15px] [.uwu_&]:hidden">
            Blue Whale API
          </span>
        </>
      ),
    },
  };
}
