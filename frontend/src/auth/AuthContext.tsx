import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useRef,
  useState,
  type ReactNode,
} from 'react';
import {
  fetchMe,
  fetchRegistrationOptions,
  login as apiLogin,
  register as apiRegister,
  registerResend as apiRegisterResend,
  registerStart as apiRegisterStart,
  registerVerify as apiRegisterVerify,
  updateMe,
  type AuthUser,
  type RegisterStartResult,
} from '../api';
import { AppToast } from '../components/AppToast';
import { CookieConsent } from '../components/CookieConsent';
import { hasCookieConsent } from './cookieConsent';
import { registrationConfigAfterFetch, type RegistrationConfigStatus } from './registerFlow';
import {
  bindSessionActivity,
  clearLastActivity,
  isSessionIdle,
  touchLastActivity,
} from './sessionIdle';

const TOKEN_KEY = 'vplayer_auth_token';
const TOAST_MS = 2000;

type AuthContextValue = {
  user: AuthUser | null;
  token: string | null;
  ready: boolean;
  /** Server AUTH_REQUIRED. True until the public config response says otherwise. */
  authRequired: boolean;
  /** Server PLAYBACK_AUTH_REQUIRED. Independent of authRequired. True until the response says otherwise. */
  playbackAuthRequired: boolean;
  loginOpen: boolean;
  openLogin: (mode?: 'login' | 'register') => void;
  closeLogin: () => void;
  authMode: 'login' | 'register';
  setAuthMode: (mode: 'login' | 'register') => void;
  login: (email: string, password: string) => Promise<void>;
  registrationConfig: RegistrationConfigStatus;
  retryRegistrationConfig: () => void;
  register: (email: string, password: string, nickname?: string) => Promise<void>;
  registerStart: (email: string, password: string, nickname?: string) => Promise<RegisterStartResult>;
  registerVerify: (email: string, code: string) => Promise<void>;
  registerResend: (email: string) => Promise<RegisterStartResult>;
  logout: () => void;
  updateProfile: (patch: { nickname?: string; avatar_hue?: number }) => Promise<AuthUser>;
  /** Cookie consent accepted — auth may persist to storage. */
  cookieConsent: boolean;
};

const AuthContext = createContext<AuthContextValue | null>(null);

function readStoredToken(): string | null {
  if (!hasCookieConsent()) return null;
  try {
    return localStorage.getItem(TOKEN_KEY);
  } catch {
    return null;
  }
}

function writeStoredToken(token: string): void {
  if (!hasCookieConsent()) return;
  try {
    localStorage.setItem(TOKEN_KEY, token);
  } catch {
    /* ignore */
  }
}

function clearStoredToken(): void {
  try {
    localStorage.removeItem(TOKEN_KEY);
  } catch {
    /* ignore */
  }
}

