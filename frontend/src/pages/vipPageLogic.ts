import type { Category, Video } from '../api';

export const VIP_CHANNEL_SLUG = 'vip-bluray';
export const VIP_CHANNEL_NAME = 'VIP蓝光影院';

/** Hero copy on /vip — about premium membership, not a single film. */
export const VIP_HERO_COPY = {
  kicker: '天涯欧乐 · 大会员',
  title: '开通大会员，畅看专属影库',
  desc: '升级大会员即可解锁 VIP蓝光影院专属内容。支持 USDT 加密货币支付，开通后可在账户查看徽章与剩余天数。',
} as const;

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

/** Status copy for the VIP / 大会员 page. */
export function vipAccessStatus(authenticated: boolean, authRequired = true): { label: string; detail: string } {
  if (!authRequired) {
    return {
      label: '可浏览大会员片库',
      detail: '可浏览本频道片库。会员专属影片需升级大会员后才能播放。',
    };
  }
  if (authenticated) {
    return {
      label: '可浏览大会员片库',
      detail: '可浏览本频道片库。会员专属影片需升级大会员后才能播放；管理员可在后台为用户开通大会员。',
    };
  }
  return {
    label: '浏览片库 · 升级后观看',
    detail: '可先浏览 VIP蓝光影院 片库；会员专属内容需升级大会员后才能观看。',
  };
}
