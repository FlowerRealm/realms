import { useEffect, useMemo, useState } from 'react';

import { createPersonalAPIKey, listPersonalAPIKeys, revokePersonalAPIKey, type PersonalAPIKey } from '../../api/personalKeys';
import { BootstrapModal } from '../../components/BootstrapModal';
import { closeModalById } from '../../components/modal';
import { formatLocalDateTimeMinute } from '../usage/usageUtils';

type CreateState =
  | { phase: 'idle' }
  | { phase: 'creating' }
  | { phase: 'created'; key: string; copied: boolean };

function statusBadge(status: number) {
  const on = status === 1;
  return on ? 'badge rounded-pill bg-success-subtle text-success border border-success-subtle' : 'badge rounded-pill bg-secondary-subtle text-secondary border border-secondary-subtle';
}

export function PersonalAPIKeysPanel() {
  const [keys, setKeys] = useState<PersonalAPIKey[]>([]);
  const [loading, setLoading] = useState(true);
  const [err, setErr] = useState('');
  const [notice, setNotice] = useState('');

  const [name, setName] = useState('');
  const [createState, setCreateState] = useState<CreateState>({ phase: 'idle' });

  const activeCount = useMemo(() => keys.filter((k) => k.status === 1).length, [keys]);

  async function refresh() {
    setErr('');
    setNotice('');
    setLoading(true);
    try {
      const res = await listPersonalAPIKeys();
      if (!res.success) throw new Error(res.message || '加载失败');
      setKeys(res.data || []);
    } catch (e) {
      setErr(e instanceof Error ? e.message : '加载失败');
      setKeys([]);
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    void refresh();
  }, []);

  return (
    <div className="row g-4">
      <div className="col-12">
        <div className="card">
          <div className="card-body">
              <div className="d-flex flex-column flex-md-row align-items-start align-items-md-center justify-content-between gap-2 mb-3">
              <div>
                <h5 className="fw-semibold mb-1">API Key</h5>
                <div className="text-muted small">
                  可创建多个、可随时撤销；Key 明文仅创建时返回一次。
                </div>
              </div>

              <div className="d-flex align-items-center gap-2">
                <span className="text-muted small">启用中：{activeCount}</span>
                <button type="button" className="btn btn-primary btn-sm" data-bs-toggle="modal" data-bs-target="#createPersonalAPIKeyModal">
                  新建 Key
                </button>
              </div>
            </div>

            {notice ? (
              <div className="alert alert-success d-flex align-items-center" role="alert">
                <span className="me-2 material-symbols-rounded">check_circle</span>
                <div>{notice}</div>
              </div>
            ) : null}

            {err ? (
              <div className="alert alert-danger d-flex align-items-center" role="alert">
                <span className="me-2 material-symbols-rounded">warning</span>
                <div>{err}</div>
              </div>
            ) : null}

            {loading ? (
              <div className="text-muted">加载中…</div>
            ) : (
              <div className="table-responsive">
                <table className="table align-middle table-hover">
                  <thead>
                    <tr>
                      <th style={{ width: 220 }}>名称</th>
                      <th style={{ width: 160 }}>Hint</th>
                      <th style={{ width: 120 }}>状态</th>
                      <th style={{ width: 180 }}>创建</th>
                      <th style={{ width: 180 }}>最后使用</th>
                      <th style={{ width: 120 }} className="text-end">
                        操作
                      </th>
                    </tr>
                  </thead>
                  <tbody>
                    {keys.map((k) => (
                      <tr key={k.id}>
                        <td>
                          <div className="fw-medium text-truncate" style={{ maxWidth: 220 }} title={(k.name || '').toString()}>
                            {(k.name || '').toString().trim() || '-'}
                          </div>
                          <div className="text-muted small">#{k.id}</div>
                        </td>
                        <td>
                          <code className="user-select-all">{(k.key_hint || '').toString().trim() || '-'}</code>
                        </td>
                        <td>
                          <span className={statusBadge(k.status)}>{k.status === 1 ? '启用' : '已撤销'}</span>
                        </td>
                        <td className="text-muted small">{k.created_at ? formatLocalDateTimeMinute(k.created_at) : '-'}</td>
                        <td className="text-muted small">{k.last_used_at ? formatLocalDateTimeMinute(k.last_used_at) : '-'}</td>
                        <td className="text-end">
                          {k.status === 1 ? (
                            <button
                              type="button"
                              className="btn btn-outline-danger btn-sm"
                              onClick={async () => {
                                if (!window.confirm('确认撤销该 Key？撤销后将无法再用于 /v1 调用。')) return;
                                setErr('');
                                setNotice('');
                                try {
                                  const res = await revokePersonalAPIKey(k.id);
                                  if (!res.success) throw new Error(res.message || '撤销失败');
                                  setNotice('已撤销');
                                  await refresh();
                                } catch (e) {
                                  setErr(e instanceof Error ? e.message : '撤销失败');
                                }
                              }}
                            >
                              撤销
                            </button>
                          ) : null}
                        </td>
                      </tr>
                    ))}
                    {keys.length === 0 ? (
                      <tr>
                        <td colSpan={6} className="text-muted">
                          暂无 Key。点击“新建 Key”生成一个 Key。
                        </td>
                      </tr>
                    ) : null}
                  </tbody>
                </table>
              </div>
            )}
          </div>
        </div>
      </div>

      <BootstrapModal
        id="createPersonalAPIKeyModal"
        title="新建数据面 API Key"
        dialogClassName="modal-dialog-centered modal-lg"
        onHidden={() => {
          setName('');
          setCreateState({ phase: 'idle' });
        }}
        footer={
          <div className="d-flex justify-content-between w-100">
            <button type="button" className="btn btn-light" data-bs-dismiss="modal">
              关闭
            </button>
            <button
              type="button"
              className="btn btn-primary"
              disabled={createState.phase === 'creating' || createState.phase === 'created'}
              onClick={async () => {
                setErr('');
                setNotice('');
                setCreateState({ phase: 'creating' });
                try {
                  const res = await createPersonalAPIKey((name || '').trim() || undefined);
                  if (!res.success) throw new Error(res.message || '创建失败');
                  const k = res.data?.key || '';
                  if (!k) throw new Error('创建失败：未返回 Key');
                  setCreateState({ phase: 'created', key: k, copied: false });
                  await refresh();
                } catch (e) {
                  setCreateState({ phase: 'idle' });
                  setErr(e instanceof Error ? e.message : '创建失败');
                }
              }}
            >
              {createState.phase === 'creating' ? '创建中…' : createState.phase === 'created' ? '已创建' : '创建'}
            </button>
          </div>
        }
      >
        <div className="row g-3">
          <div className="col-12">
            <label className="form-label fw-medium">名称（可选）</label>
            <input className="form-control" value={name} onChange={(e) => setName(e.target.value)} placeholder="例如：macbook / ci / codex-cli" autoComplete="off" />
          </div>

          {createState.phase === 'created' ? (
            <div className="col-12">
              <div className="alert alert-warning mb-0" role="alert">
                <div className="fw-semibold mb-2">请立即复制该 Key（页面不会再次显示）</div>
                <div className="d-flex flex-column flex-md-row gap-2 align-items-stretch">
                  <input className="form-control font-monospace user-select-all" value={createState.key} readOnly />
                  <button
                    type="button"
                    className="btn btn-outline-secondary"
                    onClick={async () => {
                      try {
                        await navigator.clipboard.writeText(createState.key);
                        setCreateState({ phase: 'created', key: createState.key, copied: true });
                      } catch {
                        // ignore
                      }
                    }}
                  >
                    {createState.copied ? '已复制' : '复制'}
                  </button>
                </div>
                <div className="mt-2 text-muted small">
                  使用示例：<code>Authorization: Bearer {createState.key}</code>（或 <code>x-api-key</code>）。
                </div>
                <div className="mt-2">
                  <button
                    type="button"
                    className="btn btn-sm btn-light"
                    onClick={() => {
                      closeModalById('createPersonalAPIKeyModal');
                    }}
                  >
                    我已复制，关闭
                  </button>
                </div>
              </div>
            </div>
          ) : (
            <div className="col-12">
              <div className="text-muted small">
                提示：不要把“管理 Key”分发给下游客户端；API Key 不具备管理权限。
              </div>
            </div>
          )}
        </div>
      </BootstrapModal>
    </div>
  );
}
