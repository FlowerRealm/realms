import { useEffect, useMemo, useState } from 'react';
import { useParams } from 'react-router-dom';

import {
  getAdminOAuthApp,
  rotateAdminOAuthAppSecret,
  updateAdminOAuthApp,
  type AdminOAuthApp,
} from '../../api/admin/oauthApps';
import { AutoSaveIndicator } from '../../components/AutoSaveIndicator';
import { SegmentedFrame } from '../../components/SegmentedFrame';
import { useAutoSave } from '../../hooks/useAutoSave';

function parseURIs(raw: string): string[] {
  return raw
    .split('\n')
    .map((s) => s.trim())
    .filter((s) => s);
}

export function OAuthAppDetailPage() {
  const params = useParams();
  const appId = Number.parseInt((params.id || '').toString(), 10);

  const [app, setApp] = useState<AdminOAuthApp | null>(null);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [err, setErr] = useState('');
  const [notice, setNotice] = useState('');
  const [rotatedSecret, setRotatedSecret] = useState('');
  const [autosaveResetKey, setAutosaveResetKey] = useState(0);

  const [name, setName] = useState('');
  const [status, setStatus] = useState(1);
  const [redirectURIsRaw, setRedirectURIsRaw] = useState('');

  async function refresh() {
    setErr('');
    setNotice('');
    setLoading(true);
    try {
      if (!Number.isFinite(appId) || appId <= 0) throw new Error('参数错误');
      const res = await getAdminOAuthApp(appId);
      if (!res.success) throw new Error(res.message || '加载失败');
      const a = res.data || null;
      setApp(a);
      setName(a?.name || '');
      setStatus(a?.status || 0);
      setRedirectURIsRaw((a?.redirect_uris || []).join('\n'));
      setAutosaveResetKey((x) => x + 1);
    } catch (e) {
      setErr(e instanceof Error ? e.message : '加载失败');
      setApp(null);
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    void refresh();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [appId]);

  const autosaveValue = useMemo(() => {
    if (!app) return null;
    return { name: name.trim(), status, redirect_uris: parseURIs(redirectURIsRaw) };
  }, [app, name, redirectURIsRaw, status]);

  const autosave = useAutoSave({
    enabled: !!app && !loading && !saving,
    value: autosaveValue,
    resetKey: autosaveResetKey,
    validate: (v) => {
      if (!app) return '未加载';
      if (!v) return '未加载';
      if (!v.name.trim()) return '名称不能为空';
      return '';
    },
    save: async (v) => {
      if (!app) return;
      if (!v) return;
      setErr('');
      setNotice('');
      setSaving(true);
      try {
        const res = await updateAdminOAuthApp(app.id, v);
        if (!res.success) throw new Error(res.message || '保存失败');
        setNotice(res.message || '已自动保存');
      } finally {
        setSaving(false);
      }
    },
    afterSave: async () => {
      await refresh();
    },
  });

  return (
    <div className="fade-in-up">
      <SegmentedFrame>
        <div>
          <div className="d-flex justify-content-between align-items-center mb-4">
            <div>
              <h3 className="mb-0 fw-bold">OAuth 应用</h3>
              {app ? (
                <div className="text-muted small mt-1">
                  id={app.id} · client_id：<code className="user-select-all">{app.client_id}</code>
                </div>
              ) : null}
            </div>
            <AutoSaveIndicator status={autosave.status} blockedReason={autosave.blockedReason} error={autosave.error} onRetry={autosave.retry} className="small" />
          </div>

          {rotatedSecret ? (
            <div className="alert alert-warning mb-3">
              <div className="fw-bold mb-2">已轮换 client_secret（仅展示一次，请立即保存）</div>
              <div>
                client_secret：<code className="user-select-all">{rotatedSecret}</code>
              </div>
            </div>
          ) : null}

          {notice ? (
            <div className="alert alert-success d-flex align-items-center mb-3" role="alert">
              <span className="me-2 material-symbols-rounded">check_circle</span>
              <div>{notice}</div>
            </div>
          ) : null}

          {err ? (
            <div className="alert alert-danger d-flex align-items-center mb-0" role="alert">
              <span className="me-2 material-symbols-rounded">warning</span>
              <div>{err}</div>
            </div>
          ) : null}
        </div>

        {loading ? (
          <div className="text-muted">加载中…</div>
        ) : !app ? (
          <div className="alert alert-warning mb-0">未找到该 OAuth 应用。</div>
        ) : (
          <div>
            <form
              className="row g-3"
              onSubmit={async (e) => {
                e.preventDefault();
                autosave.flush();
              }}
            >
              <div className="col-md-6">
                <label className="form-label">名称</label>
                <input className="form-control" value={name} onChange={(e) => setName(e.target.value)} />
              </div>
              <div className="col-md-6">
                <label className="form-label">状态</label>
                <select className="form-select" value={status} onChange={(e) => setStatus(Number.parseInt(e.target.value, 10) || 0)}>
                  <option value={1}>启用</option>
                  <option value={0}>停用</option>
                </select>
              </div>
              <div className="col-12">
                <label className="form-label">回调地址白名单（redirect_uri，每行一个，精确匹配）</label>
                <textarea className="form-control" rows={6} value={redirectURIsRaw} onChange={(e) => setRedirectURIsRaw(e.target.value)} />
              </div>
              <div className="col-12 d-grid d-md-flex justify-content-md-between gap-2">
                <button
                  type="button"
                  className="btn btn-outline-warning"
                  onClick={async () => {
                    if (!window.confirm('确认轮换 client_secret？旧的 secret 将立即失效。')) return;
                    setErr('');
                    setNotice('');
                    setRotatedSecret('');
                    try {
                      const res = await rotateAdminOAuthAppSecret(app.id);
                      if (!res.success) throw new Error(res.message || '轮换失败');
                      const sec = res.data?.client_secret || '';
                      setRotatedSecret(sec);
                      setNotice(res.message || '已轮换');
                      await refresh();
                    } catch (e) {
                      setErr(e instanceof Error ? e.message : '轮换失败');
                    }
                  }}
                >
                  生成/轮换 Secret
                </button>
              </div>
            </form>
          </div>
        )}
      </SegmentedFrame>
    </div>
  );
}
