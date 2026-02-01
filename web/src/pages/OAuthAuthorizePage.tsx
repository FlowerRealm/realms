import { useEffect, useMemo, useState } from 'react';
import { useLocation } from 'react-router-dom';

import { api } from '../api/client';
import type { APIResponse } from '../api/types';

type OAuthAuthorizePrepare = {
  app_name: string;
  client_id: string;
  redirect_uri: string;
  scope: string;
  state: string;
  code_challenge?: string;
  code_challenge_method?: string;
  redirect_to?: string;
};

export function OAuthAuthorizePage() {
  const location = useLocation();

  const [loading, setLoading] = useState(true);
  const [err, setErr] = useState('');
  const [data, setData] = useState<OAuthAuthorizePrepare | null>(null);
  const [remember, setRemember] = useState(true);

  const query = useMemo(() => new URLSearchParams(location.search), [location.search]);

  useEffect(() => {
    let cancelled = false;
    async function run() {
      setLoading(true);
      setErr('');
      try {
        const res = await api.get<APIResponse<OAuthAuthorizePrepare>>(`/api/oauth/authorize${location.search}`);
        if (!res.data?.success) {
          throw new Error(res.data?.message || '请求失败');
        }
        const payload = res.data.data;
        if (!payload) {
          throw new Error('响应为空');
        }
        if (payload.redirect_to) {
          window.location.href = payload.redirect_to;
          return;
        }
        if (!cancelled) {
          setData(payload);
        }
      } catch (e) {
        if (!cancelled) {
          setErr(e instanceof Error ? e.message : '请求失败');
        }
      } finally {
        if (!cancelled) {
          setLoading(false);
        }
      }
    }
    void run();
    return () => {
      cancelled = true;
    };
  }, [location.search]);

  const clientID = (data?.client_id || query.get('client_id') || '').trim();
  const redirectURI = (data?.redirect_uri || query.get('redirect_uri') || '').trim();
  const scope = (data?.scope || query.get('scope') || '').trim();
  const state = (data?.state || query.get('state') || '').trim();
  const codeChallenge = (data?.code_challenge || query.get('code_challenge') || '').trim();
  const codeChallengeMethod = (data?.code_challenge_method || query.get('code_challenge_method') || '').trim();

  async function decide(decision: 'approve' | 'deny') {
    setErr('');
    setLoading(true);
    try {
      const res = await api.post<APIResponse<{ redirect_to: string }>>('/api/oauth/authorize', {
        client_id: clientID,
        redirect_uri: redirectURI,
        scope,
        state,
        decision,
        remember: decision === 'approve' ? remember : false,
        code_challenge: codeChallenge || undefined,
        code_challenge_method: codeChallengeMethod || undefined,
      });
      if (!res.data?.success) {
        throw new Error(res.data?.message || '操作失败');
      }
      const redirectTo = res.data?.data?.redirect_to;
      if (!redirectTo) {
        throw new Error('缺少 redirect_to');
      }
      window.location.href = redirectTo;
    } catch (e) {
      setErr(e instanceof Error ? e.message : '操作失败');
      setLoading(false);
    }
  }

  if (loading && !data && !err) {
    return (
      <div className="container-fluid d-flex flex-column min-vh-100 p-0">
        <main className="flex-fill d-flex flex-column justify-content-center align-items-center">
          <div className="card border-0" style={{ width: '100%', maxWidth: 560 }}>
            <div className="card-body p-4 text-center text-muted">加载中…</div>
          </div>
        </main>
      </div>
    );
  }

  return (
    <div className="container-fluid d-flex flex-column min-vh-100 p-0">
      <main className="flex-fill d-flex flex-column justify-content-center align-items-center">
        <div className="card border-0" style={{ width: '100%', maxWidth: 560 }}>
          <div className="card-body p-4">
            <h2 className="h5 fw-semibold mb-2">应用授权</h2>
            <div className="text-muted small mb-3">第三方应用正在请求访问你的 Realms 账号。</div>

            {err ? (
              <div className="alert alert-danger py-2" role="alert">
                <span className="me-1 material-symbols-rounded">warning</span> {err}
              </div>
            ) : null}

            <div className="border rounded p-3 mb-3 bg-light">
              <div className="small text-muted mb-1">应用</div>
              <div className="fw-semibold">{data?.app_name || '未知应用'}</div>
              <div className="small text-muted mt-2">请求权限</div>
              <div className="font-monospace small">{scope || '(empty)'}</div>
            </div>

            <div className="form-check mb-3">
              <input
                id="remember"
                className="form-check-input"
                type="checkbox"
                checked={remember}
                onChange={(e) => setRemember(e.target.checked)}
                disabled={loading}
              />
              <label className="form-check-label" htmlFor="remember">
                记住此次授权（下次同权限将自动通过）
              </label>
            </div>

            <div className="d-flex gap-2">
              <button type="button" className="btn btn-primary flex-fill" disabled={loading} onClick={() => void decide('approve')}>
                {loading ? '处理中…' : '授权'}
              </button>
              <button type="button" className="btn btn-outline-secondary flex-fill" disabled={loading} onClick={() => void decide('deny')}>
                拒绝
              </button>
            </div>

            <details className="mt-3">
              <summary className="small text-muted">显示详细信息</summary>
              <div className="small text-muted mt-2">client_id</div>
              <div className="font-monospace small">{clientID || '-'}</div>
              <div className="small text-muted mt-2">redirect_uri</div>
              <div className="font-monospace small">{redirectURI || '-'}</div>
            </details>
          </div>
        </div>
      </main>
    </div>
  );
}

