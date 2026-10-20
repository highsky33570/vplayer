import assert from 'node:assert/strict';
import { describe, it } from 'node:test';
import { AVATAR_HUE_CHOICES, avatarHue, avatarInitial, avatarStyle, derivedHue } from './avatar.ts';

describe('avatar helpers', () => {
  it('derives a stable hue when the user has not picked one', () => {
    assert.equal(avatarHue({ id: 7, avatar_hue: -1 }), derivedHue(7));
    assert.equal(avatarHue({ id: 7, avatar_hue: null }), derivedHue(7));
    assert.equal(avatarHue({ id: 7 }), derivedHue(7));
    assert.equal(avatarHue({ id: 7, avatar_hue: 200 }), 200);
    assert.equal(derivedHue(1), derivedHue(1));
    assert.notEqual(derivedHue(1), derivedHue(2));
  });

  it('picks an initial letter without inventing social metadata', () => {
    assert.equal(avatarInitial('  ada  '), 'A');
    assert.equal(avatarInitial(''), '用');
    assert.equal(avatarInitial(null), '用');
    assert.match(avatarStyle(120), /hsl\(120/);
    assert.ok(AVATAR_HUE_CHOICES.length >= 6);
  });
});
