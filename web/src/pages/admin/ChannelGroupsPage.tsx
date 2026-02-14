import { useEffect, useMemo, useState } from 'react';
import { Link } from 'react-router-dom';

import { BootstrapModal } from '../../components/BootstrapModal';
import { closeModalById } from '../../components/modal';
import {
  createAdminChannelGroup,
  deleteAdminChannelGroup,
  listAdminChannelGroups,
  setAdminDefaultChannelGroup,
  upsertAdminChannelGroupPointer,
  updateAdminChannelGroup,
  type AdminChannelGroup,
} from '../../api/admin/channelGroups';

function statusBadge(status: number): { cls: string; label: string } {
  if (status === 1) return { cls: 'badge rounded-pill bg-success bg-opacity-10 text-success px-2', label: '启用' };
  return { cls: 'badge rounded-pill bg-secondary bg-opacity-10 text-secondary px-2', label: '禁用' };
}

export function ChannelGroupsPage() {
  const [items, setItems] = useState<AdminChannelGroup[]>([]);
  const [loading, setLoading] = useState(true);
  const [err, setErr] = useState('');
  const [notice, setNotice] = useState('');

  const [createName, setCreateName] = useState('');
  const [createDesc, setCreateDesc] = useState('');
  const [createMultiplier, setCreateMultiplier] = useState('1');
  const [createMaxAttempts, setCreateMaxAttempts] = useState('5');
  const [createStatus, setCreateStatus] = useState(1);

  const [editing, setEditing] = useState<AdminChannelGroup | null>(null);
  const [editName, setEditName] = useState('');
  const [editDesc, setEditDesc] = useState('');
  const [editMultiplier, setEditMultiplier] = useState('1');
  const [editMaxAttempts, setEditMaxAttempts] = useState('5');

  const enabledCount = useMemo(() => items.filter((x) => x.status === 1).length, [items]);

  async function refresh() {
    setErr('');
    setNotice('');
    setLoading(true);
    try {
      const res = await listAdminChannelGroups();
      if (!res.success) throw new Error(res.message || '加载失败');
      setItems(res.data || []);
    } catch (e) {
      setErr(e instanceof Error ? e.message : '加载失败');
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    void refresh();
  }, []);

  useEffect(() => {
    if (!editing) return;
    setEditName(editing.name || '');
    setEditDesc(editing.description || '');
    setEditMultiplier(editing.price_multiplier || '1');
    setEditMaxAttempts(String(editing.max_attempts || 5));
  }, [editing]);

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
                  <span className="fs-4 material-symbols-rounded">folder</span>
                </div>
                <div>
                  <h5 className="mb-1 fw-semibold">渠道组</h5>
                  <p className="mb-0 text-muted small">{enabledCount} 启用 / {items.length} 总计 · 用于按用户组筛选可用上游渠道（不是租户）</p>
                </div>
              </div>

              <div className="d-flex gap-2">
                <button type="button" className="btn btn-primary btn-sm" data-bs-toggle="modal" data-bs-target="#createChannelGroupModal">
                  <span className="me-1 material-symbols-rounded">add</span> 新建渠道组
                </button>
              </div>
            </div>
          </div>
        </div>

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
              暂无渠道组。
            </div>
          ) : (
            <div className="card overflow-hidden mb-0">
              <div className="table-responsive">
                <table className="table table-hover align-middle mb-0">
                  <thead className="table-light">
                    <tr>
                      <th className="ps-4">名称</th>
                      <th>指针</th>
                      <th>倍率</th>
                      <th>描述</th>
                      <th>状态</th>
                      <th>创建时间</th>
                      <th className="text-end pe-4">操作</th>
                    </tr>
                  </thead>
                  <tbody>
                    {items.map((g) => {
                      const st = statusBadge(g.status);
                      const ptrID = typeof g.pointer_channel_id === 'number' ? g.pointer_channel_id : 0;
                      const ptrLabel = ptrID
                        ? g.pointer_channel_name?.trim()
                          ? g.pointer_channel_name.trim()
                          : `channel-${ptrID}`
                        : '-';
                      return (
                        <tr key={g.id}>
                          <td className="ps-4">
                            <span className="fw-bold text-dark user-select-all">{g.name}</span>
                            {g.is_default ? <span className="badge bg-primary bg-opacity-10 text-primary border ms-2">默认</span> : null}
                          </td>
                          <td>
                            {ptrID > 0 ? (
                              <code className="text-warning user-select-all">{ptrLabel}</code>
                            ) : (
                              <span className="text-muted small fst-italic">-</span>
                            )}
                          </td>
                          <td className="fw-semibold text-dark">{g.price_multiplier}</td>
                          <td>{g.description ? <span className="text-dark">{g.description}</span> : <span className="text-muted small fst-italic">-</span>}</td>
                          <td>
                            <span className={st.cls}>{st.label}</span>
                          </td>
                          <td className="text-muted small text-nowrap">{g.created_at}</td>
                          <td className="text-end pe-4 text-nowrap">
                            <div className="d-inline-flex gap-1">
                              <Link to={`/admin/channel-groups/${g.id}`} className="btn btn-sm btn-light border text-secondary" title="进入">
                                <span className="material-symbols-rounded" style={{ fontSize: 18 }}>folder_open</span>
                              </Link>
                              {ptrID > 0 ? (
                                <button
                                  type="button"
                                  className="btn btn-sm btn-light border text-warning"
                                  title="清除指针"
                                  onClick={async () => {
                                    if (!window.confirm('确认清除该组指针？')) return;
                                    setErr('');
                                    setNotice('');
                                    try {
                                      const res = await upsertAdminChannelGroupPointer(g.id, { channel_id: 0, pinned: false });
                                      if (!res.success) throw new Error(res.message || '清除失败');
                                      setNotice('已清除指针');
                                      await refresh();
                                    } catch (e) {
                                      setErr(e instanceof Error ? e.message : '清除失败');
                                    }
                                  }}
                                >
                                  <span className="material-symbols-rounded" style={{ fontSize: 18 }}>close</span>
                                </button>
                              ) : null}
                              <button
                                type="button"
                                className="btn btn-sm btn-light border text-warning"
                                title={g.is_default ? '默认渠道组' : g.status !== 1 ? '禁用渠道组不可设为默认' : '设为默认'}
                                disabled={g.is_default || g.status !== 1}
                                onClick={async () => {
                                  if (g.is_default || g.status !== 1) return;
                                  setErr('');
                                  setNotice('');
                                  try {
                                    const res = await setAdminDefaultChannelGroup(g.id);
                                    if (!res.success) throw new Error(res.message || '设置失败');
                                    setNotice(res.message || '已设置默认渠道组');
                                    await refresh();
                                  } catch (e) {
                                    setErr(e instanceof Error ? e.message : '设置失败');
                                  }
                                }}
                              >
                                <span className="material-symbols-rounded" style={{ fontSize: 18 }}>star</span>
                              </button>
                              <button
                                type="button"
                                className={`btn btn-sm btn-light border ${g.status === 1 ? 'text-success' : 'text-secondary'}`}
                                title={g.status === 1 ? (g.is_default ? '禁用（将清空默认设置）' : '禁用') : '启用'}
                                onClick={async () => {
                                  const nextStatus = g.status === 1 ? 0 : 1;
                                  setErr('');
                                  setNotice('');
                                  try {
                                    const res = await updateAdminChannelGroup(g.id, { status: nextStatus, description: g.description ?? null });
                                    if (!res.success) throw new Error(res.message || '操作失败');
                                    setNotice(nextStatus === 1 ? '已启用' : '已禁用');
                                    await refresh();
                                  } catch (e) {
                                    setErr(e instanceof Error ? e.message : '操作失败');
                                  }
                                }}
                              >
                                <span className="material-symbols-rounded" style={{ fontSize: 18 }}>
                                  {g.status === 1 ? 'toggle_on' : 'toggle_off'}
                                </span>
                              </button>
                              <button
                                type="button"
                                className="btn btn-sm btn-light border text-primary"
                                title="编辑"
                                data-bs-toggle="modal"
                                data-bs-target="#editChannelGroupModal"
                                onClick={() => {
                                  setEditing(g);
                                }}
                              >
                                <span className="material-symbols-rounded" style={{ fontSize: 18 }}>edit</span>
                              </button>
                              <button
                                type="button"
                                className="btn btn-sm btn-light border text-danger"
                                title={g.is_default ? '删除（将清空默认设置）' : '删除'}
                                onClick={async () => {
                                  setErr('');
                                  setNotice('');
                                  try {
                                    const res = await deleteAdminChannelGroup(g.id);
                                    if (!res.success) throw new Error(res.message || '删除失败');
                                    setNotice('已删除');
                                    if (editing?.id === g.id) setEditing(null);
                                    await refresh();
                                  } catch (e) {
                                    setErr(e instanceof Error ? e.message : '删除失败');
                                  }
                                }}
                              >
                                <span className="material-symbols-rounded" style={{ fontSize: 18 }}>
                                  delete
                                </span>
                              </button>
                            </div>
                          </td>
                        </tr>
                      );
                    })}
                  </tbody>
                </table>
              </div>
            </div>
          )}
        </div>
      </div>

      <BootstrapModal
        id="createChannelGroupModal"
        title="新建渠道组"
        dialogClassName="modal-dialog-centered"
        onHidden={() => {
          setCreateName('');
          setCreateDesc('');
          setCreateMultiplier('1');
          setCreateMaxAttempts('5');
          setCreateStatus(1);
        }}
      >
        <form
          className="row g-3"
          onSubmit={async (e) => {
            e.preventDefault();
            setErr('');
            setNotice('');
            try {
              const res = await createAdminChannelGroup({
                name: createName.trim(),
                description: createDesc.trim() || null,
                price_multiplier: createMultiplier.trim() || undefined,
                max_attempts: Number.parseInt(createMaxAttempts, 10) || undefined,
                status: createStatus,
              });
              if (!res.success) throw new Error(res.message || '创建失败');
              setNotice('已创建');
              closeModalById('createChannelGroupModal');
              await refresh();
            } catch (e) {
              setErr(e instanceof Error ? e.message : '创建失败');
            }
          }}
        >
          <div className="col-12">
            <label className="form-label">渠道组名称</label>
            <input className="form-control" value={createName} onChange={(e) => setCreateName(e.target.value)} placeholder="例如：vip" required />
            <div className="form-text small text-muted">仅允许字母/数字及 _ -，最多 64 位。</div>
          </div>
          <div className="col-12">
            <label className="form-label">描述（可选）</label>
            <input className="form-control" value={createDesc} onChange={(e) => setCreateDesc(e.target.value)} placeholder="例如：VIP 用户专用上游" />
            <div className="form-text small text-muted">最多 255 字符。</div>
          </div>
          <div className="col-12">
            <label className="form-label">价格倍率</label>
            <div className="input-group">
              <span className="input-group-text">×</span>
              <input className="form-control" value={createMultiplier} onChange={(e) => setCreateMultiplier(e.target.value)} inputMode="decimal" placeholder="1" />
            </div>
            <div className="form-text small text-muted">最终计费 = 模型单价 × 倍率（最多 6 位小数）。</div>
          </div>
          <div className="col-12">
            <label className="form-label">组内最大尝试次数</label>
            <input className="form-control" value={createMaxAttempts} onChange={(e) => setCreateMaxAttempts(e.target.value)} inputMode="numeric" placeholder="5" />
            <div className="form-text small text-muted">failover 尝试上限。</div>
          </div>
          <div className="col-12">
            <label className="form-label">状态</label>
            <select className="form-select" value={createStatus} onChange={(e) => setCreateStatus(Number.parseInt(e.target.value, 10) || 0)}>
              <option value={1}>启用</option>
              <option value={0}>禁用</option>
            </select>
          </div>
          <div className="modal-footer border-top-0 px-0 pb-0">
            <button type="button" className="btn btn-light" data-bs-dismiss="modal">
              取消
            </button>
            <button className="btn btn-primary px-4" type="submit" disabled={loading || !createName.trim()}>
              创建
            </button>
          </div>
        </form>
      </BootstrapModal>

      <BootstrapModal
        id="editChannelGroupModal"
        title={editing ? `编辑渠道组：${editing.name}` : '编辑渠道组'}
        dialogClassName="modal-dialog-centered"
        onHidden={() => setEditing(null)}
      >
        {!editing ? (
          <div className="text-muted">未选择渠道组。</div>
        ) : (
          <form
            className="row g-3"
            onSubmit={async (e) => {
              e.preventDefault();
              if (!editing) return;
              setErr('');
              setNotice('');
              try {
                const oldName = (editing.name || '').trim();
                const newName = editName.trim();
                const res = await updateAdminChannelGroup(editing.id, {
                  name: newName && newName !== oldName ? newName : undefined,
                  description: editDesc.trim() || null,
                  price_multiplier: editMultiplier.trim() || undefined,
                  max_attempts: Number.parseInt(editMaxAttempts, 10) || undefined,
                  status: editing.status,
                });
                if (!res.success) throw new Error(res.message || '保存失败');
                setNotice('已保存');
                closeModalById('editChannelGroupModal');
                await refresh();
              } catch (e) {
                setErr(e instanceof Error ? e.message : '保存失败');
              }
            }}
          >
            <div className="col-12">
              <label className="form-label">渠道组名称</label>
              <input className="form-control" value={editName} onChange={(e) => setEditName(e.target.value)} placeholder="例如：vip" required />
              <div className="form-text small text-muted">仅允许字母/数字及 _ -，最多 64 位。</div>
            </div>
            <div className="col-12">
              <label className="form-label">描述（可选）</label>
              <input className="form-control" value={editDesc} onChange={(e) => setEditDesc(e.target.value)} placeholder="例如：VIP 用户专用上游" />
            </div>
            <div className="col-12">
              <label className="form-label">价格倍率</label>
              <div className="input-group">
                <span className="input-group-text">×</span>
                <input className="form-control" value={editMultiplier} onChange={(e) => setEditMultiplier(e.target.value)} inputMode="decimal" />
              </div>
              <div className="form-text small text-muted">最终计费 = 模型单价 × 倍率（最多 6 位小数）。</div>
            </div>
            <div className="col-12">
              <label className="form-label">组内最大尝试次数</label>
              <input className="form-control" value={editMaxAttempts} onChange={(e) => setEditMaxAttempts(e.target.value)} inputMode="numeric" />
            </div>
            <div className="modal-footer border-top-0 px-0 pb-0">
              <button type="button" className="btn btn-light" data-bs-dismiss="modal">
                取消
              </button>
              <button className="btn btn-primary px-4" type="submit">
                保存
              </button>
            </div>
          </form>
        )}
      </BootstrapModal>
    </div>
  );
}
