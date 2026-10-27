/** Hand-drawn stroke icons for category rails and the mobile side drawer. */
export function CategoryIcon({
  slug,
  name,
  size = 16,
}: {
  slug: string;
  name: string;
  size?: number;
}) {
  const key = `${slug} ${name}`.toLowerCase();
  const props = {
    width: size,
    height: size,
    viewBox: '0 0 24 24',
    fill: 'none' as const,
    'aria-hidden': true as const,
  };
  const stroke = {
    stroke: 'currentColor',
    strokeWidth: 1.8,
    strokeLinecap: 'round' as const,
    strokeLinejoin: 'round' as const,
  };

  if (key.includes('home') || name === '首页') {
    return (
      <svg {...props}>
        <path d="M4 11.5 12 4.5l8 7" {...stroke} />
        <path d="M7 10.5V19h10v-8.5" {...stroke} />
      </svg>
    );
  }
  if (key.includes('movie') || name.includes('电影')) {
    return (
      <svg {...props}>
        <rect x="4" y="6" width="16" height="12" rx="2" {...stroke} />
        <path d="M4 9h16M4 15h16M8 6v3M8 15v3M16 6v3M16 15v3" {...stroke} />
      </svg>
    );
  }
  if (key.includes('tv') || name.includes('连续剧') || name.includes('剧集') || name.includes('电视剧')) {
    return (
      <svg {...props}>
        <rect x="3.5" y="5" width="17" height="12" rx="2" {...stroke} />
        <path d="M8 20h8M12 17v3" {...stroke} />
      </svg>
    );
  }
  if (key.includes('variety') || name.includes('综艺')) {
    return (
      <svg {...props}>
        <path d="M12 4v3M8.5 5.5 10 8M15.5 5.5 14 8" {...stroke} />
        <path d="M8 11a4 4 0 0 1 8 0v2H8v-2Z" {...stroke} />
        <path d="M7 15h10v2a2 2 0 0 1-2 2H9a2 2 0 0 1-2-2v-2Z" {...stroke} />
      </svg>
    );
  }
  if (key.includes('anime') || name.includes('动漫')) {
    return (
      <svg {...props}>
        <circle cx="12" cy="13" r="6.5" {...stroke} />
        <path d="M8.5 12.5h.01M15.5 12.5h.01M9.5 16c1 .8 4 .8 5 0" {...stroke} />
        <path d="M7 7.5 9.5 10M17 7.5 14.5 10" {...stroke} />
      </svg>
    );
  }
  if (key.includes('short') || name.includes('短剧')) {
    return (
      <svg {...props}>
        <path d="M7 7h10v10H7z" {...stroke} />
        <path d="M10 10.5 14.5 12 10 13.5v-3Z" {...stroke} />
        <path d="M9 5h6M9 19h6" {...stroke} />
      </svg>
    );
  }
  if (key.includes('news') || name.includes('资讯') || name.includes('信息')) {
    return (
      <svg {...props}>
        <path d="M6 5h12v14H6z" {...stroke} />
        <path d="M9 9h6M9 12h6M9 15h4" {...stroke} />
      </svg>
    );
  }
  if (key.includes('rank') || name.includes('排行')) {
    return (
      <svg {...props}>
        <path d="M5 19V11h4v8M10 19V5h4v14M15 19v-6h4v6" {...stroke} />
      </svg>
    );
  }
  if (key.includes('msg') || name.includes('留言') || name.includes('消息')) {
    return (
      <svg {...props}>
        <path d="M4 6h16v10H8l-4 3V6z" {...stroke} />
      </svg>
    );
  }
  if (key.includes('midnight') || name.includes('午夜')) {
    return (
      <svg {...props}>
        <path d="M15.5 4.5A7.5 7.5 0 1 0 19.5 14 6 6 0 0 1 15.5 4.5Z" {...stroke} />
      </svg>
    );
  }
  if (key.includes('vip') || name.includes('蓝光')) {
    return (
      <svg {...props}>
        <path d="M5 9.5 7.5 6h3L12 8l1.5-2h3L19 9.5 12 19 5 9.5Z" {...stroke} />
      </svg>
    );
  }
  return (
    <svg {...props}>
      <circle cx="12" cy="12" r="7" {...stroke} />
      <path d="M12 8.5v7M8.5 12h7" {...stroke} />
    </svg>
  );
}
