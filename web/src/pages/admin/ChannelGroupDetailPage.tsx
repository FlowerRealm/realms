import { useEffect, useState } from 'react';
import { Link, useParams } from 'react-router-dom';

import { BootstrapModal } from '../../components/BootstrapModal';
import { closeModalById } from '../../components/modal';
import {
  addAdminChannelGroupChannelMember,
  createAdminChildChannelGroup,
  deleteAdminChannelGroupChannelMember,
  deleteAdminChannelGroupGroupMember,
  getAdminChannelGroupDetail,
  getAdminChannelGroupPointer,
  reorderAdminChannelGroupMembers,
  upsertAdminChannelGroupPointer,
  type AdminChannelGroupDetail,
  type AdminChannelGroupMember,
  type AdminChannelGroupPointer,
} from '../../api/admin/channelGroups';

function memberType(m: AdminChannelGroupMember): 'group' | 'channel' | 'unknown' {
  if (m.member_group_id) return 'group';
  if (m.member_channel_id) return 'channel';
  return 'unknown';
}

function statusBadge(status?: number | null): { cls: string; label: string } {
  if (status === 1) return { cls: 'badge rounded-pill bg-success bg-opacity-10 text-success px-2', label: '启用' };
  return { cls: 'badge rounded-pill bg-secondary bg-opacity-10 text-secondary px-2', label: '禁用' };
}

