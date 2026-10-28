import assert from 'node:assert/strict';
import { describe, it } from 'node:test';
import {
  membershipBadgeLabel,
  membershipModalActiveCopy,
  membershipPurchaseLocked,
} from './accountLogic.ts';

describe('membershipBadgeLabel', () => {
  it('prefers paid expiry copy over admin lifetime when expires_at is set', () => {
    const label = membershipBadgeLabel({
      is_admin: true,
      is_member: true,
      membership_days_remaining: 365,
      membership_expires_at: '2027-10-28T00:00:00Z',
    });
    assert.equal(label?.badge, '大会员');
    assert.match(label?.detail ?? '', /剩余 365 天/);
    assert.match(label?.detail ?? '', /有效至/);
  });

  it('falls back to admin lifetime when there is no expiry', () => {
    const label = membershipBadgeLabel({ is_admin: true, is_member: true });
    assert.deepEqual(label, { badge: '大会员', detail: '管理员权限 · 永久有效' });
  });
});

describe('membershipPurchaseLocked', () => {
  it('locks while days remain', () => {
    assert.equal(
      membershipPurchaseLocked({
        membership_days_remaining: 10,
        membership_expires_at: '2027-10-28T00:00:00Z',
      }),
      true,
    );
    assert.equal(membershipPurchaseLocked({ membership_days_remaining: 0 }), false);
  });
});

describe('membershipModalActiveCopy', () => {
  it('shows remaining days and no-repurchase copy for a paid member', () => {
    const copy = membershipModalActiveCopy({
      is_admin: true,
      membership_days_remaining: 360,
      membership_expires_at: '2027-10-28T00:00:00Z',
    });
    assert.match(copy ?? '', /剩余 360 天/);
    assert.match(copy ?? '', /无需重复购买/);
  });

  it('shows admin copy only when there is no paid clock', () => {
    assert.equal(
      membershipModalActiveCopy({ is_admin: true }),
      '当前为管理员，已具备大会员权限。',
    );
  });
});
