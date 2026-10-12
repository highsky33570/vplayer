import assert from 'node:assert/strict';
import { describe, it } from 'node:test';
import { categoryOwnership } from './categoryOwnership.ts';

describe('categoryOwnership', () => {
  it('marks synced when source_system and source_id are present', () => {
    assert.equal(categoryOwnership({ source_system: 'olehdtv', source_id: '1' }), 'Synced');
  });

  it('marks local when source fields are absent', () => {
    assert.equal(categoryOwnership({}), 'Local');
    assert.equal(categoryOwnership({ source_system: '', source_id: '' }), 'Local');
    assert.equal(categoryOwnership({ source_system: 'olehdtv', source_id: '' }), 'Local');
  });
});
