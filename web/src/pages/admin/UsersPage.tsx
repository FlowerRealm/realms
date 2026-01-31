import { useEffect, useMemo, useState } from 'react';

import { useAuth } from '../../auth/AuthContext';
import { listAdminChannelGroups, type AdminChannelGroup } from '../../api/admin/channelGroups';
import {
  addAdminUserBalance,
  createAdminUser,
  deleteAdminUser,
  listAdminUsers,
  resetAdminUserPassword,
  updateAdminUser,
  type AdminUser,
} from '../../api/admin/users';
import { BootstrapModal } from '../../components/BootstrapModal';
import { closeModalById } from '../../components/modal';

function roleBadge(role: string): string {
  if (role === 'root') return 'badge rounded-pill bg-primary bg-opacity-10 text-primary border border-primary border-opacity-25 px-2';
  return 'badge rounded-pill bg-light text-secondary border px-2';
}

function statusBadge(status: number): { cls: string; label: string } {
  if (status === 1) return { cls: 'badge rounded-pill bg-success bg-opacity-10 text-success px-2', label: '启用' };
  return { cls: 'badge rounded-pill bg-secondary bg-opacity-10 text-secondary px-2', label: '禁用' };
}

function isDefaultGroup(name: string): boolean {
  return name.trim().toLowerCase() === 'default';
}

function parseGroups(csv: string): string[] {
  return csv
    .split(',')
    .map((s) => s.trim())
    .filter((s) => s);
}

