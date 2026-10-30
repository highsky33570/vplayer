import type { AdminCategory } from '../api';
import { adminMainCategories, adminSubcategories } from './adminVideoCategories';

export type AdminCategoryTreeNode = {
  main: AdminCategory;
  children: AdminCategory[];
};

/** Top-level 首页 (home) row when present — first of the seven folder mains. */
export function findAdminHomeCategory(cats: AdminCategory[]): AdminCategory | undefined {
  return cats.find(
    (c) =>
      (c.parent_id == null || c.parent_id === 0) &&
      (c.slug === 'home' || c.name === '推荐' || c.name === '首页'),
  );
}

/** Canonical folder mains: 首页 + six nav channels (no orphans). */
export function adminFolderMains(cats: AdminCategory[]): AdminCategory[] {
  const byId = new Map(cats.map((c) => [c.id, c]));
  const home = findAdminHomeCategory(cats);
  const mains = adminMainCategories(cats)
    .map((m) => byId.get(m.id))
    .filter((c): c is AdminCategory => Boolean(c));
  if (home) {
    return [home, ...mains.filter((m) => m.id !== home.id)];
  }
  return mains;
}

/** Build main → children tree for 分类管理 (canonical mains only). */
export function buildAdminCategoryTree(cats: AdminCategory[]): AdminCategoryTreeNode[] {
  const byId = new Map(cats.map((c) => [c.id, c]));
  return adminFolderMains(cats).map((main) => {
    const row = byId.get(main.id) ?? main;
    const children = adminSubcategories(cats, main.id)
      .map((c) => byId.get(c.id))
      .filter((c): c is AdminCategory => Boolean(c));
    return { main: row, children };
  });
}

/** Categories not under the folder mains (kept for tests; UI no longer shows them). */
export function orphanAdminCategories(cats: AdminCategory[]): AdminCategory[] {
  const tree = buildAdminCategoryTree(cats);
  const used = new Set<number>();
  for (const node of tree) {
    used.add(node.main.id);
    for (const child of node.children) used.add(child.id);
  }
  return cats.filter((c) => !used.has(c.id)).sort((a, b) => a.id - b.id);
}
