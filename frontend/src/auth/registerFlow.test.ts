import assert from 'node:assert/strict';
import { describe, it } from 'node:test';
import {
  authFormSubmitPlan,
  canSubmitRegistration,
  registrationBlockMessage,
  registrationConfigAfterFetch,
  registerSubmitAction,
  showsVerificationStep,
  stateAfterRegisterStart,
} from './registerFlow.ts';

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

describe('email verification flag', () => {
  it('config false uses direct registration', () => {
    const status = registrationConfigAfterFetch({ ok: true, emailVerificationEnabled: false });
    assert.equal(status, 'disabled');
    assert.equal(registerSubmitAction(status), 'direct');
    assert.equal(canSubmitRegistration(status), true);
    assert.equal(showsVerificationStep(status, 'form'), false);
    assert.equal(showsVerificationStep(status, 'verify'), false);
  });

  it('config true keeps the verification flow', () => {
    const status = registrationConfigAfterFetch({ ok: true, emailVerificationEnabled: true });
    assert.equal(status, 'enabled');
    assert.equal(registerSubmitAction(status), 'start');
    assert.equal(canSubmitRegistration(status), true);
    const next = stateAfterRegisterStart();
    assert.equal(showsVerificationStep(status, next.registerStep), true);
    assert.equal(showsVerificationStep(status, 'form'), false);
  });

  it('config loading blocks registration submit', () => {
    assert.equal(registerSubmitAction('loading'), 'blocked');
    assert.equal(canSubmitRegistration('loading'), false);
    assert.notEqual(registerSubmitAction('loading'), 'direct');
    assert.equal(showsVerificationStep('loading', 'verify'), false);
  });

  it('config failure blocks registration and does not fall back to direct registration', () => {
    const status = registrationConfigAfterFetch({ ok: false });
    assert.equal(status, 'unavailable');
    assert.notEqual(status, 'disabled');
    assert.equal(registerSubmitAction(status), 'blocked');
    assert.equal(canSubmitRegistration(status), false);
    assert.notEqual(registerSubmitAction(status), 'direct');
    assert.equal(registrationBlockMessage(status), '注册暂不可用，请稍后重试');
  });

  it('login stays usable when registration config is loading or failed', () => {
    assert.equal(authFormSubmitPlan('login', 'loading'), 'login');
    assert.equal(authFormSubmitPlan('login', 'unavailable'), 'login');
    assert.equal(authFormSubmitPlan('login', 'disabled'), 'login');
    assert.equal(authFormSubmitPlan('login', 'enabled'), 'login');
    assert.equal(authFormSubmitPlan('register', 'unavailable'), 'blocked');
    assert.equal(authFormSubmitPlan('register', 'loading'), 'blocked');
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
