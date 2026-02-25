/* eslint-disable react-refresh/only-export-components */
import { createContext, useCallback, useContext, useEffect, useMemo, useState } from 'react';
import type { ReactNode } from 'react';

import { api } from '../api/client';
import type { APIResponse, User } from '../api/types';

type AuthState = {
  user: User | null;
  loading: boolean;
  selfMode: boolean;
  selfModeKeySet: boolean;
  refresh: () => Promise<void>;
  login: (login: string, password: string) => Promise<void>;
  register: (email: string, username: string, password: string, verificationCode?: string) => Promise<void>;
  logout: () => Promise<void>;
};

const AuthContext = createContext<AuthState | null>(null);

export function AuthProvider({ children }: { children: ReactNode }) {
  const [user, setUser] = useState<User | null>(null);
  const [loading, setLoading] = useState(true);
  const [selfMode, setSelfMode] = useState(false);
  const [selfModeKeySet, setSelfModeKeySet] = useState(false);
  const [metaLoaded, setMetaLoaded] = useState(false);

  const refresh = useCallback(async () => {
    setLoading(true);
    try {
      const res = await api.get<APIResponse<User>>('/api/user/self');
      if (res.data?.success && res.data.data) {
        setUser(res.data.data);
        localStorage.setItem('user', JSON.stringify(res.data.data));
      } else {
        setUser(null);
        localStorage.removeItem('user');
      }
    } finally {
      setLoading(false);
    }
  }, []);

  const login = useCallback(
    async (login: string, password: string) => {
      if (selfMode) {
        const key = (login || '').trim();
        if (!key) {
          throw new Error('Key 不能为空');
        }
        setLoading(true);
        try {
          if (!selfModeKeySet) {
            const res = await api.post<APIResponse<unknown>>('/api/self-mode/bootstrap', { key });
            if (!res.data?.success) {
              throw new Error(res.data?.message || '设置 Key 失败');
            }
            setSelfModeKeySet(true);
          }

          localStorage.setItem('self_mode_key', key);
          try {
            await refresh();
            if (!localStorage.getItem('user')) {
              throw new Error('Key 无效');
            }
            return;
          } catch (e) {
            localStorage.removeItem('self_mode_key');
            throw e;
          }
        } finally {
          setLoading(false);
        }
      }
      setLoading(true);
      try {
        const res = await api.post<APIResponse<User>>('/api/user/login', {
          login,
          password,
        });
        if (!res.data?.success) {
          throw new Error(res.data?.message || '登录失败');
        }
        await refresh();
      } finally {
        setLoading(false);
      }
    },
    [refresh, selfMode],
  );

  const register = useCallback(
    async (email: string, username: string, password: string, verificationCode?: string) => {
      if (selfMode) {
        throw new Error('自用模式不支持注册');
      }
      setLoading(true);
      try {
        const res = await api.post<APIResponse<User>>('/api/user/register', {
          email,
          username,
          password,
          verification_code: verificationCode || undefined,
        });
        if (!res.data?.success) {
          throw new Error(res.data?.message || '注册失败');
        }
        await refresh();
      } finally {
        setLoading(false);
      }
    },
    [refresh, selfMode],
  );

  const logout = useCallback(async () => {
    if (selfMode) {
      localStorage.removeItem('self_mode_key');
      setUser(null);
      localStorage.removeItem('user');
      return;
    }
    setLoading(true);
    try {
      await api.get('/api/user/logout');
      setUser(null);
      localStorage.removeItem('user');
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    let mounted = true;
    (async () => {
      try {
        const res = await api.get<APIResponse<{ self_mode?: boolean; self_mode_key_set?: boolean }>>('/api/meta');
        if (mounted && res.data?.success) {
          setSelfMode(!!res.data?.data?.self_mode);
          setSelfModeKeySet(!!res.data?.data?.self_mode_key_set);
        }
      } catch {
        // ignore
      } finally {
        if (mounted) setMetaLoaded(true);
      }
    })();
    return () => {
      mounted = false;
    };
  }, []);

  useEffect(() => {
    if (!metaLoaded) return;
    void refresh();
  }, [metaLoaded, refresh]);

  const value = useMemo<AuthState>(
    () => ({
      user,
      loading,
      selfMode,
      selfModeKeySet,
      refresh,
      login,
      register,
      logout,
    }),
    [loading, login, logout, refresh, register, selfMode, selfModeKeySet, user],
  );

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>;
}

export function useAuth() {
  const ctx = useContext(AuthContext);
  if (!ctx) {
    throw new Error('useAuth must be used within <AuthProvider />');
  }
  return ctx;
}