export function AuthProvider({ children }: { children: ReactNode }) {
  const [cookieConsent, setCookieConsent] = useState(() => hasCookieConsent());
  const [user, setUser] = useState<AuthUser | null>(null);
  const [token, setToken] = useState<string | null>(() => readStoredToken());
  const [sessionReady, setSessionReady] = useState(false);
  const [configReady, setConfigReady] = useState(false);
  const [authRequired, setAuthRequired] = useState(true);
  const [playbackAuthRequired, setPlaybackAuthRequired] = useState(true);
  const ready = sessionReady && configReady;
  const [loginOpen, setLoginOpen] = useState(false);
  const [authMode, setAuthMode] = useState<'login' | 'register'>('login');
  const [registrationConfig, setRegistrationConfig] = useState<RegistrationConfigStatus>('loading');
  const [toastMessage, setToastMessage] = useState<string | null>(null);
  const registrationRequest = useRef(0);
  const toastTimer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const pendingPersistToken = useRef<string | null>(null);

  const flashToast = useCallback((message: string) => {
    setToastMessage(message);
    if (toastTimer.current) clearTimeout(toastTimer.current);
    toastTimer.current = setTimeout(() => {
      setToastMessage(null);
      toastTimer.current = null;
    }, TOAST_MS);
  }, []);

  useEffect(() => {
    return () => {
      if (toastTimer.current) clearTimeout(toastTimer.current);
    };
  }, []);

  const retryRegistrationConfig = useCallback(() => {
    const request = ++registrationRequest.current;
    setRegistrationConfig('loading');
    void fetchRegistrationOptions()
      .then((opts) => {
        if (registrationRequest.current !== request) return;
        setRegistrationConfig(
          registrationConfigAfterFetch({ ok: true, emailVerificationEnabled: opts.email_verification_enabled === true }),
        );
        setAuthRequired(opts.auth_required !== false);
        setPlaybackAuthRequired(opts.playback_auth_required !== false);
        setConfigReady(true);
      })
      .catch(() => {
        if (registrationRequest.current !== request) return;
        setRegistrationConfig(registrationConfigAfterFetch({ ok: false }));
        setAuthRequired(true);
        setPlaybackAuthRequired(true);
        setConfigReady(true);
      });
  }, []);

  useEffect(() => {
    retryRegistrationConfig();
  }, [retryRegistrationConfig]);

  const clearSession = useCallback(() => {
    clearStoredToken();
    clearLastActivity();
    pendingPersistToken.current = null;
    setToken(null);
    setUser(null);
  }, []);

  useEffect(() => {
    let cancelled = false;
    async function hydrate() {
      if (!token) {
        if (!cancelled) {
          setUser(null);
          setSessionReady(true);
        }
        return;
      }
      if (isSessionIdle()) {
        clearSession();
        if (!cancelled) setSessionReady(true);
        return;
      }
      try {
        const me = await fetchMe(token);
        if (!cancelled) {
          setUser(me);
          touchLastActivity();
        }
      } catch {
        clearStoredToken();
        clearLastActivity();
        if (!cancelled) {
          setToken(null);
          setUser(null);
        }
      } finally {
        if (!cancelled) setSessionReady(true);
      }
    }
    void hydrate();
    return () => {
      cancelled = true;
    };
  }, [token, clearSession]);

  // Idle expiry while logged in.
  useEffect(() => {
    if (!token) return;

    const onActivity = () => touchLastActivity();
    const unbind = bindSessionActivity(onActivity);
    touchLastActivity();

    const tick = window.setInterval(() => {
      if (isSessionIdle()) {
        clearSession();
        flashToast('登录已过期，请重新登录');
      }
    }, 30_000);

    return () => {
      unbind();
      window.clearInterval(tick);
    };
  }, [token, clearSession, flashToast]);

  const persist = useCallback((nextToken: string, nextUser: AuthUser) => {
    if (hasCookieConsent()) {
      writeStoredToken(nextToken);
      pendingPersistToken.current = null;
    } else {
      pendingPersistToken.current = nextToken;
    }
    touchLastActivity();
    setToken(nextToken);
    setUser(nextUser);
  }, []);

  const onCookieAccepted = useCallback(() => {
    // CookieConsent already wrote + verified consent; unlock app storage now.
    setCookieConsent(true);
    const pending = pendingPersistToken.current;
    if (pending) {
      writeStoredToken(pending);
      pendingPersistToken.current = null;
      setToken(pending);
    } else if (token) {
      writeStoredToken(token);
    } else {
      try {
        const existing = localStorage.getItem(TOKEN_KEY);
        if (existing) setToken(existing);
      } catch {
        /* ignore */
      }
    }
    touchLastActivity();
  }, [token]);

  const login = useCallback(
    async (email: string, password: string) => {
      const result = await apiLogin(email, password);
      persist(result.token, result.user);
      setLoginOpen(false);
      flashToast('登录成功');
    },
    [persist, flashToast],
  );

  const register = useCallback(
    async (email: string, password: string, nickname?: string) => {
      const result = await apiRegister(email, password, nickname);
      persist(result.token, result.user);
      setLoginOpen(false);
      flashToast('登录成功');
    },
    [persist, flashToast],
  );

  const registerStart = useCallback(async (email: string, password: string, nickname?: string) => {
    return apiRegisterStart(email, password, nickname);
  }, []);

  const registerVerify = useCallback(
    async (email: string, code: string) => {
      const result = await apiRegisterVerify(email, code);
      persist(result.token, result.user);
      setLoginOpen(false);
      flashToast('登录成功');
    },
    [persist, flashToast],
  );

  const registerResend = useCallback(async (email: string) => {
    return apiRegisterResend(email);
  }, []);

  const logout = useCallback(() => {
    clearSession();
  }, [clearSession]);

  const updateProfile = useCallback(
    async (patch: { nickname?: string; avatar_hue?: number }) => {
      const next = await updateMe(token, patch);
      setUser(next);
      touchLastActivity();
      return next;
    },
    [token],
  );

  const openLogin = useCallback(
    (mode: 'login' | 'register' = 'login') => {
      if (!authRequired || !cookieConsent) return;
      setAuthMode(mode);
      setLoginOpen(true);
    },
    [authRequired, cookieConsent],
  );

  useEffect(() => {
    if (!cookieConsent) setLoginOpen(false);
  }, [cookieConsent]);

  useEffect(() => {
    if (!authRequired) setLoginOpen(false);
  }, [authRequired]);

  const closeLogin = useCallback(() => setLoginOpen(false), []);

  const value = useMemo(
    () => ({
      user,
      token,
      ready,
      authRequired,
      playbackAuthRequired,
      loginOpen: authRequired && loginOpen,
      openLogin,
      closeLogin,
      authMode,
      setAuthMode,
      login,
      registrationConfig,
      retryRegistrationConfig,
      register,
      registerStart,
      registerVerify,
      registerResend,
      logout,
      updateProfile,
      cookieConsent,
    }),
    [
      user,
      token,
      ready,
      authRequired,
      playbackAuthRequired,
      loginOpen,
      openLogin,
      closeLogin,
      authMode,
      login,
      registrationConfig,
      retryRegistrationConfig,
      register,
      registerStart,
      registerVerify,
      registerResend,
      logout,
      updateProfile,
      cookieConsent,
    ],
  );

  return (
    <AuthContext.Provider value={value}>
      <div className={cookieConsent ? undefined : 'cookie-gate-locked'} aria-hidden={!cookieConsent}>
        {children}
      </div>
      <AppToast message={toastMessage} />
      <CookieConsent onAccepted={onCookieAccepted} />
    </AuthContext.Provider>
  );
}

