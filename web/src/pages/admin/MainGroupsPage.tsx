import { useEffect, useMemo, useState } from 'react';

import { listAdminChannelGroups, type AdminChannelGroup } from '../../api/admin/channelGroups';
import {
  createAdminMainGroup,
  deleteAdminMainGroup,
  listAdminMainGroupSubgroups,
  listAdminMainGroups,
  replaceAdminMainGroupSubgroups,
  updateAdminMainGroup,
  type AdminMainGroup,
} from '../../api/admin/mainGroups';
import { BootstrapModal } from '../../components/BootstrapModal';
import { closeModalById } from '../../components/modal';

function statusBadge(status: number): { cls: string; label: string } {
  if (status === 1) return { cls: 'badge rounded-pill bg-success bg-opacity-10 text-success px-2', label: '启用' };
  return { cls: 'badge rounded-pill bg-secondary bg-opacity-10 text-secondary px-2', label: '禁用' };
}

export function MainGroupsPage() {
  const [groups, setGroups] = useState<AdminMainGroup[]>([]);
  const [channelGroups, setChannelGroups] = useState<AdminChannelGroup[]>([]);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [err, setErr] = useState('');
  const [notice, setNotice] = useState('');

  const [createName, setCreateName] = useState('');
  const [createDesc, setCreateDesc] = useState('');
  const [createStatus, setCreateStatus] = useState(1);

  const [editing, setEditing] = useState<AdminMainGroup | null>(null);
  const [editDesc, setEditDesc] = useState('');
  const [editStatus, setEditStatus] = useState(1);

  const [subgroupsFor, setSubgroupsFor] = useState<AdminMainGroup | null>(null);
  const [subgroupsLoading, setSubgroupsLoading] = useState(false);
  const [subgroups, setSubgroups] = useState<string[]>([]);
  const [addSubgroup, setAddSubgroup] = useState('');

  const selectableChannelGroups = useMemo(() => channelGroups.filter((g) => g.status === 1).slice().sort((a, b) => a.name.localeCompare(b.name, 'zh-CN')), [channelGroups]);
  const channelGroupByName = useMemo(() => {
    const m = new Map<string, AdminChannelGroup>();
    for (const g of channelGroups) m.set(g.name, g);
    return m;
  }, [channelGroups]);

  async function refresh() {
    setErr('');
    setNotice('');
    setLoading(true);
    try {
      const [groupsRes, channelGroupsRes] = await Promise.all([listAdminMainGroups(), listAdminChannelGroups()]);
      if (!channelGroupsRes.success) throw new Error(channelGroupsRes.message || '加载渠道分组失败');
      setChannelGroups(channelGroupsRes.data || []);
      if (!groupsRes.success) throw new Error(groupsRes.message || '加载失败');
      setGroups(groupsRes.data || []);
    } catch (e) {
      setErr(e instanceof Error ? e.message : '加载失败');
      setGroups([]);
      setChannelGroups([]);
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    void refresh();
  }, []);

  useEffect(() => {
    if (!editing) return;
    setEditDesc((editing.description || '').toString());
    setEditStatus(editing.status || 0);
  }, [editing]);

  async function openSubgroupsModal(g: AdminMainGroup) {
    setErr('');
    setNotice('');
    setSubgroupsFor(g);
    setSubgroupsLoading(true);
    setAddSubgroup('');
    try {
      const res = await listAdminMainGroupSubgroups(g.name);
      if (!res.success) throw new Error(res.message || '加载子组失败');
      const ordered = (res.data || []).map((x) => (x.subgroup || '').trim()).filter((x) => x);
      const dedup: string[] = [];
      const seen = new Set<string>();
      for (const name of ordered) {
        if (seen.has(name)) continue;
        seen.add(name);
        dedup.push(name);
      }
      setSubgroups(dedup);
    } catch (e) {
      setSubgroups([]);
      setErr(e instanceof Error ? e.message : '加载子组失败');
    } finally {
      setSubgroupsLoading(false);
      window.setTimeout(() => document.getElementById('openMainGroupSubgroupsModal')?.click(), 0);
    }
  }

  function removeSubgroup(name: string) {
    setSubgroups((prev) => {
      return prev.filter((x) => x !== name);
    });
  }

  function addSubgroupName(name: string) {
    const v = (name || '').trim();
    if (!v) return;
    setSubgroups((prev) => {
      if (prev.includes(v)) return prev;
      return [...prev, v];
    });
  }

  return (
    <div className="fade-in-up">
      <div className="row g-4">
        <div className="col-12">
          <div className="card">
            <div className="card-body d-flex flex-column flex-md-row justify-content-between align-items-center">
              <div className="d-flex align-items-center mb-3 mb-md-0">
                <div className="bg-primary bg-opacity-10 text-primary rounded-circle d-flex align-items-center justify-content-center me-3" style={{ width: 48, height: 48 }}>
                  <span className="fs-4 material-symbols-rounded">layers</span>
                </div>
                <div>
                  <h5 className="mb-1 fw-semibold">用户分组管理</h5>
                  <p className="mb-0 text-muted small">定义用户所属用户分组（单选），以及该用户分组可绑定的“子组（渠道分组）”。</p>
                </div>
              </div>
              <button type="button" className="btn btn-primary btn-sm" data-bs-toggle="modal" data-bs-target="#createMainGroupModal">
                <span className="me-1 material-symbols-rounded">add</span> 创建用户分组
              </button>
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
          ) : groups.length === 0 ? (
            <div className="text-center py-5 text-muted">
              <span className="fs-1 d-block mb-3 material-symbols-rounded">inbox</span>
              暂无用户分组。
            </div>
          ) : (
            <div className="card overflow-hidden mb-0">
              <div className="table-responsive">
                <table className="table table-hover align-middle mb-0">
                  <thead className="table-light">
                    <tr>
                      <th className="ps-4">名称</th>
                      <th>描述</th>
                      <th>状态</th>
                      <th>更新时间</th>
                      <th className="text-end pe-4">操作</th>
                    </tr>
                  </thead>
                  <tbody>
	                    {groups.map((g) => {
	                      const st = statusBadge(g.status);
	                      const name = (g.name || '').trim();
	                      return (
	                        <tr key={name}>
	                          <td className="ps-4">
	                            <span className="badge bg-light text-dark border fw-normal font-monospace">{name}</span>
	                          </td>
                          <td className="text-muted small">{(g.description || '').toString().trim() || '-'}</td>
                          <td>
                            <span className={st.cls}>{st.label}</span>
                          </td>
                          <td className="text-muted small">{g.updated_at}</td>
                          <td className="text-end pe-4 text-nowrap">
                            <div className="d-inline-flex gap-1">
                              <button type="button" className="btn btn-sm btn-light border text-primary" title="配置子组" onClick={() => void openSubgroupsModal(g)}>
                                <i className="ri-node-tree"></i>
                              </button>
                              <button
                                type="button"
                                className="btn btn-sm btn-light border text-success"
                                title="编辑用户分组"
                                data-bs-toggle="modal"
                                data-bs-target="#editMainGroupModal"
                                onClick={() => setEditing(g)}
                              >
                                <i className="ri-edit-line"></i>
                              </button>
	                              <button
	                                type="button"
	                                className="btn btn-sm btn-light border text-danger"
	                                title="删除用户分组"
	                                onClick={async () => {
	                                  if (!window.confirm(`确认删除用户分组 ${name}？此操作不可恢复。`)) return;
	                                  setErr('');
	                                  setNotice('');
	                                  setSaving(true);
                                  try {
                                    const res = await deleteAdminMainGroup(name);
                                    if (!res.success) throw new Error(res.message || '删除失败');
                                    setNotice('已删除');
                                    await refresh();
                                  } catch (e) {
                                    setErr(e instanceof Error ? e.message : '删除失败');
                                  } finally {
                                    setSaving(false);
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
        id="createMainGroupModal"
        title="创建用户分组"
        onHidden={() => {
          setCreateName('');
          setCreateDesc('');
          setCreateStatus(1);
        }}
      >
        <form
          onSubmit={async (e) => {
            e.preventDefault();
            setErr('');
            setNotice('');
            setSaving(true);
            try {
              const res = await createAdminMainGroup({
                name: createName.trim(),
                description: createDesc.trim() || undefined,
                status: createStatus,
              });
              if (!res.success) throw new Error(res.message || '创建失败');
              closeModalById('createMainGroupModal');
              setNotice('已创建');
              await refresh();
            } catch (e) {
              setErr(e instanceof Error ? e.message : '创建失败');
            } finally {
              setSaving(false);
            }
          }}
        >
          <div className="row g-3">
            <div className="col-12 col-md-6">
              <label className="form-label">名称</label>
              <input className="form-control font-monospace" value={createName} onChange={(e) => setCreateName(e.target.value)} placeholder="例如: team_a" />
              <div className="text-muted smaller mt-1">仅允许字母/数字/下划线/连字符，最长 64。</div>
            </div>
            <div className="col-12 col-md-6">
              <label className="form-label">状态</label>
              <select className="form-select" value={createStatus} onChange={(e) => setCreateStatus(Number.parseInt(e.target.value, 10) || 0)}>
                <option value={1}>启用</option>
                <option value={0}>禁用</option>
              </select>
            </div>
            <div className="col-12">
              <label className="form-label">描述</label>
              <input className="form-control" value={createDesc} onChange={(e) => setCreateDesc(e.target.value)} placeholder="可选" />
            </div>
            <div className="col-12 d-grid">
              <button className="btn btn-primary" type="submit" disabled={saving}>
                {saving ? '创建中…' : '创建'}
              </button>
            </div>
          </div>
        </form>
      </BootstrapModal>

      <BootstrapModal
        id="editMainGroupModal"
        title={editing ? `编辑用户分组：${editing.name}` : '编辑用户分组'}
        onHidden={() => setEditing(null)}
      >
        {!editing ? (
          <div className="text-muted">未选择用户分组。</div>
        ) : (
          <form
            onSubmit={async (e) => {
              e.preventDefault();
              setErr('');
	              setNotice('');
	              setSaving(true);
	              try {
	                const name = (editing.name || '').trim();
	                const res = await updateAdminMainGroup(name, { description: editDesc.trim() || undefined, status: editStatus });
	                if (!res.success) throw new Error(res.message || '保存失败');
                closeModalById('editMainGroupModal');
                setNotice('已保存');
                await refresh();
              } catch (e) {
                setErr(e instanceof Error ? e.message : '保存失败');
              } finally {
                setSaving(false);
              }
            }}
          >
	            <div className="row g-3">
	              <div className="col-12 col-md-6">
	                <label className="form-label">名称</label>
	                <input className="form-control font-monospace" value={(editing.name || '').trim()} readOnly />
	              </div>
	              <div className="col-12 col-md-6">
	                <label className="form-label">状态</label>
	                <select className="form-select" value={editStatus} onChange={(e) => setEditStatus(Number.parseInt(e.target.value, 10) || 0)}>
	                  <option value={1}>启用</option>
	                  <option value={0}>禁用</option>
	                </select>
	              </div>
              <div className="col-12">
                <label className="form-label">描述</label>
                <input className="form-control" value={editDesc} onChange={(e) => setEditDesc(e.target.value)} placeholder="可选" />
              </div>
              <div className="col-12 d-grid">
                <button className="btn btn-primary" type="submit" disabled={saving}>
                  {saving ? '保存中…' : '保存'}
                </button>
              </div>
            </div>
          </form>
        )}
      </BootstrapModal>

      <button type="button" id="openMainGroupSubgroupsModal" data-bs-toggle="modal" data-bs-target="#editMainGroupSubgroupsModal" className="d-none" />
      <BootstrapModal
        id="editMainGroupSubgroupsModal"
        title={subgroupsFor ? `配置子组：${subgroupsFor.name}` : '配置子组'}
        onHidden={() => setSubgroupsFor(null)}
      >
        {!subgroupsFor ? (
          <div className="text-muted">未选择用户分组。</div>
        ) : (
	          <div>
	            <div className="d-flex align-items-center justify-content-between mb-3">
	              <div className="text-muted small">子组是可绑定的“渠道分组”（用于限制 Token 的可选范围）。</div>
	              <button
                type="button"
                className="btn btn-primary btn-sm"
                disabled={saving || subgroupsLoading}
                onClick={async () => {
                  setErr('');
                  setNotice('');
                  setSaving(true);
                  try {
                    const name = (subgroupsFor.name || '').trim();
                    const res = await replaceAdminMainGroupSubgroups(name, subgroups);
                    if (!res.success) throw new Error(res.message || '保存失败');
                    closeModalById('editMainGroupSubgroupsModal');
                    setNotice('已保存');
                  } catch (e) {
                    setErr(e instanceof Error ? e.message : '保存失败');
                  } finally {
                    setSaving(false);
                  }
                }}
              >
                {saving ? '保存中…' : '保存'}
              </button>
            </div>

            {subgroupsLoading ? <div className="text-muted small">加载中…</div> : null}

            <div className="row g-2 mb-3">
              <div className="col-12 col-md-8">
                <select className="form-select" value={addSubgroup} onChange={(e) => setAddSubgroup(e.target.value)}>
                  <option value="">选择要添加的渠道分组…</option>
                  {selectableChannelGroups.map((g) => (
                    <option key={g.id} value={g.name} disabled={subgroups.includes(g.name)}>
                      {g.name} · x{g.price_multiplier}
                    </option>
                  ))}
                </select>
              </div>
              <div className="col-12 col-md-4 d-grid">
                <button
                  type="button"
                  className="btn btn-outline-primary"
                  onClick={() => {
                    addSubgroupName(addSubgroup);
                    setAddSubgroup('');
                  }}
                  disabled={!addSubgroup || subgroups.includes(addSubgroup)}
                >
                  添加
                </button>
              </div>
            </div>

	            <div className="list-group">
	              {subgroups.map((name, idx) => {
	                const cg = channelGroupByName.get(name);
	                const mult = cg ? `x${cg.price_multiplier}` : 'x?';
	                const status = cg ? (cg.status === 1 ? '启用' : '禁用') : '未知';
	                const statusCls = !cg ? 'badge bg-secondary bg-opacity-10 text-secondary border' : cg.status === 1 ? 'badge bg-success bg-opacity-10 text-success border border-success-subtle' : 'badge bg-secondary bg-opacity-10 text-secondary border';
	                return (
	                  <div key={name} className="list-group-item d-flex align-items-center justify-content-between">
                    <div className="d-flex align-items-center gap-3" style={{ minWidth: 0 }}>
	                      <span className="badge bg-light text-dark border font-monospace">{idx + 1}</span>
	                      <div className="d-flex flex-column" style={{ minWidth: 0 }}>
	                        <div className="d-flex align-items-center gap-2" style={{ minWidth: 0 }}>
	                          <span className="fw-semibold font-monospace text-truncate" style={{ maxWidth: 240 }} title={name}>
	                            {name}
	                          </span>
	                          <span className="badge bg-light text-dark border fw-normal">{mult}</span>
	                          <span className={statusCls}>{status}</span>
	                        </div>
                        {cg?.description ? <div className="text-muted smaller text-truncate" style={{ maxWidth: 520 }} title={cg.description || ''}>{cg.description}</div> : null}
                      </div>
	                    </div>
	                    <div className="d-inline-flex gap-1">
	                      <button type="button" className="btn btn-sm btn-light border text-danger" title="移除" onClick={() => removeSubgroup(name)}>
	                        <i className="ri-close-line"></i>
	                      </button>
	                    </div>
	                  </div>
	                );
	              })}
            </div>
          </div>
        )}
      </BootstrapModal>
    </div>
  );
}
