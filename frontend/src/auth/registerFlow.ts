export type RegisterStep = 'form' | 'verify';

/** State after a successful register/start. Password must not remain in the form. */
export function stateAfterRegisterStart(): { password: string; code: string; registerStep: RegisterStep } {
  return { password: '', code: '', registerStep: 'verify' };
}
