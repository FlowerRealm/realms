/* eslint-disable react-refresh/only-export-components */
import { createContext, useCallback, useContext, useEffect, useMemo, useState } from 'react';
import type { ReactNode } from 'react';

import { api } from '../api/client';
import type { APIResponse, User } from '../api/types';

type AuthState = {
  user: User | null;
  loading: boolean;
  refresh: () => Promise<void>;
  login: (login: string, password: string) => Promise<void>;
  register: (email: string, username: string, password: string, verificationCode?: string) => Promise<void>;
  logout: () => Promise<void>;
};

const AuthContext = createContext<AuthState | null>(null);

export function AuthProvider({ children }: { children: ReactNode }) {
  const [user, setUser] = useState<User | null>(null);
  const [loading, setLoading] = useState(true);

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
    [refresh],
  );

  const register = useCallback(
    async (email: string, username: string, password: string, verificationCode?: string) => {
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
    [refresh],
  );

  const logout = useCallback(async () => {
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
    void refresh();
  }, [refresh]);

  const value = useMemo<AuthState>(
    () => ({
      user,
      loading,
      refresh,
      login,
      register,
      logout,
    }),
    [loading, login, logout, refresh, register, user],
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
