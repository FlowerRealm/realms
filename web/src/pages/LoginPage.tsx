import { useMemo, useState } from 'react';
import { Link, Navigate, useLocation, useNavigate, useOutletContext } from 'react-router-dom';

import { useAuth } from '../auth/AuthContext';
import { SegmentedFrame } from '../components/SegmentedFrame';
import type { PublicLayoutContext } from '../layout/PublicLayout';

type LocationState = {
  from?: string;
  notice?: string;
  error?: string;
};

export function LoginPage() {
  const { user, login, loading, selfMode, selfModeKeySet } = useAuth();
  const navigate = useNavigate();
  const location = useLocation();
  const { allowOpenRegistration } = useOutletContext<PublicLayoutContext>();

  const [form, setForm] = useState({ login: '', password: '' });
  const [err, setErr] = useState('');

  const notice = useMemo(() => {
    const state = location.state as LocationState | null;
    const v = (state?.notice || '').toString().trim();
    return v ? v : '';
  }, [location.state, selfMode]);

  const from = useMemo(() => {
    const state = location.state as LocationState | null;
    const next = (state?.from || '').toString().trim();
    if (next) {
      return next;
    }
    return selfMode ? '/admin' : '/dashboard';
  }, [location.state]);

  if (user) {
    return <Navigate to={user.self_mode ? '/admin' : '/dashboard'} replace />;
  }

  return (
    <SegmentedFrame>
      <div className="card border-0 mb-0">
        <div className="card-body p-4">
          <h2 className="h4 card-title text-center mb-4">
            {selfMode ? (selfModeKeySet ? '解锁 Realms' : '初始化 Realms') : '登录 Realms'}
          </h2>

          {notice ? (
            <div className="alert alert-success py-2" role="alert">
              <span className="me-1 material-symbols-rounded">check_circle</span> {notice}
            </div>
          ) : null}

          {err ? (
            <div className="alert alert-danger py-2" role="alert">
              <span className="me-1 material-symbols-rounded">warning</span> {err}
            </div>
          ) : null}

          {selfMode ? (
            <form
              onSubmit={async (e) => {
                e.preventDefault();
                setErr('');
                try {
                  const key = (form.login || '').trim();
                  if (!key) {
                    setErr('Key 不能为空');
                    return;
                  }
                  if (!selfModeKeySet) {
                    const confirm = (form.password || '').trim();
                    if (!confirm) {
                      setErr('请再次输入 Key 确认');
                      return;
                    }
                    if (confirm !== key) {
                      setErr('两次输入的 Key 不一致');
                      return;
                    }
                  }

                  await login(key, '');
                  navigate(from, { replace: true });
                } catch (e) {
                  setErr(e instanceof Error ? e.message : '解锁失败');
                }
              }}
            >
              <div className="mb-3">
                <label className="form-label">{selfModeKeySet ? '管理 Key' : '设置管理 Key'}</label>
                <input
                  className="form-control"
                  name="key"
                  type="password"
                  autoComplete="off"
                  required
                  placeholder={selfModeKeySet ? '输入你设置的 Key' : '输入一个新的 Key'}
                  value={form.login}
                  onChange={(e) => setForm((p) => ({ ...p, login: e.target.value }))}
                />
                <div className="form-text">自用模式下使用 Key 作为鉴权，不需要账号系统。</div>
              </div>

              {!selfModeKeySet ? (
                <div className="mb-3">
                  <label className="form-label">确认 Key</label>
                  <input
                    className="form-control"
                    name="key_confirm"
                    type="password"
                    autoComplete="off"
                    required
                    placeholder="再次输入 Key"
                    value={form.password}
                    onChange={(e) => setForm((p) => ({ ...p, password: e.target.value }))}
                  />
                </div>
              ) : null}

              <div className="d-grid mt-4">
                <button type="submit" className="btn btn-primary" disabled={loading}>
                  {loading ? (selfModeKeySet ? '解锁中…' : '初始化中…') : selfModeKeySet ? '进入管理后台' : '完成初始化'}
                </button>
              </div>
            </form>
          ) : (
            <form
              onSubmit={async (e) => {
                e.preventDefault();
                setErr('');
                try {
                  await login(form.login, form.password);
                  navigate(from, { replace: true });
                } catch (e) {
                  setErr(e instanceof Error ? e.message : '登录失败');
                }
              }}
            >
              <div className="mb-3">
                <label className="form-label">邮箱或账号名</label>
                <input
                  className="form-control"
                  name="login"
                  type="text"
                  autoComplete="username"
                  required
                  placeholder="name@example.com 或 alice"
                  value={form.login}
                  onChange={(e) => setForm((p) => ({ ...p, login: e.target.value }))}
                />
              </div>

              <div className="mb-3">
                <label className="form-label">密码</label>
                <input
                  className="form-control"
                  name="password"
                  type="password"
                  autoComplete="current-password"
                  required
                  placeholder="******"
                  value={form.password}
                  onChange={(e) => setForm((p) => ({ ...p, password: e.target.value }))}
                />
              </div>

              <div className="d-grid mt-4">
                <button type="submit" className="btn btn-primary" disabled={loading}>
                  {loading ? '登录中…' : '立即登录'}
                </button>
              </div>
            </form>
          )}
        </div>

        {!selfMode && allowOpenRegistration ? (
          <div className="card-footer bg-transparent text-center py-3">
            <span className="text-muted small">还没有账号？</span>{' '}
            <Link to="/register" className="text-decoration-none">
              注册新账号
            </Link>
          </div>
        ) : null}
      </div>
    </SegmentedFrame>
  );
}
