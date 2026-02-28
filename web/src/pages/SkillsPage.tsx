import { useEffect, useMemo, useRef, useState } from 'react';
import { Navigate } from 'react-router-dom';

import type { SkillV1, SkillsStoreV1, SkillsTargetKey } from '../api/admin/skills';
import { useAuth } from '../auth/AuthContext';
import { BootstrapModal } from '../components/BootstrapModal';
import { closeModalById, showModalById } from '../components/modal';

import { useSkillsManager } from './skills/useSkillsManager';
import { SkillEditModal, type SkillEditDraft } from './skills/modals/SkillEditModal';
import { SkillsConflictModal, type ConflictPick } from './skills/modals/SkillsConflictModal';
import { buildUnionRows, conflictsFromUnion, skillPromptSummary, type SkillsUnionRow } from './skills/skillsUtils';

function initDraft(sk: SkillV1 | null): SkillEditDraft {
  const id = (sk?.id || '').trim();
  const per = sk?.per_target || {};
  const enabled = (k: 'codex' | 'claude' | 'gemini') => {
    const v = per?.[k]?.enabled;
    if (v === undefined) return true;
    return !!v;
  };
  return {
    id,
    title: (sk?.title || id).trim(),
    description: (sk?.description || '').trim(),
    prompt: (sk?.prompt || '').trim(),
    enabledCodex: enabled('codex'),
    enabledClaude: enabled('claude'),
    enabledGemini: enabled('gemini'),
  };
}

function draftToSkill(d: SkillEditDraft, originalSkill: SkillV1 | null): SkillV1 {
  const id = (d.id || '').trim();
  const title = (d.title || id).trim();
  const prompt = (d.prompt || '').trim();
  const desc = (d.description || '').trim();

  const out: SkillV1 = { id, title, prompt };
  if (desc) out.description = desc;

  const per: NonNullable<SkillV1['per_target']> = {};
  const originalPer = originalSkill?.per_target || {};
  const enabledFlags: Record<SkillsTargetKey, boolean> = {
    codex: d.enabledCodex,
    claude: d.enabledClaude,
    gemini: d.enabledGemini,
  };
  for (const target of ['codex', 'claude', 'gemini'] as const) {
    const opts = { ...(originalPer[target] || {}) };
    if (!enabledFlags[target]) opts.enabled = false;
    else delete opts.enabled;
    if (Object.keys(opts).length) per[target] = opts;
  }
  if (Object.keys(per).length) out.per_target = per;

  return out;
}

function setSkillTargetEnabled(skill: SkillV1, target: SkillsTargetKey, enabled: boolean): SkillV1 {
  const out: SkillV1 = { ...skill };
  const per: NonNullable<SkillV1['per_target']> = { ...(out.per_target || {}) };
  const cur: NonNullable<SkillV1['per_target']>[SkillsTargetKey] = { ...(per[target] || {}) };

  if (enabled) {
    delete cur.enabled;
  } else {
    cur.enabled = false;
  }

  if (Object.keys(cur).length === 0) delete per[target];
  else per[target] = cur;

  if (Object.keys(per).length === 0) {
    const { per_target: _perTarget, ...rest } = out;
    void _perTarget;
    return rest;
  }
  out.per_target = per;
  return out;
}

