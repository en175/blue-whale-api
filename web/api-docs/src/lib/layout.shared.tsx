import type { BaseLayoutProps } from 'fumadocs-ui/layouts/shared';
import { i18n } from '@/lib/i18n';
import Image from 'next/image';
import type { LinkItemType } from 'fumadocs-ui/layouts/docs';

// 导航右侧图标链接：Blue Whale 不外跳 git，留空
export const linkItems: LinkItemType[] = [];

export const logoUrl =
  'https://bluewhale-assets.tos-cn-beijing.volces.com/blue-whale-api/blue_whale_v2.svg';

// 磨边定稿版为正方形画幅（384×384），按正方形档位适配，不裁剪
export const logo = (
  <Image
    alt="Blue Whale API"
    src={logoUrl}
    width={20}
    height={20}
    className="size-5"
    priority
    unoptimized
  />
);

export function baseOptions(locale: string): BaseLayoutProps {
  return {
    i18n,
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
