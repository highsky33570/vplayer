export type CategoryOwnership = 'Synced' | 'Local';

/** Derive ownership from source fields (not persisted). */
export function categoryOwnership(c: {
  source_system?: string;
  source_id?: string;
}): CategoryOwnership {
  const system = (c.source_system ?? '').trim();
  const sourceID = (c.source_id ?? '').trim();
  if (system !== '' && sourceID !== '') return 'Synced';
  return 'Local';
}
