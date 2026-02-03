import { useMemo, useState } from 'react';
import { useNavigate } from 'react-router-dom';

import { useAuth } from '../auth/AuthContext';
import { updateEmail, updatePassword } from '../api/account';

function normalizeEmail(v: string): string {
  return (v || '').trim().toLowerCase();
}

export function AccountPage() {
  const { user, logout, refresh } = useAuth();
  const navigate = useNavigate();

  const emailVerificationEnabled = !!user?.email_verification_enabled;

  const [err, setErr] = useState('');
  const [notice, setNotice] = useState('');

  const [newEmail, setNewEmail] = useState('');
  const [verificationCode, setVerificationCode] = useState('');
  const [sendingCode, setSendingCode] = useState(false);
  const [emailHint, setEmailHint] = useState('验证码 10 分钟内有效。');

  const [oldPassword, setOldPassword] = useState('');
  const [newPassword, setNewPassword] = useState('');

  const displayEmail = useMemo(() => (user?.email || '').toString(), [user?.email]);

  async function forceLogout(msg: string) {
    try {
      await logout();
    } catch {
      // ignore
    }
    try {
      await refresh();
    } catch {
      // ignore
    }
    navigate('/login', { replace: true, state: { notice: msg || '请重新登录' } });
  }

  return (
    <div className="fade-in-up">
      <div className="row g-4">
        <div className="col-12">
          <div className="card">
            <div className="card-body d-flex flex-column flex-md-row justify-content-between align-items-center">
              <div className="d-flex align-items-center mb-3 mb-md-0">
                <div
                  className="bg-primary bg-opacity-10 text-primary rounded-circle d-flex align-items-center justify-content-center me-3"
                  style={{ width: 48, height: 48 }}
                >
                  <span className="fs-4 material-symbols-rounded">manage_accounts</span>
                </div>
                <div>
                  <h5 className="mb-1 fw-semibold">账号设置</h5>
                  <p className="mb-0 text-muted small">修改邮箱/密码成功后将强制登出，需要重新登录。</p>
                </div>
              </div>
            </div>
          </div>
        </div>

        {notice ? (
          <div className="col-12">
            <div className="alert alert-success py-2 mb-0" role="alert">
              <span className="me-1 material-symbols-rounded">check_circle</span> {notice}
            </div>
          </div>
        ) : null}

        {err ? (
          <div className="col-12">
            <div className="alert alert-danger py-2 mb-0" role="alert">
              <span className="me-1 material-symbols-rounded">warning</span> {err}
            </div>
          </div>
        ) : null}

        <div className="col-lg-6">
          <div className="card h-100">
            <div className="card-body">
              <h5 className="fw-semibold mb-3">
                <span className="me-2 text-primary material-symbols-rounded">alternate_email</span>账号名
              </h5>
              <div className="mb-3">
                <label className="form-label">账号名</label>
                <input
                  name="username"
                  type="text"
                  className="form-control"
                  autoComplete="username"
                  disabled
                  value={(user?.username || '').toString()}
                />
                <div className="form-text">账号名不可修改；用于登录（区分大小写，仅字母/数字）。</div>
              </div>
            </div>
          </div>
        </div>

        <div className="col-lg-6">
          <div className="card h-100">
            <div className="card-body">
              <h5 className="fw-semibold mb-3">
                <span className="me-2 text-primary material-symbols-rounded">mail</span>邮箱
              </h5>
              <div className="mb-2 small text-muted">
                当前邮箱：<strong className="text-dark">{displayEmail || '-'}</strong>
              </div>

              {!emailVerificationEnabled ? (
                <div className="alert alert-warning small">
                  <span className="me-1 material-symbols-rounded">gpp_maybe</span> 当前环境未启用邮箱验证码，无法修改邮箱。
                </div>
              ) : null}

              <form
                onSubmit={async (e) => {
                  e.preventDefault();
                  setErr('');
                  setNotice('');
                  try {
                    if (!emailVerificationEnabled) {
                      throw new Error('当前环境未启用邮箱验证码，无法修改邮箱');
                    }
                    const res = await updateEmail(normalizeEmail(newEmail), (verificationCode || '').trim());
                    if (!res.success) throw new Error(res.message || '保存失败');
                    if (res.data?.force_logout) {
                      await forceLogout(res.message || '邮箱已更新，请重新登录');
                      return;
                    }
                    setNotice('已保存');
                  } catch (e) {
                    setErr(e instanceof Error ? e.message : '保存失败');
                  }
                }}
              >
                <div className="mb-3">
                  <label className="form-label">新邮箱</label>
                  <input
                    name="email"
                    type="email"
                    className="form-control"
                    autoComplete="email"
                    placeholder="name@example.com"
                    disabled={!emailVerificationEnabled}
                    value={newEmail}
                    onChange={(e) => setNewEmail(e.target.value)}
                  />
                </div>
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
                      disabled={!emailVerificationEnabled}
                      value={verificationCode}
                      onChange={(e) => setVerificationCode(e.target.value)}
                    />
                    <button
                      type="button"
                      className="btn btn-outline-secondary"
                      disabled={!emailVerificationEnabled || sendingCode}
                      onClick={async () => {
                        setErr('');
                        setNotice('');
                        const email = normalizeEmail(newEmail);
                        if (!email) {
                          setEmailHint('请先填写新邮箱。');
                          return;
                        }
                        setSendingCode(true);
                        setEmailHint('正在发送…');
                        try {
                          const resp = await fetch('/api/email/verification/send', {
                            method: 'POST',
                            headers: { 'Content-Type': 'application/x-www-form-urlencoded; charset=UTF-8' },
                            body: new URLSearchParams({ email }),
                          });
                          if (!resp.ok) {
                            const txt = (await resp.text()).trim();
                            setEmailHint(txt || '发送失败，请稍后重试。');
                            return;
                          }
                          setEmailHint('验证码已发送，请查收邮箱（10 分钟内有效）。');
                        } catch {
                          setEmailHint('发送失败，请稍后重试。');
                        } finally {
                          setSendingCode(false);
                        }
                      }}
                    >
                      {sendingCode ? '发送中…' : '发送验证码'}
                    </button>
                  </div>
                  <div className="form-text" id="account-email-hint">
                    {emailHint}
                  </div>
                </div>
                <button type="submit" className="btn btn-primary" disabled={!emailVerificationEnabled}>
                  保存并重新登录
                </button>
              </form>
            </div>
          </div>
        </div>

        <div className="col-lg-6">
          <div className="card h-100">
            <div className="card-body">
              <h5 className="fw-semibold mb-3">
                <span className="me-2 text-primary material-symbols-rounded">key</span>修改密码
              </h5>
              <form
                onSubmit={async (e) => {
                  e.preventDefault();
                  setErr('');
                  setNotice('');
                  try {
                    const res = await updatePassword(oldPassword, newPassword);
                    if (!res.success) throw new Error(res.message || '保存失败');
                    if (res.data?.force_logout) {
                      await forceLogout(res.message || '密码已更新，请重新登录');
                      return;
                    }
                    setNotice('已保存');
                    setOldPassword('');
                    setNewPassword('');
                  } catch (e) {
                    setErr(e instanceof Error ? e.message : '保存失败');
                  }
                }}
              >
                <div className="mb-3">
                  <label className="form-label">旧密码</label>
                  <input
                    name="old_password"
                    type="password"
                    className="form-control"
                    autoComplete="current-password"
                    placeholder="******"
                    required
                    value={oldPassword}
                    onChange={(e) => setOldPassword(e.target.value)}
                  />
                </div>
                <div className="mb-3">
                  <label className="form-label">新密码</label>
                  <input
                    name="new_password"
                    type="password"
                    className="form-control"
                    autoComplete="new-password"
                    placeholder="至少 8 位字符"
                    required
                    value={newPassword}
                    onChange={(e) => setNewPassword(e.target.value)}
                  />
                  <div className="form-text">修改成功后会强制登出所有已登录会话。</div>
                </div>
                <button type="submit" className="btn btn-primary">
                  保存并重新登录
                </button>
              </form>
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}
