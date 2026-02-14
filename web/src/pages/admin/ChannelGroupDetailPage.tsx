import { useCallback, useEffect, useMemo, useState, type CSSProperties, type ReactNode } from 'react';
import { Link, useParams } from 'react-router-dom';

import {
  DndContext,
  MouseSensor,
  TouchSensor,
  closestCenter,
  pointerWithin,
  useSensor,
  useSensors,
  type CollisionDetection,
  type DragEndEvent,
  type DragStartEvent,
  type Modifier,
} from '@dnd-kit/core';
import { SortableContext, arrayMove, useSortable, verticalListSortingStrategy } from '@dnd-kit/sortable';
import { CSS } from '@dnd-kit/utilities';

import { BootstrapModal } from '../../components/BootstrapModal';
import { closeModalById } from '../../components/modal';
import { PortalDragOverlay } from '../../components/PortalDragOverlay';
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

function statusBadge(status: number): { cls: string; label: string } {
  if (status === 1) return { cls: 'badge bg-success bg-opacity-10 text-success border border-success-subtle', label: '启用' };
  return { cls: 'badge bg-secondary bg-opacity-10 text-secondary border', label: '禁用' };
}

type UseSortableReturn = ReturnType<typeof useSortable>;
type SortableRowRenderArgs = Pick<
  UseSortableReturn,
  'attributes' | 'listeners' | 'setActivatorNodeRef' | 'setNodeRef' | 'transform' | 'transition' | 'isDragging' | 'isOver'
> & {
  setRowRef: (node: HTMLTableRowElement | null) => void;
};

function wrapDndListeners(listeners: SortableRowRenderArgs['listeners']): SortableRowRenderArgs['listeners'] {
  if (!listeners) return listeners;

  const getTarget = (e: unknown): Element | null => {
    if (!e || typeof e !== 'object') return null;
    const target = (e as { target?: unknown }).target;
    return target instanceof Element ? target : null;
  };

  const shouldIgnore = (target: Element | null) => {
    if (!target) return false;
    return !!target.closest('button, a, input, textarea, select, label, [data-rlm-dnd-ignore]');
  };

  const wrap = (fn: unknown) => {
    if (typeof fn !== 'function') return fn;
    return (e: unknown) => {
      if (shouldIgnore(getTarget(e))) return;
      (fn as (e: unknown) => void)(e);
    };
  };

  const base = listeners as unknown as Record<string, unknown>;
  const out: Record<string, unknown> = { ...base };
  out.onMouseDown = wrap(base.onMouseDown);
  out.onTouchStart = wrap(base.onTouchStart);
  out.onPointerDown = wrap(base.onPointerDown);
  return out as unknown as SortableRowRenderArgs['listeners'];
}

const restrictToVerticalAxisModifier: Modifier = ({ transform }) => {
  if (!transform) return transform;
  return { ...transform, x: 0 };
};

function SortableRow({
  id,
  disabled,
  children,
}: {
  id: number;
  disabled: boolean | { draggable?: boolean; droppable?: boolean };
  children: (args: SortableRowRenderArgs) => ReactNode;
}) {
  const { attributes, listeners, setActivatorNodeRef, setNodeRef, transform, transition, isDragging, isOver } = useSortable({ id, disabled });
  const wrappedListeners = useMemo(() => wrapDndListeners(listeners), [listeners]);
  const setRowRef = useCallback(
    (node: HTMLTableRowElement | null) => {
      setNodeRef(node);
      setActivatorNodeRef(node);
    },
    [setActivatorNodeRef, setNodeRef],
  );
  return children({ attributes, listeners: wrappedListeners, setActivatorNodeRef, setNodeRef, setRowRef, transform, transition, isDragging, isOver });
}

function channelTypeLabel(t: string): string {
  if (t === 'openai_compatible') return 'OpenAI 兼容';
  if (t === 'anthropic') return 'Anthropic';
  if (t === 'codex_oauth') return 'Codex OAuth';
  return t;
}

