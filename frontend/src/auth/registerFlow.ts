export type RegisterStep = 'form' | 'verify';

/** Known only after GET /auth/registration succeeds. Failure stays unavailable. */
export type RegistrationConfigStatus = 'loading' | 'enabled' | 'disabled' | 'unavailable';

export const REGISTRATION_UNAVAILABLE_MESSAGE = '注册暂不可用，请稍后重试';

/** State after a successful register/start. Password must not remain in the form. */
export function stateAfterRegisterStart(): { password: string; code: string; registerStep: RegisterStep } {
  return { password: '', code: '', registerStep: 'verify' };
}

export function registrationConfigAfterFetch(
  result: { ok: true; emailVerificationEnabled: boolean } | { ok: false },
): RegistrationConfigStatus {
  if (!result.ok) return 'unavailable';
  return result.emailVerificationEnabled ? 'enabled' : 'disabled';
}

/** Direct registration only when the server flag is false. Loading and failure stay blocked. */
export function registerSubmitAction(status: RegistrationConfigStatus): 'direct' | 'start' | 'blocked' {
  if (status === 'enabled') return 'start';
  if (status === 'disabled') return 'direct';
  return 'blocked';
}

export function canSubmitRegistration(status: RegistrationConfigStatus): boolean {
  return registerSubmitAction(status) !== 'blocked';
}

/** Verification UI is shown only after start, and only while verification is enabled. */
export function showsVerificationStep(status: RegistrationConfigStatus, step: RegisterStep): boolean {
  return status === 'enabled' && step === 'verify';
}

export function registrationBlockMessage(status: RegistrationConfigStatus): string | null {
  if (status === 'unavailable') return REGISTRATION_UNAVAILABLE_MESSAGE;
  return null;
}

/** Login never depends on the registration-config request. */
export function authFormSubmitPlan(
  mode: 'login' | 'register',
  status: RegistrationConfigStatus,
): 'login' | 'direct' | 'start' | 'blocked' {
  if (mode === 'login') return 'login';
  return registerSubmitAction(status);
}
