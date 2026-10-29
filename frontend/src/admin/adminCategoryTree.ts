import type { AdminCategory } from '../api';
import { adminMainCategories, adminSubcategories } from './adminVideoCategories';

export type AdminCategoryTreeNode = {
  main: AdminCategory;
  children: AdminCategory[];
};

/** Build main → children tree for 分类管理 (canonical mains only). */
export function buildAdminCategoryTree(cats: AdminCategory[]): AdminCategoryTreeNode[] {
  const byId = new Map(cats.map((c) => [c.id, c]));
  const mains = adminMainCategories(cats);
  return mains.map((main) => {
    const row = byId.get(main.id)!;
    const children = adminSubcategories(cats, main.id)
      .map((c) => byId.get(c.id))
      .filter((c): c is AdminCategory => Boolean(c));
    return { main: row, children };
  });
}

/** Categories not under the six canonical mains (shown separately). */
export function orphanAdminCategories(cats: AdminCategory[]): AdminCategory[] {
  const tree = buildAdminCategoryTree(cats);
  const used = new Set<number>();
  for (const node of tree) {
    used.add(node.main.id);
    for (const child of node.children) used.add(child.id);
  }
  return cats.filter((c) => !used.has(c.id)).sort((a, b) => a.id - b.id);
}