export function ChannelGroupDetailPage() {
  const params = useParams();
  const groupId = Number.parseInt((params.id || '').toString(), 10);

  const [data, setData] = useState<AdminChannelGroupDetail | null>(null);
  const [members, setMembers] = useState<AdminChannelGroupMember[]>([]);
  const [pointer, setPointer] = useState<AdminChannelGroupPointer | null>(null);
  const [loading, setLoading] = useState(true);
  const [err, setErr] = useState('');
  const [notice, setNotice] = useState('');

  const [addChannelID, setAddChannelID] = useState('');

  const [childName, setChildName] = useState('');
  const [childDesc, setChildDesc] = useState('');
  const [childMultiplier, setChildMultiplier] = useState('1');
  const [childMaxAttempts, setChildMaxAttempts] = useState('5');
  const [childStatus, setChildStatus] = useState(1);

  async function refresh() {
    setErr('');
    setNotice('');
    setLoading(true);
    try {
      if (!Number.isFinite(groupId) || groupId <= 0) throw new Error('参数错误');
      const [detailRes, pointerRes] = await Promise.all([getAdminChannelGroupDetail(groupId), getAdminChannelGroupPointer(groupId)]);
      if (!detailRes.success) throw new Error(detailRes.message || '加载失败');
      const d = detailRes.data || null;
      setData(d);
      setMembers(d?.members || []);
      if (pointerRes.success) {
        setPointer(pointerRes.data || null);
      } else {
        setPointer(null);
      }
    } catch (e) {
      setErr(e instanceof Error ? e.message : '加载失败');
      setData(null);
      setMembers([]);
      setPointer(null);
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    void refresh();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [groupId]);

  const group = data?.group;
  const breadcrumb = data?.breadcrumb || [];
  const channels = data?.channels || [];
  const canClearPointer = !!pointer && pointer.pinned && pointer.channel_id > 0;

  async function persistOrder(nextIDs: number[]) {
    setErr('');
    setNotice('');
    try {
      const res = await reorderAdminChannelGroupMembers(groupId, nextIDs);
      if (!res.success) throw new Error(res.message || '保存排序失败');
      setNotice('已保存排序');
    } catch (e) {
      setErr(e instanceof Error ? e.message : '保存排序失败');
    }
  }

  function moveMember(idx: number, delta: -1 | 1) {
    const next = [...members];
    const target = idx + delta;
    if (target < 0 || target >= next.length) return;
    const tmp = next[idx];
    next[idx] = next[target];
    next[target] = tmp;
    setMembers(next);
    void persistOrder(next.map((m) => m.member_id));
  }

  return (
    <div className="fade-in-up">
      <div className="d-flex justify-content-between align-items-center mb-4">
        <div>
          <h3 className="mb-1 fw-bold">渠道组</h3>
          <nav aria-label="breadcrumb">
            <ol className="breadcrumb breadcrumb-sm mb-1">
              <li className="breadcrumb-item">
                <Link to="/admin/channel-groups">分组列表</Link>
              </li>
              {breadcrumb.map((b) =>
                b.id === group?.id ? (
                  <li key={b.id} className="breadcrumb-item active" aria-current="page">
                    {b.name}
                  </li>
                ) : (
                  <li key={b.id} className="breadcrumb-item">
                    <Link to={`/admin/channel-groups/${b.id}`}>{b.name}</Link>
                  </li>
                ),
              )}
            </ol>
          </nav>
          {group ? (
            <div className="text-muted small">
              名称：<code>{group.name}</code> <span className="mx-2">·</span> max_attempts：<code>{group.max_attempts}</code> <span className="mx-2">·</span> 倍率：
              <code>{group.price_multiplier}</code>
            </div>
          ) : null}
        </div>
        <div className="d-flex gap-2">
          <button type="button" className="btn btn-light border" data-bs-toggle="modal" data-bs-target="#addChannelToGroupModal" disabled={!group}>
            <span className="me-1 material-symbols-rounded">add</span> 添加渠道
          </button>
          <button
            type="button"
            className="btn btn-light border"
            disabled={!group || !canClearPointer}
            onClick={async () => {
              if (!window.confirm('确认清除该组指针？')) return;
              setErr('');
              setNotice('');
              try {
                const res = await upsertAdminChannelGroupPointer(groupId, { channel_id: 0, pinned: false });
                if (!res.success) throw new Error(res.message || '清除失败');
                setNotice('已清除指针');
                await refresh();
              } catch (e) {
                setErr(e instanceof Error ? e.message : '清除失败');
              }
            }}
          >
            清除指针
          </button>
          <button type="button" className="btn btn-primary" data-bs-toggle="modal" data-bs-target="#createChildGroupModal" disabled={!group}>
            <span className="me-1 material-symbols-rounded">add</span> 新建子组
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
      ) : !group ? (
        <div className="alert alert-warning">未找到该分组。</div>
      ) : (
        <div className="row g-4">
          <div className="col-12">
            <div className="card border-0 overflow-hidden mb-0">
              <div className="card-header d-flex justify-content-between align-items-center">
                <span className="fw-semibold">成员</span>
                <span className="text-muted small">支持上下移动排序（等价于拖拽排序）。</span>
              </div>
              <div className="table-responsive">
                <table className="table table-hover align-middle mb-0">
                  <thead className="table-light">
                    <tr>
                      <th style={{ width: 96 }} className="text-center">
                        排序
                      </th>
                      <th>成员</th>
                      <th>类型</th>
                      <th>状态</th>
                      <th className="text-end pe-4">操作</th>
                    </tr>
                  </thead>
                  <tbody>
                    {members.map((m, idx) => {
                      const typ = memberType(m);
                      const st =
                        typ === 'group'
                          ? statusBadge(m.member_group_status)
                          : typ === 'channel'
                            ? statusBadge(m.member_channel_status)
                            : statusBadge(0);
                      const isPointerChannel = typ === 'channel' && !!pointer && pointer.pinned && pointer.channel_id === m.member_channel_id;
                      return (
                        <tr key={m.member_id} className={isPointerChannel ? 'table-warning' : undefined}>
                          <td className="text-center">
                            <div className="d-inline-flex gap-1">
                              <button
                                className="btn btn-sm btn-light border"
                                type="button"
                                disabled={idx === 0}
                                title="上移"
                                onClick={() => moveMember(idx, -1)}
                              >
                                <i className="ri-arrow-up-line"></i>
                              </button>
                              <button
                                className="btn btn-sm btn-light border"
                                type="button"
                                disabled={idx === members.length - 1}
                                title="下移"
                                onClick={() => moveMember(idx, 1)}
                              >
                                <i className="ri-arrow-down-line"></i>
                              </button>
                            </div>
                          </td>
                          <td style={{ minWidth: 0 }}>
                            {typ === 'group' ? (
                              <div className="d-flex flex-column">
                                <div className="d-flex flex-wrap align-items-center gap-2">
                                  <Link className="fw-bold text-dark text-decoration-none" to={`/admin/channel-groups/${m.member_group_id}`}>
                                    {m.member_group_name || `group-${m.member_group_id}`}
                                  </Link>
                                  <span className="badge rounded-pill bg-light text-secondary border small">group</span>
                                </div>
                                {m.member_group_max_attempts ? <div className="text-muted small">max_attempts：{m.member_group_max_attempts}</div> : null}
                              </div>
                            ) : typ === 'channel' ? (
                              <div className="d-flex flex-column">
                                <div className="d-flex flex-wrap align-items-center gap-2">
                                  <span className="fw-bold text-dark">{m.member_channel_name || `channel-${m.member_channel_id}`}</span>
                                  {m.member_channel_type ? (
                                    <span className="badge rounded-pill bg-light text-secondary border small">
                                      {m.member_channel_type}
                                    </span>
                                  ) : null}
                                  {m.promotion ? (
                                    <span className="badge rounded-pill bg-warning bg-opacity-10 text-warning border small">
                                      优先
                                    </span>
                                  ) : null}
                                </div>
                                {m.member_channel_groups ? (
                                  <div className="text-muted small">
                                    groups：<code className="user-select-all">{m.member_channel_groups}</code>
                                  </div>
                                ) : null}
                              </div>
                            ) : (
                              <span className="text-muted small fst-italic">-</span>
                            )}
                          </td>
                          <td>
                            {typ === 'group' ? (
                              <span className="badge rounded-pill bg-primary bg-opacity-10 text-primary px-2">子组</span>
                            ) : typ === 'channel' ? (
                              <span className="badge rounded-pill bg-success bg-opacity-10 text-success px-2">渠道</span>
                            ) : (
                              <span className="badge rounded-pill bg-secondary bg-opacity-10 text-secondary px-2">未知</span>
                            )}
                          </td>
                          <td>
                            <span className={st.cls}>{st.label}</span>
                          </td>
                          <td className="text-end pe-4 text-nowrap">
                            <div className="d-inline-flex gap-1">
                              {typ === 'channel' && typeof m.member_channel_id === 'number' && m.member_channel_id > 0
                                ? (() => {
                                    const channelID = m.member_channel_id;
                                    return (
                                      <button
                                        type="button"
                                        className="btn btn-sm btn-light border text-warning"
                                        title="设为指针"
                                        disabled={isPointerChannel}
                                        onClick={async () => {
                                          if (!window.confirm('确认将该渠道设为该组指针？')) return;
                                          setErr('');
                                          setNotice('');
                                          try {
                                            const res = await upsertAdminChannelGroupPointer(groupId, { channel_id: channelID, pinned: true });
                                            if (!res.success) throw new Error(res.message || '设置失败');
                                            setNotice('已设置指针');
                                            await refresh();
                                          } catch (e) {
                                            setErr(e instanceof Error ? e.message : '设置失败');
                                          }
                                        }}
                                      >
                                        <i className="ri-pushpin-2-line"></i>
                                      </button>
                                    );
                                  })()
                                : null}
                              {typ === 'group' && m.member_group_id ? (
                                <Link to={`/admin/channel-groups/${m.member_group_id}`} className="btn btn-sm btn-light border text-secondary" title="进入">
                                  <i className="ri-folder-open-line"></i>
                                </Link>
                              ) : null}
                              <button
                                type="button"
                                className="btn btn-sm btn-light border text-danger"
                                title="移除"
                                onClick={async () => {
                                  if (!window.confirm('确认从该组移除该成员？')) return;
                                  setErr('');
                                  setNotice('');
                                  try {
                                    if (typ === 'group' && m.member_group_id) {
                                      const res = await deleteAdminChannelGroupGroupMember(groupId, m.member_group_id);
                                      if (!res.success) throw new Error(res.message || '移除失败');
                                    } else if (typ === 'channel' && m.member_channel_id) {
                                      const res = await deleteAdminChannelGroupChannelMember(groupId, m.member_channel_id);
                                      if (!res.success) throw new Error(res.message || '移除失败');
                                    } else {
                                      throw new Error('成员类型不合法');
                                    }
                                    setNotice('已移除');
                                    await refresh();
                                  } catch (e) {
                                    setErr(e instanceof Error ? e.message : '移除失败');
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
                    {members.length === 0 ? (
                      <tr>
                        <td colSpan={5} className="text-center py-5 text-muted">
                          暂无成员，请先添加渠道或创建子组。
                        </td>
                      </tr>
                    ) : null}
                  </tbody>
                </table>
              </div>
            </div>
          </div>
        </div>
      )}

      <BootstrapModal
        id="addChannelToGroupModal"
        title="添加渠道到该组"
        dialogClassName="modal-dialog-centered"
        onHidden={() => {
          setAddChannelID('');
        }}
      >
        <form
          className="row g-3"
          onSubmit={async (e) => {
            e.preventDefault();
            setErr('');
            setNotice('');
            try {
              const id = Number.parseInt(addChannelID, 10);
              if (!Number.isFinite(id) || id <= 0) throw new Error('请选择渠道');
              const res = await addAdminChannelGroupChannelMember(groupId, id);
              if (!res.success) throw new Error(res.message || '添加失败');
              setAddChannelID('');
              setNotice('已添加');
              closeModalById('addChannelToGroupModal');
              await refresh();
            } catch (e) {
              setErr(e instanceof Error ? e.message : '添加失败');
            }
          }}
        >
          <div className="col-12">
            <label className="form-label">选择渠道</label>
            <select className="form-select" value={addChannelID} onChange={(e) => setAddChannelID(e.target.value)}>
              <option value="">请选择</option>
              {channels.map((c) => (
                <option key={c.id} value={String(c.id)}>
                  {c.name} (id={c.id}, {c.type})
                </option>
              ))}
            </select>
            <div className="form-text small text-muted">添加后会更新该渠道的 groups 缓存。</div>
          </div>
          <div className="modal-footer border-top-0 px-0 pb-0">
            <button type="button" className="btn btn-light" data-bs-dismiss="modal">
              取消
            </button>
            <button className="btn btn-primary px-4" type="submit" disabled={!addChannelID.trim()}>
              添加
            </button>
          </div>
        </form>
      </BootstrapModal>

      <BootstrapModal
        id="createChildGroupModal"
        title="新建子组"
        dialogClassName="modal-dialog-centered"
        onHidden={() => {
          setChildName('');
          setChildDesc('');
          setChildMultiplier('1');
          setChildMaxAttempts('5');
          setChildStatus(1);
        }}
      >
        <form
          className="row g-3"
          onSubmit={async (e) => {
            e.preventDefault();
            setErr('');
            setNotice('');
            try {
              const res = await createAdminChildChannelGroup(groupId, {
                name: childName.trim(),
                description: childDesc.trim() || null,
                price_multiplier: childMultiplier.trim() || undefined,
                max_attempts: Number.parseInt(childMaxAttempts, 10) || undefined,
                status: childStatus,
              });
              if (!res.success) throw new Error(res.message || '创建失败');
              setNotice('已创建子组');
              closeModalById('createChildGroupModal');
              await refresh();
            } catch (e) {
              setErr(e instanceof Error ? e.message : '创建失败');
            }
          }}
        >
          <div className="col-12">
            <label className="form-label">名称</label>
            <input className="form-control" value={childName} onChange={(e) => setChildName(e.target.value)} placeholder="例如：vip" required />
            <div className="form-text small text-muted">仅允许字母/数字及 _ -，最多 64 位。</div>
          </div>
          <div className="col-12">
            <label className="form-label">描述（可选）</label>
            <input className="form-control" value={childDesc} onChange={(e) => setChildDesc(e.target.value)} placeholder="例如：VIP 用户专用上游" />
            <div className="form-text small text-muted">最多 255 字符。</div>
          </div>
          <div className="col-12">
            <label className="form-label">倍率</label>
            <div className="input-group">
              <span className="input-group-text">×</span>
              <input className="form-control" value={childMultiplier} onChange={(e) => setChildMultiplier(e.target.value)} inputMode="decimal" placeholder="1" />
            </div>
            <div className="form-text small text-muted">最终计费 = 模型单价 × 倍率（最多 6 位小数）。</div>
          </div>
          <div className="col-12">
            <label className="form-label">max_attempts</label>
            <input className="form-control" value={childMaxAttempts} onChange={(e) => setChildMaxAttempts(e.target.value)} inputMode="numeric" placeholder="5" />
            <div className="form-text small text-muted">组内成员 failover 尝试上限。</div>
          </div>
          <div className="col-12">
            <label className="form-label">状态</label>
            <select className="form-select" value={childStatus} onChange={(e) => setChildStatus(Number.parseInt(e.target.value, 10) || 0)}>
              <option value={1}>启用</option>
              <option value={0}>禁用</option>
            </select>
          </div>
          <div className="modal-footer border-top-0 px-0 pb-0">
            <button type="button" className="btn btn-light" data-bs-dismiss="modal">
              取消
            </button>
            <button className="btn btn-primary px-4" type="submit" disabled={!childName.trim()}>
              创建
            </button>
          </div>
        </form>
      </BootstrapModal>
    </div>
  );
}
