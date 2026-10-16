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
  type AuthUser,
  type RegisterStartResult,
} from '../api';
import { registrationConfigAfterFetch, type RegistrationConfigStatus } from './registerFlow';

const TOKEN_KEY = 'vplayer_auth_token';

type AuthContextValue = {
  user: AuthUser | null;
  token: string | null;
  ready: boolean;
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
};

const AuthContext = createContext<AuthContextValue | null>(null);

export function AuthProvider({ children }: { children: ReactNode }) {
  const [user, setUser] = useState<AuthUser | null>(null);
  const [token, setToken] = useState<string | null>(() => localStorage.getItem(TOKEN_KEY));
  const [ready, setReady] = useState(false);
  const [loginOpen, setLoginOpen] = useState(false);
  const [authMode, setAuthMode] = useState<'login' | 'register'>('login');
  const [registrationConfig, setRegistrationConfig] = useState<RegistrationConfigStatus>('loading');
  const registrationRequest = useRef(0);

  const retryRegistrationConfig = useCallback(() => {
    const request = ++registrationRequest.current;
    setRegistrationConfig('loading');
    void fetchRegistrationOptions()
      .then((opts) => {
        if (registrationRequest.current !== request) return;
        setRegistrationConfig(
          registrationConfigAfterFetch({ ok: true, emailVerificationEnabled: opts.email_verification_enabled === true }),
        );
      })
      .catch(() => {
        if (registrationRequest.current !== request) return;
        setRegistrationConfig(registrationConfigAfterFetch({ ok: false }));
      });
  }, []);

  useEffect(() => {
    retryRegistrationConfig();
  }, [retryRegistrationConfig]);

  useEffect(() => {
    let cancelled = false;
    async function hydrate() {
      if (!token) {
        if (!cancelled) {
          setUser(null);
          setReady(true);
        }
        return;
      }
      try {
        const me = await fetchMe(token);
        if (!cancelled) setUser(me);
      } catch {
        localStorage.removeItem(TOKEN_KEY);
        if (!cancelled) {
          setToken(null);
          setUser(null);
        }
      } finally {
        if (!cancelled) setReady(true);
      }
    }
    void hydrate();
    return () => {
      cancelled = true;
    };
  }, [token]);

  const persist = useCallback((nextToken: string, nextUser: AuthUser) => {
    localStorage.setItem(TOKEN_KEY, nextToken);
    setToken(nextToken);
    setUser(nextUser);
  }, []);

  const login = useCallback(
    async (email: string, password: string) => {
      const result = await apiLogin(email, password);
      persist(result.token, result.user);
      setLoginOpen(false);
    },
    [persist],
  );

  const register = useCallback(
    async (email: string, password: string, nickname?: string) => {
      const result = await apiRegister(email, password, nickname);
      persist(result.token, result.user);
      setLoginOpen(false);
    },
    [persist],
  );

  const registerStart = useCallback(async (email: string, password: string, nickname?: string) => {
    return apiRegisterStart(email, password, nickname);
  }, []);

  const registerVerify = useCallback(
    async (email: string, code: string) => {
      const result = await apiRegisterVerify(email, code);
      persist(result.token, result.user);
      setLoginOpen(false);
    },
    [persist],
  );

  const registerResend = useCallback(async (email: string) => {
    return apiRegisterResend(email);
  }, []);

  const logout = useCallback(() => {
    localStorage.removeItem(TOKEN_KEY);
    setToken(null);
    setUser(null);
  }, []);

  const openLogin = useCallback((mode: 'login' | 'register' = 'login') => {
    setAuthMode(mode);
    setLoginOpen(true);
  }, []);

  const closeLogin = useCallback(() => setLoginOpen(false), []);

  const value = useMemo(
    () => ({
      user,
      token,
      ready,
      loginOpen,
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
    }),
    [
      user,
      token,
      ready,
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
    ],
  );

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>;
}

export function useAuth() {
  const ctx = useContext(AuthContext);
  if (!ctx) throw new Error('useAuth must be used within AuthProvider');
  return ctx;
}
