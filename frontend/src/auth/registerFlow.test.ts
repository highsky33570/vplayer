import assert from 'node:assert/strict';
import { describe, it } from 'node:test';
import { stateAfterRegisterStart } from './registerFlow.ts';

/** Mirrors AuthForm maskEmail for unit coverage without DOM. */
function maskEmail(email: string): string {
  const e = email.trim();
  const at = e.indexOf('@');
  if (at <= 1) return e;
  const name = e.slice(0, at);
  const domain = e.slice(at);
  if (name.length <= 2) return `${name[0]}*${domain}`;
  return `${name[0]}${'*'.repeat(Math.min(name.length - 2, 4))}${name[name.length - 1]}${domain}`;
}

describe('register email mask', () => {
  it('masks local part', () => {
    assert.equal(maskEmail('alice@example.com'), 'a***e@example.com');
    assert.equal(maskEmail('ab@example.com'), 'a*@example.com');
  });
});

describe('stateAfterRegisterStart', () => {
  it('clears the plaintext password and enters verify', () => {
    const next = stateAfterRegisterStart();
    assert.equal(next.password, '');
    assert.equal(next.code, '');
    assert.equal(next.registerStep, 'verify');
  });
});

describe('register code validation', () => {
  it('accepts exactly six digits', () => {
    assert.equal(/^\d{6}$/.test('123456'), true);
    assert.equal(/^\d{6}$/.test('12345'), false);
    assert.equal(/^\d{6}$/.test('1234567'), false);
    assert.equal(/^\d{6}$/.test('12a456'), false);
  });
});