export function ChannelGroupDetailPage() {
  const params = useParams();
  const groupId = Number.parseInt((params.id || '').toString(), 10);

  const [data, setData] = useState<AdminChannelGroupDetail | null>(null);
  const [members, setMembers] = useState<AdminChannelGroupMember[]>([]);
  const [pointer, setPointer] = useState<AdminChannelGroupPointer | null>(null);
  const [loading, setLoading] = useState(true);
  const [reordering, setReordering] = useState(false);
  const [draggingID, setDraggingID] = useState<number | null>(null);
  const [dragOverlayWidth, setDragOverlayWidth] = useState<number | null>(null);
  const [dragOverlayColWidths, setDragOverlayColWidths] = useState<number[] | null>(null);
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

  const reduceMotion = useMemo(() => {
    if (typeof window === 'undefined' || typeof window.matchMedia !== 'function') return false;
    return window.matchMedia('(prefers-reduced-motion: reduce)').matches;
  }, []);

  const sensors = useSensors(
    useSensor(MouseSensor, { activationConstraint: { distance: 6 } }),
    useSensor(TouchSensor, { activationConstraint: { delay: 180, tolerance: 7 } }),
  );
  const collisionDetection = useCallback<CollisionDetection>((args) => {
    const pointerCollisions = pointerWithin(args);
    if (pointerCollisions.length > 0) return pointerCollisions;
    return closestCenter(args);
  }, []);

  const memberIDs = useMemo(() => members.map((m) => m.member_id), [members]);
  const draggingMember = useMemo(() => {
    if (draggingID === null) return null;
    return members.find((m) => m.member_id === draggingID) || null;
  }, [members, draggingID]);

  useEffect(() => {
    if (draggingID === null) return;
    const prevCursor = document.body.style.cursor;
    const prevUserSelect = document.body.style.userSelect;
    document.body.style.cursor = 'grabbing';
    document.body.style.userSelect = 'none';
    document.body.classList.add('rlm-dnd-sorting');
    return () => {
      document.body.style.cursor = prevCursor;
      document.body.style.userSelect = prevUserSelect;
      document.body.classList.remove('rlm-dnd-sorting');
    };
  }, [draggingID]);

  function sameIDOrder(a: number[], b: number[]): boolean {
    if (a.length !== b.length) return false;
    for (let i = 0; i < a.length; i++) {
      if (a[i] !== b[i]) return false;
    }
    return true;
  }

  function handleDragStart(e: DragStartEvent) {
    if (loading || reordering) return;
    const raw = e.active.id;
    const id = typeof raw === 'number' ? raw : Number.parseInt(String(raw), 10);
    if (!Number.isFinite(id) || id <= 0) return;
    setDraggingID(id);

    const row =
      typeof document !== 'undefined'
        ? (document.querySelector(`tr[data-rlm-channel-group-member-row="1"][data-rlm-member-id="${id}"]`) as HTMLElement | null)
        : null;
    const rect = row?.getBoundingClientRect() || null;
    setDragOverlayWidth(rect ? Math.round(rect.width) : null);

    const cols = row ? Array.from(row.querySelectorAll('td')) : [];
    const colWidths = cols.map((td) => Math.round(td.getBoundingClientRect().width));
    setDragOverlayColWidths(colWidths.length > 0 && colWidths.every((w) => Number.isFinite(w) && w > 0) ? colWidths : null);
  }

  function handleDragCancel() {
    setDraggingID(null);
    setDragOverlayWidth(null);
    setDragOverlayColWidths(null);
  }

  async function handleDragEnd(e: DragEndEvent) {
    const rawActive = e.active.id;
    const rawOver = e.over?.id;
    const activeID = typeof rawActive === 'number' ? rawActive : Number.parseInt(String(rawActive), 10);
    const overID = rawOver == null ? null : typeof rawOver === 'number' ? rawOver : Number.parseInt(String(rawOver), 10);

    setDraggingID(null);
    setDragOverlayWidth(null);
    setDragOverlayColWidths(null);
    if (!overID || !Number.isFinite(activeID) || activeID <= 0) return;
    if (activeID === overID) return;

    const startList = members;
    const from = startList.findIndex((m) => m.member_id === activeID);
    const to = startList.findIndex((m) => m.member_id === overID);
    if (from < 0 || to < 0) return;

    const next = arrayMove(startList, from, to);
    const nextIDs = next.map((m) => m.member_id);
    if (sameIDOrder(nextIDs, startList.map((m) => m.member_id))) return;

    setReordering(true);
    setErr('');
    setNotice('');
    setMembers(next);
    try {
      const res = await reorderAdminChannelGroupMembers(groupId, nextIDs);
      if (!res.success) throw new Error(res.message || '保存排序失败');
      setNotice('已保存排序');
      await refresh();
    } catch (e2) {
      setMembers(startList);
      setErr(e2 instanceof Error ? e2.message : '保存排序失败');
    } finally {
      setReordering(false);
    }
  }

  return (
    <DndContext sensors={sensors} collisionDetection={collisionDetection} onDragStart={handleDragStart} onDragEnd={handleDragEnd} onDragCancel={handleDragCancel}>
      <div className="fade-in-up">
        <PortalDragOverlay modifiers={[restrictToVerticalAxisModifier]} zIndex={2000}>
          {draggingMember ? (
            <div className="rlm-channel-dnd-overlay" style={{ width: dragOverlayWidth || undefined }}>
              <table className="table table-hover align-middle mb-0" style={{ tableLayout: 'fixed', width: '100%' }}>
                {dragOverlayColWidths ? (
                  <colgroup>
                    {dragOverlayColWidths.map((w, idx) => (
                      <col key={idx} style={{ width: w }} />
                    ))}
                  </colgroup>
                ) : null}
                <tbody>
                  <tr className="rlm-channel-row-main rlm-channel-row-drag-preview">
                    <td className="text-center text-muted">
                      <span className="d-inline-flex align-items-center justify-content-center" style={{ width: 48 }}>
                        <i className="ri-drag-move-2-line fs-5"></i>
                      </span>
                    </td>
                    <td className="ps-4" style={{ minWidth: 0 }}>
                      {(() => {
                        const typ = memberType(draggingMember);
                        const showPromotion = draggingMember.promotion;
                        if (typ === 'group') {
                          const name = draggingMember.member_group_name || `group-${draggingMember.member_group_id}`;
                          const maxAttempts = draggingMember.member_group_max_attempts ?? null;
                          return (
                            <div className="d-flex flex-column">
                              <div className="d-flex flex-wrap align-items-center gap-2">
                                <span className="fw-bold text-dark">{name}</span>
                                <span className="text-muted small">(渠道组)</span>
                                {showPromotion ? (
                                  <span className="small text-warning fw-medium">
                                    <i className="ri-fire-line me-1"></i>优先
                                  </span>
                                ) : null}
                              </div>
                              <div className="d-flex flex-wrap align-items-center gap-2 small text-muted mt-1">
                                <div className="d-flex align-items-center">
                                  <span className="me-1">max_attempts:</span>
                                  <span className="text-secondary font-monospace user-select-all">{maxAttempts ?? '-'}</span>
                                </div>
                              </div>
                            </div>
                          );
                        }
                        if (typ === 'channel') {
                          const dragOverlayChannel = {
                            id: draggingMember.member_channel_id || 0,
                            name: draggingMember.member_channel_name,
                            type: (draggingMember.member_channel_type || '').trim(),
                            promotion: draggingMember.promotion,
                            base_url: '',
                            groups: draggingMember.member_channel_groups || '',
                          };
                          return (
                            <div className="d-flex flex-column">
                              <div className="d-flex flex-wrap align-items-center gap-2">
                                <span className="fw-bold text-dark">{dragOverlayChannel.name || `渠道 #${dragOverlayChannel.id}`}</span>
                                <span className="text-muted small">({channelTypeLabel(dragOverlayChannel.type)})</span>
                                {dragOverlayChannel.promotion ? (
                                  <span className="small text-warning fw-medium">
                                    <i className="ri-fire-line me-1"></i>优先
                                  </span>
                                ) : null}
                              </div>
                              <div className="d-flex flex-wrap align-items-center gap-2 small text-muted mt-1">
                                {dragOverlayChannel.base_url ? (
                                  <span
                                    className="font-monospace d-inline-block user-select-all"
                                    style={{ maxWidth: 360, whiteSpace: 'nowrap', overflow: 'hidden', textOverflow: 'ellipsis' }}
                                    title={dragOverlayChannel.base_url}
                                  >
                                    {dragOverlayChannel.base_url}
                                  </span>
                                ) : null}
                                <div className="d-flex align-items-center">
                                  {dragOverlayChannel.base_url ? <span className="text-secondary">·</span> : null}
                                  <span className={`${dragOverlayChannel.base_url ? 'ms-2 ' : ''}me-1`}>渠道组:</span>
                                  <span className="text-secondary font-monospace user-select-all">{(dragOverlayChannel.groups || '').trim() || '-'}</span>
                                </div>
                              </div>
                            </div>
                          );
                        }
                        return <span className="text-muted small fst-italic">-</span>;
                      })()}
                    </td>
                    <td>
                      {(() => {
                        const typ = memberType(draggingMember);
                        if (typ === 'group') return <span className="badge rounded-pill bg-primary bg-opacity-10 text-primary px-2">子组</span>;
                        if (typ === 'channel') return <span className="badge rounded-pill bg-success bg-opacity-10 text-success px-2">渠道</span>;
                        return <span className="badge rounded-pill bg-secondary bg-opacity-10 text-secondary px-2">未知</span>;
                      })()}
                    </td>
                    <td>
                      {(() => {
                        const typ = memberType(draggingMember);
                        const st =
                          typ === 'group'
                            ? statusBadge(draggingMember.member_group_status ?? 0)
                            : typ === 'channel'
                              ? statusBadge(draggingMember.member_channel_status ?? 0)
                              : statusBadge(0);
                        return <span className={st.cls}>{st.label}</span>;
                      })()}
                    </td>
                    <td className="text-end pe-4 text-muted small">拖动中…</td>
                  </tr>
                </tbody>
              </table>
            </div>
          ) : null}
        </PortalDragOverlay>

      <div className="d-flex justify-content-between align-items-center mb-4">
        <div>
          <h3 className="mb-1 fw-bold">渠道组</h3>
          <nav aria-label="breadcrumb">
            <ol className="breadcrumb breadcrumb-sm mb-1">
              <li className="breadcrumb-item">
                <Link to="/admin/channel-groups">渠道组列表</Link>
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
        <div className="alert alert-warning">未找到该渠道组。</div>
      ) : (
        <div className="row g-4">
          <div className="col-12">
            <div className="card border-0 shadow-sm overflow-hidden mb-0">
              <div className="bg-primary bg-opacity-10 py-3 px-4 d-flex justify-content-between align-items-center">
                <div>
                  <span className="text-primary fw-bold text-uppercase small">成员列表</span>
                </div>
                <div className="text-primary text-opacity-75 small">
                  <i className="ri-drag-move-2-line me-1"></i> 支持拖拽排序
                </div>
              </div>
              <div className="table-responsive">
                <table className="table table-hover align-middle mb-0">
                    <thead className="table-light">
                      <tr>
                        <th style={{ width: 60 }}></th>
                        <th className="ps-4">成员详情</th>
                        <th>类型</th>
                        <th>状态</th>
                        <th className="text-end pe-4">操作</th>
                      </tr>
                    </thead>
                    <tbody>
                      <SortableContext items={memberIDs} strategy={verticalListSortingStrategy}>
                        {members.map((m) => (
                          <SortableRow key={m.member_id} id={m.member_id} disabled={{ draggable: loading || reordering, droppable: loading || reordering }}>
                            {({ attributes, listeners, setRowRef, transform, transition, isDragging, isOver }) => {
                              const typ = memberType(m);
                              const st =
                                typ === 'group'
                                  ? statusBadge(m.member_group_status ?? 0)
                                  : typ === 'channel'
                                    ? statusBadge(m.member_channel_status ?? 0)
                                    : statusBadge(0);
                              const isPointerChannel = typ === 'channel' && !!pointer && pointer.pinned && pointer.channel_id === m.member_channel_id;
                              const dropTarget = draggingID !== null && isOver && !isDragging;
                              const rowClassName = [
                                'rlm-channel-row-main',
                                dropTarget ? 'table-primary rlm-channel-row-drop-target' : isPointerChannel ? 'table-warning' : '',
                                isDragging ? 'rlm-channel-row-dragging' : '',
                              ]
                                .filter((v) => v)
                                .join(' ');
                              const style: CSSProperties = {
                                transform: CSS.Transform.toString(transform ? { ...transform, x: 0, scaleX: 1, scaleY: 1 } : null),
                                transition: reduceMotion ? undefined : transition ? `${transition}, background-color 0.12s ease, box-shadow 0.12s ease, opacity 0.12s ease` : undefined,
                                cursor: loading || reordering ? 'not-allowed' : isDragging ? 'grabbing' : 'grab',
                              };
                              return (
                                <tr
                                  ref={setRowRef}
                                  style={style}
                                  className={rowClassName || undefined}
                                  {...attributes}
                                  {...listeners}
                                  data-rlm-channel-group-member-row="1"
                                  data-rlm-member-id={m.member_id}
                                >
                                  <td className="text-center text-muted" title={loading || reordering ? '不可拖动' : '拖动排序'}>
                                    <span
                                      className="d-inline-flex align-items-center justify-content-center"
                                      style={{ width: 48, touchAction: loading || reordering ? 'auto' : isDragging ? 'none' : 'auto' }}
                                      aria-label="拖动排序"
                                      data-rlm-drag-handle="1"
                                      onClick={(e) => e.stopPropagation()}
                                    >
                                      <i className="ri-drag-move-2-line fs-5"></i>
                                    </span>
                                  </td>
                                  <td className="ps-4" style={{ minWidth: 0 }}>
                                    {typ === 'group' ? (
                                      <div className="d-flex flex-column">
                                        <div className="d-flex flex-wrap align-items-center gap-2">
                                          <Link className="fw-bold text-dark text-decoration-none" to={`/admin/channel-groups/${m.member_group_id}`}>
                                            {m.member_group_name || `group-${m.member_group_id}`}
                                          </Link>
                                          <span className="text-muted small">(渠道组)</span>
                                          {m.promotion ? (
                                            <span className="small text-warning fw-medium">
                                              <i className="ri-fire-line me-1"></i>优先
                                            </span>
                                          ) : null}
                                        </div>
                                        <div className="d-flex flex-wrap align-items-center gap-2 small text-muted mt-1">
                                          <div className="d-flex align-items-center">
                                            <span className="me-1">max_attempts:</span>
                                            <span className="text-secondary font-monospace user-select-all">{m.member_group_max_attempts ?? '-'}</span>
                                          </div>
                                        </div>
                                      </div>
                                    ) : typ === 'channel' ? (
                                      (() => {
                                        const ch = {
                                          id: m.member_channel_id || 0,
                                          name: m.member_channel_name,
                                          type: (m.member_channel_type || '').trim(),
                                          promotion: m.promotion,
                                          base_url: '',
                                          groups: m.member_channel_groups || '',
                                        };
                                        return (
                                          <div className="d-flex flex-column">
                                            <div className="d-flex flex-wrap align-items-center gap-2">
                                              <span className="fw-bold text-dark">{ch.name || `渠道 #${ch.id}`}</span>
                                              <span className="text-muted small">({channelTypeLabel(ch.type)})</span>
                                              {ch.promotion ? (
                                                <span className="small text-warning fw-medium">
                                                  <i className="ri-fire-line me-1"></i>优先
                                                </span>
                                              ) : null}
                                            </div>
                                            <div className="d-flex flex-wrap align-items-center gap-2 small text-muted mt-1">
                                              {ch.base_url ? (
                                                <span
                                                  className="font-monospace d-inline-block user-select-all"
                                                  style={{ maxWidth: 360, whiteSpace: 'nowrap', overflow: 'hidden', textOverflow: 'ellipsis' }}
                                                  title={ch.base_url}
                                                >
                                                  {ch.base_url}
                                                </span>
                                              ) : null}
                                              <div className="d-flex align-items-center">
                                                {ch.base_url ? <span className="text-secondary">·</span> : null}
                                                <span className={`${ch.base_url ? 'ms-2 ' : ''}me-1`}>渠道组:</span>
                                                <span className="text-secondary font-monospace user-select-all">{(ch.groups || '').trim() || '-'}</span>
                                              </div>
                                            </div>
                                          </div>
                                        );
                                      })()
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
                            }}
                          </SortableRow>
                        ))}
                        {members.length === 0 ? (
                          <tr>
                            <td colSpan={5} className="text-center py-5 text-muted">
                              暂无成员，请先添加渠道或创建子组。
                            </td>
                          </tr>
                        ) : null}
                      </SortableContext>
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
    </DndContext>
  );
}