export function UsersPage() {
  const { user: self } = useAuth();
  const selfID = self?.id || 0;

  const [users, setUsers] = useState<AdminUser[]>([]);
  const [groups, setGroups] = useState<AdminChannelGroup[]>([]);
  const [loading, setLoading] = useState(true);
  const [err, setErr] = useState('');
  const [notice, setNotice] = useState('');

  const [createEmail, setCreateEmail] = useState('');
  const [createUsername, setCreateUsername] = useState('');
  const [createPassword, setCreatePassword] = useState('');
  const [createRole, setCreateRole] = useState<'user' | 'root'>('user');
  const [createGroups, setCreateGroups] = useState<string[]>(['default']);

  const [editing, setEditing] = useState<AdminUser | null>(null);
  const [editEmail, setEditEmail] = useState('');
  const [editUsername, setEditUsername] = useState('');
  const [editRole, setEditRole] = useState<'user' | 'root'>('user');
  const [editStatus, setEditStatus] = useState(1);
  const [editGroups, setEditGroups] = useState<string[]>(['default']);

  const [balanceAmount, setBalanceAmount] = useState('');
  const [balanceNote, setBalanceNote] = useState('');

  const [newPassword, setNewPassword] = useState('');

  const enabledCount = useMemo(() => users.filter((u) => u.status === 1).length, [users]);

  async function refresh() {
    setErr('');
    setNotice('');
    setLoading(true);
    try {
      const [usersRes, groupsRes] = await Promise.all([listAdminUsers(), listAdminChannelGroups()]);
      if (!groupsRes.success) throw new Error(groupsRes.message || '加载分组失败');
      setGroups(groupsRes.data || []);
      if (!usersRes.success) throw new Error(usersRes.message || '加载用户失败');
      setUsers(usersRes.data || []);
    } catch (e) {
      setErr(e instanceof Error ? e.message : '加载失败');
      setUsers([]);
      setGroups([]);
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    void refresh();
  }, []);

  useEffect(() => {
    if (!editing) return;
    setEditEmail(editing.email || '');
    setEditUsername(editing.username || '');
    setEditRole((editing.role || 'user') as 'user' | 'root');
    setEditStatus(editing.status || 0);
    setEditGroups(parseGroups(editing.groups || 'default'));
    setBalanceAmount('');
    setBalanceNote('');
    setNewPassword('');
  }, [editing]);

  function groupDisabled(name: string, selected: string[]): boolean {
    if (isDefaultGroup(name)) return true;
    const g = groups.find((x) => x.name === name);
    if (!g) return false;
    if (g.status === 1) return false;
    return !selected.includes(name);
  }

  function toggleGroup(name: string, selected: string[], setSelected: (next: string[]) => void) {
    if (isDefaultGroup(name)) return;
    if (selected.includes(name)) {
      setSelected(selected.filter((x) => x !== name));
      return;
    }
    setSelected([...selected, name]);
  }

  return (
    <div className="fade-in-up">
      <div className="row g-4">
        <div className="col-12">
          <div className="card">
            <div className="card-body d-flex flex-column flex-md-row justify-content-between align-items-center">
              <div className="d-flex align-items-center mb-3 mb-md-0">
                <div className="bg-warning bg-opacity-10 text-warning rounded-circle d-flex align-items-center justify-content-center me-3" style={{ width: 48, height: 48 }}>
                  <span className="fs-4 material-symbols-rounded">group</span>
                </div>
                <div>
                  <h5 className="mb-1 fw-semibold">用户管理</h5>
                  <p className="mb-0 text-muted small">
                    {enabledCount} 启用 / {users.length} 总计 · 仅 root 可管理用户
                  </p>
                </div>
              </div>

              <div className="d-flex gap-2">
                <button type="button" className="btn btn-primary btn-sm" data-bs-toggle="modal" data-bs-target="#createUserModal">
                  <span className="me-1 material-symbols-rounded">person_add</span> 创建用户
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
          ) : users.length === 0 ? (
            <div className="text-center py-5 text-muted">
              <span className="fs-1 d-block mb-3 material-symbols-rounded">inbox</span>
              暂无用户。
            </div>
          ) : (
            <div className="card overflow-hidden mb-0">
              <div className="table-responsive">
                <table className="table table-hover align-middle mb-0">
                  <thead className="table-light">
                    <tr>
                      <th className="ps-4">邮箱</th>
                      <th>账号名</th>
                      <th>组</th>
                      <th>角色</th>
                      <th>状态</th>
                      <th>余额(USD)</th>
                      <th>创建时间</th>
                      <th className="text-end pe-4">操作</th>
                    </tr>
                  </thead>
                  <tbody>
                    {users.map((u) => {
                      const st = statusBadge(u.status);
                      return (
                        <tr key={u.id}>
                          <td className="ps-4">
                            <span className="fw-bold text-dark">{u.email}</span>
                          </td>
                          <td>{u.username ? <span className="text-dark fw-medium user-select-all">{u.username}</span> : <span className="text-muted small fst-italic">未设置</span>}</td>
                          <td>
                            <span className="badge bg-light text-secondary border fw-normal">{u.groups || 'default'}</span>
                          </td>
                          <td>
                            <span className={roleBadge(u.role)}>{u.role}</span>
                          </td>
                          <td>
                            <span className={st.cls}>{st.label}</span>
                          </td>
                          <td className="fw-medium text-dark">{u.balance_usd}</td>
                          <td className="text-muted small">{u.created_at}</td>
                          <td className="text-end pe-4 text-nowrap">
                            <div className="d-inline-flex gap-1">
                              <button
                                type="button"
                                className="btn btn-sm btn-light border text-success"
                                title="加余额"
                                data-bs-toggle="modal"
                                data-bs-target="#addBalanceModal"
                                onClick={() => setEditing(u)}
                              >
                                <i className="ri-money-dollar-circle-line"></i>
                              </button>
                              <button
                                type="button"
                                className="btn btn-sm btn-light border text-primary"
                                title="编辑用户"
                                data-bs-toggle="modal"
                                data-bs-target="#editUserModal"
                                onClick={() => setEditing(u)}
                              >
                                <i className="ri-edit-line"></i>
                              </button>
                              <button
                                type="button"
                                className="btn btn-sm btn-light border text-warning"
                                title="重置密码"
                                data-bs-toggle="modal"
                                data-bs-target="#resetPasswordModal"
                                onClick={() => setEditing(u)}
                              >
                                <i className="ri-key-2-line"></i>
                              </button>
                              <button
                                type="button"
                                className="btn btn-sm btn-light border text-danger"
                                title={u.id === selfID ? '不能删除当前登录用户' : '删除用户'}
                                disabled={u.id === selfID}
                                onClick={async () => {
                                  if (u.id === selfID) return;
                                  if (!window.confirm('确认删除该用户？此操作不可恢复。')) return;
                                  setErr('');
                                  setNotice('');
                                  try {
                                    const res = await deleteAdminUser(u.id);
                                    if (!res.success) throw new Error(res.message || '删除失败');
                                    setNotice('已删除');
                                    if (editing?.id === u.id) setEditing(null);
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
        id="createUserModal"
        title="创建用户"
        dialogClassName="modal-dialog-centered modal-lg"
        onHidden={() => {
          setCreateEmail('');
          setCreateUsername('');
          setCreatePassword('');
          setCreateRole('user');
          setCreateGroups(['default']);
        }}
      >
        <form
          className="row g-3"
          onSubmit={async (e) => {
            e.preventDefault();
            setErr('');
            setNotice('');
            try {
              const res = await createAdminUser({
                email: createEmail.trim(),
                username: createUsername.trim(),
                password: createPassword,
                role: createRole,
                groups: createGroups,
              });
              if (!res.success) throw new Error(res.message || '创建失败');
              setNotice('已创建');
              closeModalById('createUserModal');
              await refresh();
            } catch (e) {
              setErr(e instanceof Error ? e.message : '创建失败');
            }
          }}
        >
          <div className="col-md-6">
            <label className="form-label">邮箱</label>
            <input className="form-control" value={createEmail} onChange={(e) => setCreateEmail(e.target.value)} placeholder="alice@example.com" required />
          </div>
          <div className="col-md-6">
            <label className="form-label">账号名</label>
            <input className="form-control" value={createUsername} onChange={(e) => setCreateUsername(e.target.value)} placeholder="alice" required />
            <div className="form-text small text-muted">支持字母/数字及 . _ -，最多 32 位；用于登录。</div>
          </div>
          <div className="col-md-6">
            <label className="form-label">初始密码</label>
            <input className="form-control" value={createPassword} onChange={(e) => setCreatePassword(e.target.value)} placeholder="至少 8 位字符" type="password" autoComplete="new-password" required />
          </div>
          <div className="col-md-6">
            <label className="form-label">角色</label>
            <select className="form-select" value={createRole} onChange={(e) => setCreateRole((e.target.value as 'user' | 'root') || 'user')}>
              <option value="user">普通用户</option>
              <option value="root">超级管理员</option>
            </select>
          </div>
          <div className="col-12">
            <label className="form-label">组</label>
            <div className="card p-2" style={{ maxHeight: 240, overflowY: 'auto' }}>
              {groups.map((g) => {
                const checked = createGroups.includes(g.name) || isDefaultGroup(g.name);
                const disabled = groupDisabled(g.name, createGroups);
                return (
                  <div key={g.id} className="form-check">
                    <input
                      className="form-check-input"
                      type="checkbox"
                      id={`create-group-${g.name}`}
                      checked={checked}
                      disabled={disabled}
                      onChange={() => toggleGroup(g.name, createGroups, setCreateGroups)}
                    />
                      <label className="form-check-label w-100" htmlFor={`create-group-${g.name}`}>
                        {g.name}
                        {g.status !== 1 ? (
                        <span className="badge bg-secondary ms-1">
                          禁用
                        </span>
                        ) : null}
                      </label>
                  </div>
                );
              })}
            </div>
            <div className="form-text small text-muted">用户可属于多个组（并集生效），且必须包含 default。</div>
          </div>
          <div className="modal-footer border-top-0 px-0 pb-0">
            <button type="button" className="btn btn-light" data-bs-dismiss="modal">
              取消
            </button>
            <button className="btn btn-primary px-4" type="submit" disabled={loading || !createEmail.trim() || !createUsername.trim() || !createPassword}>
              创建
            </button>
          </div>
        </form>
      </BootstrapModal>

      <BootstrapModal
        id="editUserModal"
        title={editing ? `编辑用户：${editing.email}` : '编辑用户'}
        dialogClassName="modal-dialog-centered modal-lg"
        onHidden={() => {
          setEditing(null);
        }}
      >
        {!editing ? (
          <div className="text-muted">未选择用户。</div>
        ) : (
          <form
            className="row g-3"
            onSubmit={async (e) => {
              e.preventDefault();
              if (!editing) return;
              setErr('');
              setNotice('');
              try {
                const res = await updateAdminUser(editing.id, {
                  email: editEmail.trim(),
                  username: editUsername.trim(),
                  role: editRole,
                  status: editStatus,
                  groups: editGroups,
                });
                if (!res.success) throw new Error(res.message || '保存失败');
                setNotice('已保存');
                closeModalById('editUserModal');
                await refresh();
              } catch (e) {
                setErr(e instanceof Error ? e.message : '保存失败');
              }
            }}
          >
            <div className="col-md-6">
              <label className="form-label">邮箱</label>
              <input className="form-control" value={editEmail} onChange={(e) => setEditEmail(e.target.value)} required />
              <div className="form-text small text-muted">修改邮箱成功后会强制登出该用户。</div>
            </div>
            <div className="col-md-6">
              <label className="form-label">账号名</label>
              <input className="form-control" value={editUsername} onChange={(e) => setEditUsername(e.target.value)} required />
              <div className="form-text small text-muted">支持字母/数字及 . _ -，最多 32 位；用于登录。</div>
            </div>
            <div className="col-md-6">
              <label className="form-label">状态</label>
              <select className="form-select" value={editStatus} onChange={(e) => setEditStatus(Number.parseInt(e.target.value, 10) || 0)} disabled={editing.id === selfID}>
                <option value={1}>启用</option>
                <option value={0}>禁用</option>
              </select>
            </div>
            <div className="col-md-6">
              <label className="form-label">角色</label>
              <select className="form-select" value={editRole} onChange={(e) => setEditRole((e.target.value as 'user' | 'root') || 'user')} disabled={editing.id === selfID}>
                <option value="user">普通用户</option>
                <option value="root">超级管理员</option>
              </select>
              {editing.id === selfID ? <div className="form-text small text-muted">不能修改当前登录用户的状态或角色。</div> : null}
            </div>
            <div className="col-12">
              <label className="form-label">组</label>
              <div className="card p-2" style={{ maxHeight: 240, overflowY: 'auto' }}>
                {groups.map((g) => {
                  const checked = editGroups.includes(g.name) || isDefaultGroup(g.name);
                  const disabled = groupDisabled(g.name, editGroups);
                  return (
                    <div key={g.id} className="form-check">
                      <input
                        className="form-check-input"
                        type="checkbox"
                        id={`edit-group-${editing.id}-${g.name}`}
                        checked={checked}
                        disabled={disabled}
                        onChange={() => toggleGroup(g.name, editGroups, setEditGroups)}
                      />
                      <label className="form-check-label w-100" htmlFor={`edit-group-${editing.id}-${g.name}`}>
                        {g.name}
                        {g.status !== 1 ? (
                          <span className="badge bg-secondary ms-1">
                            禁用
                          </span>
                        ) : null}
                      </label>
                    </div>
                  );
                })}
              </div>
              <div className="form-text small text-muted">用户可属于多个组（并集生效），且必须包含 default。</div>
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

      <BootstrapModal
        id="addBalanceModal"
        title={editing ? `加余额：${editing.email}` : '加余额'}
        dialogClassName="modal-dialog-centered"
        onHidden={() => {
          setEditing(null);
          setBalanceAmount('');
          setBalanceNote('');
        }}
      >
        {!editing ? (
          <div className="text-muted">未选择用户。</div>
        ) : (
          <form
            className="row g-3"
            onSubmit={async (e) => {
              e.preventDefault();
              if (!editing) return;
              setErr('');
              setNotice('');
              try {
                const res = await addAdminUserBalance(editing.id, balanceAmount.trim(), balanceNote.trim());
                if (!res.success) throw new Error(res.message || '加余额失败');
                setNotice('已加余额');
                closeModalById('addBalanceModal');
                await refresh();
              } catch (e) {
                setErr(e instanceof Error ? e.message : '加余额失败');
              }
            }}
          >
            <div className="col-12">
              <div className="alert alert-light border py-2 small mb-0">
                <div className="d-flex justify-content-between">
                  <span className="text-muted">当前余额</span>
                  <span className="fw-bold text-dark">{editing.balance_usd} USD</span>
                </div>
              </div>
            </div>
            <div className="col-12">
              <label className="form-label">增加金额 (USD)</label>
              <input className="form-control" value={balanceAmount} onChange={(e) => setBalanceAmount(e.target.value)} placeholder="例如：5 或 0.5" inputMode="decimal" required />
              <div className="form-text small text-muted">最多 6 位小数；仅支持增加（不支持扣减/设置）。</div>
            </div>
            <div className="col-12">
              <label className="form-label">备注（可选）</label>
              <textarea className="form-control" rows={3} maxLength={200} value={balanceNote} onChange={(e) => setBalanceNote(e.target.value)} placeholder="用于审计记录（最多 200 字符）" />
            </div>
            <div className="modal-footer border-top-0 px-0 pb-0">
              <button type="button" className="btn btn-light" data-bs-dismiss="modal">
                取消
              </button>
              <button className="btn btn-success px-4" type="submit" disabled={!balanceAmount.trim()}>
                确认加余额
              </button>
            </div>
          </form>
        )}
      </BootstrapModal>

      <BootstrapModal
        id="resetPasswordModal"
        title={editing ? `重置密码：${editing.email}` : '重置密码'}
        dialogClassName="modal-dialog-centered"
        onHidden={() => {
          setEditing(null);
          setNewPassword('');
        }}
      >
        {!editing ? (
          <div className="text-muted">未选择用户。</div>
        ) : (
          <form
            className="row g-3"
            onSubmit={async (e) => {
              e.preventDefault();
              if (!editing) return;
              if (!window.confirm('确认重置密码并强制登出该用户？')) return;
              setErr('');
              setNotice('');
              try {
                const res = await resetAdminUserPassword(editing.id, newPassword);
                if (!res.success) throw new Error(res.message || '重置失败');
                setNotice('已重置密码');
                closeModalById('resetPasswordModal');
                await refresh();
              } catch (e) {
                setErr(e instanceof Error ? e.message : '重置失败');
              }
            }}
          >
            <div className="col-12">
              <label className="form-label">新密码</label>
              <input className="form-control" value={newPassword} onChange={(e) => setNewPassword(e.target.value)} type="password" autoComplete="new-password" placeholder="至少 8 位字符" required />
              <div className="form-text small text-muted">重置成功后会清理该用户所有已登录会话。</div>
            </div>
            <div className="modal-footer border-top-0 px-0 pb-0">
              <button type="button" className="btn btn-light" data-bs-dismiss="modal">
                取消
              </button>
              <button className="btn btn-primary px-4" type="submit" disabled={!newPassword}>
                重置
              </button>
            </div>
          </form>
        )}
      </BootstrapModal>
    </div>
  );
}
