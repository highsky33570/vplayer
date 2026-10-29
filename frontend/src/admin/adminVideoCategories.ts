import type { AdminCategory, Category } from '../api';
import { childrenOfParent, resolveNavCategories } from '../components/discoveryNavLogic';

/** Map admin categories into the public Category shape for nav helpers. */
export function adminCategoriesAsNav(cats: AdminCategory[]): Category[] {
  return cats.map((c) => ({
    id: c.id,
    name: c.name,
    slug: c.slug,
    sort: c.sort,
    parent_id: c.parent_id ?? null,
    source_id: c.source_id,
  }));
}

/** Canonical main channels shown in the admin videos category filter (excl. 首页). */
export function adminMainCategories(cats: AdminCategory[]): Category[] {
  return resolveNavCategories(adminCategoriesAsNav(cats));
}

/** Subcategories under a main channel id. */
export function adminSubcategories(cats: AdminCategory[], mainId: number): Category[] {
  if (!mainId) return [];
  return childrenOfParent(adminCategoriesAsNav(cats), mainId);
}

/**
 * Resolve which main + optional subcategory the current category_id maps to.
 * categoryId 0 => 首页 (no filter).
 */
export function resolveAdminCategorySelection(
  cats: AdminCategory[],
  categoryId: number,
): { mainId: number; subId: number } {
  if (!categoryId) return { mainId: 0, subId: 0 };
  const byId = new Map(cats.map((c) => [c.id, c]));
  const selected = byId.get(categoryId);
  if (!selected) return { mainId: 0, subId: 0 };
  const parentId = selected.parent_id != null && selected.parent_id > 0 ? selected.parent_id : 0;
  if (parentId > 0) {
    return { mainId: parentId, subId: selected.id };
  }
  return { mainId: selected.id, subId: 0 };
}
