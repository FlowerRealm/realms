import { useEffect, useMemo, useState } from 'react';
import { Link } from 'react-router-dom';

import { BootstrapModal } from '../../components/BootstrapModal';
import { closeModalById } from '../../components/modal';
import { createAdminOAuthApp, listAdminOAuthApps, type AdminOAuthApp } from '../../api/admin/oauthApps';

function statusBadge(status: number): string {
  if (status === 1) return 'badge rounded-pill bg-success bg-opacity-10 text-success px-2';
  return 'badge rounded-pill bg-secondary bg-opacity-10 text-secondary px-2';
}

function parseURIs(raw: string): string[] {
  return raw
    .split('\n')
    .map((s) => s.trim())
    .filter((s) => s);
}

export function OAuthAppsAdminPage() {
  const [items, setItems] = useState<AdminOAuthApp[]>([]);
  const [loading, setLoading] = useState(true);
  const [err, setErr] = useState('');
  const [notice, setNotice] = useState('');

  const [name, setName] = useState('');
  const [status, setStatus] = useState(1);
  const [redirectURIsRaw, setRedirectURIsRaw] = useState('');

  const [createdClientID, setCreatedClientID] = useState('');
  const [createdClientSecret, setCreatedClientSecret] = useState('');

  const enabledCount = useMemo(() => items.filter((a) => a.status === 1).length, [items]);

  async function refresh() {
    setErr('');
    setNotice('');
    setLoading(true);
    try {
      const res = await listAdminOAuthApps();
      if (!res.success) throw new Error(res.message || '加载失败');
      setItems(res.data || []);
    } catch (e) {
      setErr(e instanceof Error ? e.message : '加载失败');
      setItems([]);
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    void refresh();
  }, []);

  return (
    <div className="fade-in-up">
      <div className="row g-4">
        <div className="col-12">
          <div className="card">
            <div className="card-body d-flex flex-column flex-md-row justify-content-between align-items-center">
              <div className="d-flex align-items-center mb-3 mb-md-0">
                <div
                  className="bg-warning bg-opacity-10 text-warning rounded-circle d-flex align-items-center justify-content-center me-3"
                  style={{ width: 48, height: 48 }}
                >
                  <span className="fs-4 material-symbols-rounded">apps</span>
                </div>
                <div>
                  <h5 className="mb-1 fw-semibold">OAuth 应用</h5>
                  <p className="mb-0 text-muted small">
                    {enabledCount} 启用 / {items.length} 总计
                  </p>
                </div>
              </div>

              <div className="d-flex gap-2">
                <button type="button" className="btn btn-primary btn-sm" data-bs-toggle="modal" data-bs-target="#createOAuthAppModal">
                  <span className="material-symbols-rounded me-1">add</span> 新建应用
                </button>
              </div>
            </div>
          </div>
        </div>

        {createdClientSecret ? (
          <div className="col-12">
            <div className="alert alert-warning">
              <div className="fw-bold mb-2">已创建 OAuth 应用（client_secret 仅展示一次，请立即保存）</div>
              <div className="mb-1">
                client_id：<code className="user-select-all">{createdClientID}</code>
              </div>
              <div>
                client_secret：<code className="user-select-all">{createdClientSecret}</code>
              </div>
            </div>
          </div>
        ) : null}

        {notice ? (
          <div className="col-12">
            <div className="alert alert-success d-flex align-items-center" role="alert">
              <span className="me-2 material-symbols-rounded">check_circle</span>
              <div>{notice}</div>
            </div>
          </div>
        ) : null}

        {err ? (
          <div className="col-12">
            <div className="alert alert-danger d-flex align-items-center" role="alert">
              <span className="me-2 material-symbols-rounded">warning</span>
              <div>{err}</div>
            </div>
          </div>
        ) : null}

        <div className="col-12">
          {loading ? (
            <div className="text-muted">加载中…</div>
          ) : items.length === 0 ? (
            <div className="text-center py-5 text-muted">
              <span className="fs-1 d-block mb-3 material-symbols-rounded">inbox</span>
              暂无 OAuth 应用。
            </div>
          ) : (
            <div className="card overflow-hidden mb-0">
              <div className="table-responsive">
                <table className="table table-hover align-middle mb-0">
                  <thead className="table-light">
                    <tr>
                      <th className="ps-4">应用</th>
                      <th>状态</th>
                      <th>回调地址</th>
                      <th className="text-end pe-4">操作</th>
                    </tr>
                  </thead>
                  <tbody>
                    {items.map((a) => (
                      <tr key={a.id}>
                        <td className="ps-4" style={{ minWidth: 0 }}>
                          <div className="fw-bold text-dark">{a.name}</div>
                          <div className="text-muted small">
                            client_id：<code className="user-select-all">{a.client_id}</code>
                          </div>
                        </td>
                        <td>
                          <span className={statusBadge(a.status)}>{a.status_label}</span>
                          {!a.has_secret ? <div className="text-warning small mt-1">未设置 secret</div> : null}
                        </td>
                        <td className="text-muted small">
                          {a.redirect_uris?.length ? (
                            <ul className="mb-0 ps-3">
                              {a.redirect_uris.slice(0, 3).map((u) => (
                                <li key={u} className="text-truncate" style={{ maxWidth: 520 }}>
                                  <code className="user-select-all">{u}</code>
                                </li>
                              ))}
                              {a.redirect_uris.length > 3 ? <li>…</li> : null}
                            </ul>
                          ) : (
                            <span className="text-muted fst-italic">-</span>
                          )}
                        </td>
                        <td className="text-end pe-4 text-nowrap">
                          <Link className="btn btn-sm btn-light border text-primary" to={`/admin/oauth-apps/${a.id}`} title="管理">
                            <i className="ri-settings-3-line"></i>
                          </Link>
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            </div>
          )}
        </div>
      </div>

      <BootstrapModal
        id="createOAuthAppModal"
        title="新建 OAuth 应用"
        dialogClassName="modal-dialog-centered modal-lg modal-dialog-scrollable"
        onHidden={() => {
          setErr('');
          setName('');
          setStatus(1);
          setRedirectURIsRaw('');
        }}
      >
        <form
          className="row g-3"
          onSubmit={async (e) => {
            e.preventDefault();
            setErr('');
            setNotice('');
            setCreatedClientID('');
            setCreatedClientSecret('');
            try {
              const res = await createAdminOAuthApp({
                name: name.trim(),
                status,
                redirect_uris: parseURIs(redirectURIsRaw),
              });
              if (!res.success) throw new Error(res.message || '创建失败');
              const cid = res.data?.client_id || '';
              const sec = res.data?.client_secret || '';
              setCreatedClientID(cid);
              setCreatedClientSecret(sec);
              setNotice('已创建');
              closeModalById('createOAuthAppModal');
              await refresh();
            } catch (e) {
              setErr(e instanceof Error ? e.message : '创建失败');
            }
          }}
        >
          <div className="col-md-6">
            <label className="form-label">应用名称</label>
            <input className="form-control" value={name} onChange={(e) => setName(e.target.value)} placeholder="例如：My Chat Client" required />
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
            <textarea className="form-control font-monospace" rows={8} value={redirectURIsRaw} onChange={(e) => setRedirectURIsRaw(e.target.value)} placeholder="https://example.com/oauth/callback" required />
          </div>

          <div className="modal-footer border-top-0 px-0 pb-0">
            <button type="button" className="btn btn-light" data-bs-dismiss="modal">
              取消
            </button>
            <button className="btn btn-primary px-4" type="submit" disabled={loading}>
              创建
            </button>
          </div>
        </form>
      </BootstrapModal>
    </div>
  );
}
