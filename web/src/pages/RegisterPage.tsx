import { useState } from 'react';
import { Link, Navigate, useNavigate, useOutletContext } from 'react-router-dom';

import { useAuth } from '../auth/AuthContext';
import type { PublicLayoutContext } from '../layout/PublicLayout';

export function RegisterPage() {
  const { user, register, loading } = useAuth();
  const navigate = useNavigate();
  const { allowOpenRegistration, emailVerificationEnabled } = useOutletContext<PublicLayoutContext>();

  const [form, setForm] = useState({
    email: '',
    username: '',
    password: '',
    verificationCode: '',
  });
  const [err, setErr] = useState('');
  const [notice, setNotice] = useState('');
  const [sendingCode, setSendingCode] = useState(false);

  if (user) {
    return <Navigate to="/dashboard" replace />;
  }

  return (
    <div className="card border-0">
      <div className="card-body p-4">
        <h2 className="h4 card-title text-center mb-4">注册账号</h2>

        {!allowOpenRegistration ? (
          <div className="alert alert-warning py-2" role="alert">
            <span className="me-1 material-symbols-rounded">info</span> 当前环境未开放注册。
          </div>
        ) : null}

        {err ? (
          <div className="alert alert-danger py-2" role="alert">
            <span className="me-1 material-symbols-rounded">warning</span> {err}
          </div>
        ) : null}

        {notice ? (
          <div className="alert alert-success py-2" role="alert">
            <span className="me-1 material-symbols-rounded">check_circle</span> {notice}
          </div>
        ) : null}

        <form
          onSubmit={async (e) => {
            e.preventDefault();
            setErr('');
            setNotice('');
            try {
              await register(form.email.trim(), form.username.trim(), form.password, form.verificationCode.trim() || undefined);
              navigate('/dashboard', { replace: true });
            } catch (e) {
              setErr(e instanceof Error ? e.message : '注册失败');
            }
          }}
        >
          <div className="mb-3">
            <label className="form-label">邮箱</label>
            <input
              name="email"
              type="email"
              className="form-control"
              autoComplete="email"
              required
              placeholder="name@example.com"
              value={form.email}
              onChange={(e) => setForm((p) => ({ ...p, email: e.target.value }))}
              disabled={!allowOpenRegistration}
            />
          </div>

          <div className="mb-3">
            <label className="form-label">账号名</label>
            <input
              name="username"
              type="text"
              className="form-control"
              autoComplete="username"
              required
              placeholder="例如：alice"
              value={form.username}
              onChange={(e) => setForm((p) => ({ ...p, username: e.target.value }))}
              disabled={!allowOpenRegistration}
            />
            <div className="form-text">支持字母/数字及 . _ -，最多 32 位；用于登录。</div>
          </div>

          {emailVerificationEnabled ? (
            <div className="mb-3">
              <label className="form-label">验证码</label>
              <div className="input-group">
                <input
                  name="verification_code"
                  type="text"
                  className="form-control"
                  autoComplete="one-time-code"
                  inputMode="numeric"
                  maxLength={6}
                  placeholder="6 位验证码"
                  value={form.verificationCode}
                  onChange={(e) => setForm((p) => ({ ...p, verificationCode: e.target.value }))}
                  disabled={!allowOpenRegistration}
                />
                <button
                  type="button"
                  className="btn btn-outline-secondary"
                  disabled={!allowOpenRegistration || sendingCode}
                  onClick={async () => {
                    setErr('');
                    setNotice('');
                    const email = form.email.trim().toLowerCase();
                    if (!email) {
                      setErr('请先填写邮箱。');
                      return;
                    }
                    setSendingCode(true);
                    try {
                      const resp = await fetch('/api/email/verification/send', {
                        method: 'POST',
                        headers: { 'Content-Type': 'application/x-www-form-urlencoded; charset=UTF-8' },
                        body: new URLSearchParams({ email }),
                      });
                      if (!resp.ok) {
                        const txt = await resp.text();
                        setErr((txt || '').trim() || '发送失败，请稍后重试。');
                        return;
                      }
                      setNotice('验证码已发送，请查收邮箱（10 分钟内有效）。');
                    } catch {
                      setErr('发送失败，请稍后重试。');
                    } finally {
                      setSendingCode(false);
                    }
                  }}
                >
                  {sendingCode ? '发送中…' : '发送验证码'}
                </button>
              </div>
              <div className="form-text">验证码 10 分钟内有效。</div>
            </div>
          ) : null}

          <div className="mb-3">
            <label className="form-label">密码</label>
            <input
              name="password"
              type="password"
              className="form-control"
              autoComplete="new-password"
              required
              placeholder="至少 8 位字符"
              value={form.password}
              onChange={(e) => setForm((p) => ({ ...p, password: e.target.value }))}
              disabled={!allowOpenRegistration}
            />
            <div className="form-text">密码将通过 bcrypt 加密存储。</div>
          </div>

          <div className="d-grid mt-4">
            <button type="submit" className="btn btn-primary" disabled={!allowOpenRegistration || loading}>
              {loading ? '提交中…' : '创建账号'}
            </button>
          </div>
        </form>
      </div>

      <div className="card-footer bg-transparent text-center py-3">
        <span className="text-muted small">已有账号？</span> <Link to="/login" className="text-decoration-none">直接登录</Link>
      </div>
    </div>
  );
}
