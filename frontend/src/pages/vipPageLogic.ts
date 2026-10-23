import type { Category, Video } from '../api';

export const VIP_CHANNEL_SLUG = 'vip-bluray';
export const VIP_CHANNEL_NAME = 'VIP蓝光影院';

/** Truthful feature cards — only capabilities VPlayer actually has. */
export const VIP_FEATURE_CARDS = [
  {
    id: 'catalog',
    title: '精选片库',
    body: '集中浏览 VIP蓝光影院 频道中的高质量影视内容。',
  },
  {
    id: 'quality',
    title: '片源整理',
    body: '按分类整理的片源入口，方便按兴趣筛选与观看。',
  },
  {
    id: 'episodes',
    title: '分集浏览',
    body: '剧集类内容支持分集选择，连续追剧更清晰。',
  },
  {
    id: 'devices',
    title: '跨端访问',
    body: '登录后可在不同设备上继续浏览与播放站点内容。',
  },
] as const;

export function resolveVipCategory(categories: Category[]): Category | undefined {
  return (
    categories.find((c) => c.slug === VIP_CHANNEL_SLUG) ||
    categories.find((c) => c.name === VIP_CHANNEL_NAME)
  );
}

export function pickVipHero(videos: Video[]): Video | null {
  return videos[0] ?? null;
}

export function vipFeaturedStrip(videos: Video[], limit = 8): Video[] {
  return videos.slice(0, Math.max(0, limit));
}

export function vipCatalogGrid(videos: Video[], offset = 1, limit = 24): Video[] {
  return videos.slice(Math.max(0, offset), Math.max(0, offset) + Math.max(0, limit));
}

/** Status copy for the VIP page card — never invents paid membership. */
export function vipAccessStatus(authenticated: boolean, authRequired = true): { label: string; detail: string } {
  if (!authRequired) {
    return {
      label: '可浏览 VIP蓝光影院',
      detail: '天涯欧乐 没有付费会员等级。可浏览本频道片库。片源本身不可播放时仍不可用，并不表示会员限制已解除。',
    };
  }
  if (authenticated) {
    return {
      label: '可浏览 VIP蓝光影院',
      detail: '天涯欧乐 没有付费会员等级。登录后即可浏览本频道片库并观看内容。',
    };
  }
  return {
    label: '登录后观看',
    detail: '可先浏览 VIP蓝光影院 片库；完整播放需要登录。站点没有付费订阅。',
  };
}