export function SkillsPage() {
  const { user, loading: authLoading } = useAuth();
  const isRoot = user?.role === 'root';
  const isReady = !authLoading;
  const isPersonalBuild = import.meta.env.MODE === 'personal';

  const {
    loading,
    scanning,
    saving,
    err,
    notice,
    setErr,
    setNotice,
    store,
    targets,
    desiredHashes,
    scanTargets,
    applyResults,
    conflicts,
    applyNow,
    scanNow,
    saveStore,
    importFrom,
    removeSkill,
  } = useSkillsManager();

  const [editing, setEditing] = useState<SkillV1 | null>(null);
  const [draft, setDraft] = useState<SkillEditDraft>(() => initDraft(null));

  const [createMode, setCreateMode] = useState<'manual' | 'import'>('import');
  const [importSource, setImportSource] = useState<SkillsTargetKey>('claude');
  const [importMode, setImportMode] = useState<'merge' | 'replace'>('merge');
  const [importApplyAfter, setImportApplyAfter] = useState(true);

  type ConfirmState = {
    title: string;
    message: string;
    confirmText: string;
    confirmBtnClass?: string;
    reopenModalId?: string;
    onConfirm: () => Promise<void>;
  };
  const [confirmState, setConfirmState] = useState<ConfirmState | null>(null);
  const confirming = useRef(false);
  const [confirmBusy, setConfirmBusy] = useState(false);
  const confirmAction = useRef<'none' | 'cancel' | 'ok'>('none');
  const confirmStateRef = useRef<ConfirmState | null>(null);

  useEffect(() => {
    confirmStateRef.current = confirmState;
  }, [confirmState]);

  const { unionRows, diffSummary } = useMemo(() => {
    return buildUnionRows({
      desiredSkills: (store?.skills || {}) as Record<string, SkillV1>,
      scanTargets: scanTargets || undefined,
      desiredHashes: desiredHashes || undefined,
      includeUnmanaged: false,
    });
  }, [store, scanTargets, desiredHashes]);

  const [conflictPicks, setConflictPicks] = useState<Record<string, ConflictPick>>({});
  const scanConflicts = useMemo(() => conflictsFromUnion(unionRows, desiredHashes), [unionRows, desiredHashes]);
  const activeConflicts = useMemo(() => (conflicts && conflicts.length ? conflicts : scanConflicts) || [], [conflicts, scanConflicts]);
  const conflictModalOpen = useRef(false);

  useEffect(() => {
    if (!activeConflicts || activeConflicts.length === 0) {
      conflictModalOpen.current = false;
      return;
    }
    if (conflictModalOpen.current) return;
    conflictModalOpen.current = true;
    const next: Record<string, ConflictPick> = {};
    for (const c of activeConflicts) next[`${c.target}:${c.id}`] = { action: 'keep' };
    setConflictPicks(next);
    showModalById('skillsConflictModal');
  }, [activeConflicts]);

  function openCreate() {
    setEditing(null);
    setDraft(initDraft(null));
    setCreateMode('import');
    setImportSource('claude');
    setImportMode('merge');
    setImportApplyAfter(true);
    showModalById('skillsEditModal');
  }

  function openConfirm(next: ConfirmState) {
    confirming.current = false;
    confirmAction.current = 'none';
    setConfirmBusy(false);
    setConfirmState(next);
    showModalById('skillsConfirmModal');
  }

  async function confirmOK() {
    if (!confirmState || confirming.current) return;
    confirming.current = true;
    setConfirmBusy(true);
    confirmAction.current = 'ok';
    closeModalById('skillsConfirmModal');
    try {
      await confirmState.onConfirm();
    } finally {
      confirming.current = false;
      setConfirmBusy(false);
    }
  }

  function confirmCancel() {
    confirmAction.current = 'cancel';
    closeModalById('skillsConfirmModal');
  }

  function openEditFromRow(r: SkillsUnionRow) {
    if (!r.desired) return;
    setEditing(r.desired);
    setDraft(initDraft(r.desired));
    setCreateMode('manual');
    showModalById('skillsEditModal');
  }

  async function confirmDelete(id: string) {
    openConfirm({
      title: '确认删除',
      message: `确认删除 skill：${id}？（会从 store 删除，并尝试删除目标文件）`,
      confirmText: '删除',
      confirmBtnClass: 'btn-danger',
      onConfirm: async () => {
        await removeSkill(id);
      },
    });
  }

  async function saveDraftToStore() {
    const sk = draftToSkill(draft, editing);
    if (!sk.id.trim()) {
      setErr('ID 不能为空');
      return;
    }
    if (!sk.title.trim()) {
      setErr('标题不能为空');
      return;
    }
    if (!sk.prompt.trim()) {
      setErr('Prompt 不能为空');
      return;
    }

    const next: SkillsStoreV1 = { version: store.version || 1, skills: { ...(store.skills || {}) } };
    if (!editing && next.skills[sk.id]) {
      setErr('ID 已存在');
      return;
    }
    const id = editing ? editing.id : sk.id;
    next.skills[id] = { ...sk, id };

    closeModalById('skillsEditModal');
    await saveStore(next, true);
  }

  async function doImport() {
    if (importMode === 'replace') {
      closeModalById('skillsEditModal');
      openConfirm({
        title: '确认 Replace 导入',
        message: `确认 replace 导入？这会用 ${importSource} 的实际内容覆盖 Realms 里的全部 skills。`,
        confirmText: '继续导入',
        confirmBtnClass: 'btn-danger',
        reopenModalId: 'skillsEditModal',
        onConfirm: async () => {
          await importFrom(importSource, importMode, importApplyAfter);
        },
      });
      return;
    }
    closeModalById('skillsEditModal');
    await importFrom(importSource, importMode, importApplyAfter);
  }

  async function setTargetEnabled(id: string, k: SkillsTargetKey, enabled: boolean) {
    setErr('');
    setNotice('');
    const cur = (store.skills || {})[id];
    if (!cur) return;
    const nextSkill = setSkillTargetEnabled(cur, k, enabled);
    const next: SkillsStoreV1 = { version: store.version || 1, skills: { ...(store.skills || {}) } };
    next.skills[id] = nextSkill;
    await saveStore(next, true);
  }

  if (!isReady) return null;
  if (!isRoot) return <Navigate to="/login" replace />;
  if (!isPersonalBuild) return <Navigate to="/admin/channels" replace />;

  const statusBadge = (st: SkillsUnionRow['status']) =>
    st === 'synced'
      ? 'bg-light text-success border'
      : st === 'new'
        ? 'bg-light text-primary border'
        : st === 'missing' || st === 'disabled'
          ? 'bg-light text-secondary border'
          : 'bg-light text-danger border';

  const cell = (r: SkillsUnionRow, k: SkillsTargetKey) => {
    if (!r.desired) return <span className="text-muted small">-</span>;
    const checked = r.enabledByTarget[k];
    return (
      <div className="form-check form-switch d-flex justify-content-center m-0">
        <input
          className="form-check-input"
          type="checkbox"
          role="switch"
          checked={checked}
          disabled={loading || saving}
          aria-label={`${r.id}:${k}`}
          onChange={(e) => void setTargetEnabled(r.id, k, e.target.checked)}
        />
      </div>
    );
  };

  return (
    <div className="fade-in-up">
      <BootstrapModal
        id="skillsConfirmModal"
        title={confirmState?.title || '确认操作'}
        footer={
          <div className="d-flex gap-2">
            <button className="btn btn-light border" type="button" onClick={() => confirmCancel()}>
              取消
            </button>
            <button
              className={`btn ${confirmState?.confirmBtnClass || 'btn-primary'}`.trim()}
              type="button"
              disabled={saving || confirmBusy}
              onClick={() => void confirmOK()}
            >
              {confirmState?.confirmText || '确认'}
            </button>
          </div>
        }
        onHidden={() => {
          const st = confirmStateRef.current;
          const act = confirmAction.current;
          confirmAction.current = 'none';
          setConfirmState(null);
          if (!st) return;

          // 用户点遮罩/右上角关闭：等同于取消。
          const shouldReopen = act !== 'ok';
          if (shouldReopen && st.reopenModalId) {
            setTimeout(() => showModalById(st.reopenModalId as string), 200);
          }
        }}
      >
        <div>{confirmState?.message || ''}</div>
      </BootstrapModal>

      <div className="d-flex justify-content-between align-items-center mb-4">
        <div>
          <h3 className="mb-1 fw-bold">Skills 管理</h3>
          <p className="text-muted small mb-0">自动读取当前生效的 skills/commands，并保持配置一致。</p>
        </div>
        <div className="d-flex gap-2">
          <button className="btn btn-light border" type="button" disabled={loading || saving || scanning} onClick={() => void scanNow()}>
            <span className="me-1 material-symbols-rounded">refresh</span> 刷新
          </button>
          <button className="btn btn-light border" type="button" disabled={loading || saving} onClick={() => openCreate()}>
            <span className="me-1 material-symbols-rounded">add</span> 新增
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

      {diffSummary.nConflict > 0 ? (
        <div className="alert alert-warning d-flex align-items-center mt-3" role="alert">
          <span className="me-2 material-symbols-rounded">report</span>
          <div className="flex-grow-1">检测到冲突，需要处理后才能继续对齐。</div>
        </div>
      ) : null}

      <div className="card mt-3">
        <div className="card-body">
          <div className="d-flex justify-content-between align-items-center">
            <div>
              <div className="fw-semibold">Skills</div>
              <div className="text-muted small">列表展示“当前生效”与“Realms 记录”的合并视图。</div>
            </div>
          </div>

          <div className="table-responsive mt-3">
            <table className="table table-hover align-middle mb-0">
              <thead className="table-light">
                <tr>
                  <th>ID</th>
                  <th>标题</th>
                  <th>摘要</th>
                  <th>Codex</th>
                  <th>Claude</th>
                  <th>Gemini</th>
                  <th>状态</th>
                  <th className="text-end">操作</th>
                </tr>
              </thead>
              <tbody>
                {loading ? (
                  <tr>
                    <td colSpan={8} className="text-center py-5 text-muted">
                      加载中…
                    </td>
                  </tr>
                ) : unionRows.length === 0 ? (
                  <tr>
                    <td colSpan={8} className="text-center py-5 text-muted">
                      暂无 Skills。
                    </td>
                  </tr>
                ) : (
                  unionRows.map((r) => {
                    const title = (r.desired?.title || r.id).trim();
                    const summary = r.desired
                      ? skillPromptSummary(r.desired)
                      : (r.actualByTarget.codex?.path || r.actualByTarget.claude?.path || r.actualByTarget.gemini?.path || '').trim();
                    return (
                      <tr key={r.id}>
                        <td className="font-monospace">{r.id}</td>
                        <td>{title}</td>
                        <td className="font-monospace text-truncate" style={{ maxWidth: 520 }}>
                          {summary}
                        </td>
                        <td>{cell(r, 'codex')}</td>
                        <td>{cell(r, 'claude')}</td>
                        <td>{cell(r, 'gemini')}</td>
                        <td>
                          <span className={`badge ${statusBadge(r.status)}`}>{r.status}</span>
                        </td>
                        <td className="text-end">
                          <>
                            <button className="btn btn-light border btn-sm me-2" type="button" onClick={() => openEditFromRow(r)} disabled={saving}>
                              编辑
                            </button>
                            <button className="btn btn-light border btn-sm" type="button" onClick={() => void confirmDelete(r.id)} disabled={saving}>
                              删除
                            </button>
                          </>
                        </td>
                      </tr>
                    );
                  })
                )}
              </tbody>
            </table>
          </div>

          <details className="mt-3">
            <summary className="text-muted small">排障信息</summary>
            <div className="mt-2 small">
              <div className="text-muted">仅用于排查读取失败/落盘失败等问题。</div>
              <div className="mt-2">
                {(['codex', 'claude', 'gemini'] as const).map((k) => {
                  const info = (targets || {})[k];
                  const scan = (scanTargets || {})[k];
                  return (
                    <div key={k} className="mt-3">
                      <div className="fw-semibold text-capitalize">{k}</div>
                      <div className="text-muted font-monospace text-truncate">{info?.path || scan?.path || '-'}</div>
                      <div className="text-muted">exists: {info?.exists || scan?.exists ? 'yes' : 'no'}</div>
                      <div className="text-muted">enabled: {info?.enabled ? 'yes' : 'no'}</div>
                      <div className="text-muted">count: {scan?.skill_count ?? '-'}</div>
                      {scan?.parse_error ? <div className="text-danger">parse_error: {scan.parse_error}</div> : null}
                    </div>
                  );
                })}
              </div>

              {applyResults.length ? (
                <div className="mt-3">
                  <div className="fw-semibold">最近 apply 结果</div>
                  <div className="table-responsive mt-2">
                    <table className="table table-sm">
                      <thead className="table-light">
                        <tr>
                          <th>ID</th>
                          <th>Target</th>
                          <th>Path</th>
                          <th>Changed</th>
                          <th>Error</th>
                        </tr>
                      </thead>
                      <tbody>
                        {applyResults.map((r, idx) => (
                          <tr key={`${r.target}:${r.id}:${idx}`}>
                            <td className="font-monospace">{r.id}</td>
                            <td className="text-capitalize">{r.target}</td>
                            <td className="font-monospace text-truncate" style={{ maxWidth: 520 }}>
                              {r.path}
                            </td>
                            <td>{r.changed ? 'yes' : 'no'}</td>
                            <td className="text-danger">{r.error || ''}</td>
                          </tr>
                        ))}
                      </tbody>
                    </table>
                  </div>
                </div>
              ) : null}
            </div>
          </details>
        </div>
      </div>

      <SkillEditModal
        saving={saving}
        editing={editing}
        setEditing={setEditing}
        draft={draft}
        setDraft={setDraft}
        initDraft={initDraft}
        createMode={createMode}
        setCreateMode={setCreateMode}
        importSource={importSource}
        setImportSource={setImportSource}
        importMode={importMode}
        setImportMode={setImportMode}
        importApplyAfter={importApplyAfter}
        setImportApplyAfter={setImportApplyAfter}
        onImport={() => void doImport()}
        onSave={() => void saveDraftToStore()}
      />

      <SkillsConflictModal
        saving={saving}
        conflicts={activeConflicts}
        picks={conflictPicks}
        setPick={(key, pick) => setConflictPicks((p) => ({ ...p, [key]: pick }))}
        onConfirm={() =>
          void (async () => {
            const resolutions = Object.entries(conflictPicks).map(([k, v]) => {
              const [target, id] = k.split(':');
              return { target, id, action: v.action, name: v.name };
            });
            await applyNow({ resolutions });
            closeModalById('skillsConflictModal');
            conflictModalOpen.current = false;
          })()
        }
        onHidden={() => {
          setConflictPicks({});
          conflictModalOpen.current = false;
        }}
      />
    </div>
  );
}
