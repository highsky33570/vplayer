import { useEffect, useState } from 'react';
import { useLocation, useNavigate, useSearchParams } from 'react-router-dom';
import { listCategories, type Category } from '../api';
import { useAuth } from '../auth/AuthContext';
import { CategoryIcon } from './categoryIcons';
import { resolveNavCategories } from './discoveryNavLogic';

type DrawerItem = {
  id: string;
  label: string;
  slug: string;
  name: string;
  href?: string;
};

export function MobileCategoryDrawer({
  open,
  onClose,
}: {
  open: boolean;
  onClose: () => void;
}) {
  const navigate = useNavigate();
  const { pathname } = useLocation();
  const [params] = useSearchParams();
  const { user, authRequired, openLogin } = useAuth();
  const [categories, setCategories] = useState<Category[]>([]);
  const active = pathname === '/' ? params.get('ch') || 'home' : '';
  const rankHot = pathname === '/' && !params.get('q')?.trim() && active === 'home' && params.get('rank') === 'hot';

  useEffect(() => {
    let cancelled = false;
    listCategories()
      .then((rows) => {
        if (!cancelled) setCategories(rows);
      })
      .catch(() => {
        if (!cancelled) setCategories([]);
      });
    return () => {
      cancelled = true;
    };
  }, []);

  useEffect(() => {
    if (!open) return;
    function onKey(e: KeyboardEvent) {
      if (e.key === 'Escape') onClose();
    }
    document.addEventListener('keydown', onKey);
    const prev = document.body.style.overflow;
    document.body.style.overflow = 'hidden';
    return () => {
      document.removeEventListener('keydown', onKey);
      document.body.style.overflow = prev;
    };
  }, [open, onClose]);

  const mains = resolveNavCategories(categories);
  const items: DrawerItem[] = [
    { id: 'home', label: '首页', slug: 'home', name: '首页' },
    ...mains.map((main) => ({
      id: `cat-${main.id}`,
      label: main.name,
      slug: main.slug,
      name: main.name,
    })),
    { id: 'rank', label: '排行榜', slug: 'rank', name: '排行榜', href: '/?rank=hot' },
    { id: 'messages', label: '留言板', slug: 'msg', name: '留言板', href: '/messages' },
  ];

  function go(item: DrawerItem) {
    onClose();
    if (item.href) {
      navigate(item.href);
      return;
    }
    if (item.slug === 'home') navigate('/');
    else navigate(`/?ch=${encodeURIComponent(item.slug)}`);
  }

  function isActive(item: DrawerItem) {
    if (item.id === 'rank') return rankHot;
    if (item.id === 'messages') return pathname.startsWith('/messages');
    if (item.slug === 'home') return pathname === '/' && active === 'home' && !rankHot;
    return pathname === '/' && active === item.slug;
  }

  return (
    <div className={`mobile-cat-drawer${open ? ' is-open' : ''}`} aria-hidden={!open}>
      <button type="button" className="mobile-cat-backdrop" aria-label="关闭菜单" onClick={onClose} />
      <aside className="mobile-cat-panel" role="dialog" aria-modal="true" aria-label="分类导航">
        <div className="mobile-cat-hero">
          {authRequired && !user ? (
            <button
              type="button"
              className="mobile-cat-login"
              onClick={() => {
                onClose();
                openLogin('login');
              }}
            >
              登录账号
            </button>
          ) : user ? (
            <button
              type="button"
              className="mobile-cat-login"
              onClick={() => {
                onClose();
                navigate('/account');
              }}
            >
              {user.nickname || '我的账号'}
            </button>
          ) : (
            <button
              type="button"
              className="mobile-cat-login"
              onClick={() => {
                onClose();
                navigate('/account');
              }}
            >
              浏览空间
            </button>
          )}
        </div>
        <nav className="mobile-cat-list" aria-label="频道分类">
          {items.map((item) => (
            <button
              key={item.id}
              type="button"
              className={`mobile-cat-item${isActive(item) ? ' is-active' : ''}`}
              onClick={() => go(item)}
            >
              <span className="mobile-cat-icon" aria-hidden>
                <CategoryIcon slug={item.slug} name={item.name} size={20} />
              </span>
              <span>{item.label}</span>
            </button>
          ))}
        </nav>
      </aside>
    </div>
  );
}