export function useAuth() {
  const ctx = useContext(AuthContext);
  if (!ctx) throw new Error('useAuth must be used within AuthProvider');
  return ctx;
}

/** Deterministic auth wrapper for router/header integration tests (no network). */
export function AuthTestProvider({
  user,
  token = 'test-token',
  authRequired = true,
  playbackAuthRequired = true,
  children,
}: {
  user: AuthUser;
  token?: string;
  authRequired?: boolean;
  playbackAuthRequired?: boolean;
  children: ReactNode;
}) {
  const value = useMemo<AuthContextValue>(
    () => ({
      user,
      token,
      ready: true,
      authRequired,
      playbackAuthRequired,
      loginOpen: false,
      openLogin: () => undefined,
      closeLogin: () => undefined,
      authMode: 'login',
      setAuthMode: () => undefined,
      login: async () => undefined,
      registrationConfig: 'disabled',
      retryRegistrationConfig: () => undefined,
      register: async () => undefined,
      registerStart: async () => ({
        verification_required: false,
        expires_in_sec: 0,
        resend_after_sec: 0,
      }),
      registerVerify: async () => undefined,
      registerResend: async () => ({
        verification_required: false,
        expires_in_sec: 0,
        resend_after_sec: 0,
      }),
      logout: () => undefined,
      updateProfile: async () => user,
      cookieConsent: true,
    }),
    [token, user, authRequired, playbackAuthRequired],
  );
  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>;
}
