import { useEffect, useMemo, useState } from 'react';
import { Link } from 'react-router-dom';

import { BootstrapModal } from '../../components/BootstrapModal';
import { closeModalById } from '../../components/modal';
import {
  createAdminChannelGroup,
  deleteAdminChannelGroup,
  listAdminChannelGroups,
  updateAdminChannelGroup,
  type AdminChannelGroup,
} from '../../api/admin/channelGroups';

function isDefaultGroup(name: string): boolean {
  return name.trim().toLowerCase() === 'default';
}

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
  const [editDesc, setEditDesc] = useState('');
  const [editMultiplier, setEditMultiplier] = useState('1');
  const [editMaxAttempts, setEditMaxAttempts] = useState('5');
  const [editStatus, setEditStatus] = useState(1);

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
    setEditDesc(editing.description || '');
    setEditMultiplier(editing.price_multiplier || '1');
    setEditMaxAttempts(String(editing.max_attempts || 5));
    setEditStatus(editing.status || 0);
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
                  <h5 className="mb-1 fw-semibold">分组</h5>
                  <p className="mb-0 text-muted small">{enabledCount} 启用 / {items.length} 总计 · 用于按用户组筛选可用上游渠道（不是租户）</p>
                </div>
              </div>

              <div className="d-flex gap-2">
                <button type="button" className="btn btn-primary btn-sm" data-bs-toggle="modal" data-bs-target="#createChannelGroupModal">
                  <span className="me-1 material-symbols-rounded">add</span> 新建分组
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
              暂无分组。
            </div>
          ) : (
            <div className="card overflow-hidden mb-0">
              <div className="table-responsive">
                <table className="table table-hover align-middle mb-0">
                  <thead className="table-light">
                    <tr>
                      <th className="ps-4">名称</th>
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
                      return (
                        <tr key={g.id}>
                          <td className="ps-4">
                            <span className="fw-bold text-dark user-select-all">{g.name}</span>
                            {isDefaultGroup(g.name) ? <span className="badge bg-light text-dark border ms-2">default</span> : null}
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
                                <i className="ri-folder-open-line"></i>
                              </Link>
                              <button
                                type="button"
                                className="btn btn-sm btn-light border text-primary"
                                title="编辑"
                                data-bs-toggle="modal"
                                data-bs-target="#editChannelGroupModal"
                                onClick={() => setEditing(g)}
                              >
                                <i className="ri-edit-line"></i>
                              </button>
                              <button
                                type="button"
                                className="btn btn-sm btn-light border text-danger"
                                title={isDefaultGroup(g.name) ? '默认分组不可删除' : '删除'}
                                disabled={isDefaultGroup(g.name)}
                                onClick={async () => {
                                  if (isDefaultGroup(g.name)) return;
                                  if (!window.confirm(`确认删除分组 ${g.name} ?`)) return;
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
                                <i className="ri-delete-bin-line"></i>
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
        title="新建分组"
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
            <label className="form-label">分组名称</label>
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
        title={editing ? `编辑分组：${editing.name}` : '编辑分组'}
        dialogClassName="modal-dialog-centered"
        onHidden={() => setEditing(null)}
      >
        {!editing ? (
          <div className="text-muted">未选择分组。</div>
        ) : (
          <form
            className="row g-3"
            onSubmit={async (e) => {
              e.preventDefault();
              if (!editing) return;
              setErr('');
              setNotice('');
              try {
                const res = await updateAdminChannelGroup(editing.id, {
                  description: editDesc.trim() || null,
                  price_multiplier: editMultiplier.trim() || undefined,
                  max_attempts: Number.parseInt(editMaxAttempts, 10) || undefined,
                  status: isDefaultGroup(editing.name) ? 1 : editStatus,
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
              <label className="form-label">分组名称</label>
              <input className="form-control bg-light" value={editing.name} disabled />
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
            <div className="col-12">
              <label className="form-label">状态</label>
              <select className="form-select" value={editStatus} onChange={(e) => setEditStatus(Number.parseInt(e.target.value, 10) || 0)} disabled={isDefaultGroup(editing.name)}>
                <option value={1}>启用</option>
                <option value={0}>禁用</option>
              </select>
              {isDefaultGroup(editing.name) ? <div className="form-text small text-muted">default 分组不允许禁用。</div> : null}
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
