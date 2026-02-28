import { useEffect, useMemo, useState } from 'react';
import { Navigate } from 'react-router-dom';

import { parseAdminMcp, type McpServerV2 } from '../api/admin/mcp';
import { useAuth } from '../auth/AuthContext';
import { closeModalById, showModalById } from '../components/modal';

import type { ImportPick, ImportSource, Row, TargetKey, UnionRow } from './mcp/mcpTypes';
import { chooseActualServer, equalServerCore, mainSummary, serverType, targetEnabledForServer, typeBadge } from './mcp/mcpUtils';
import { buildServer, initForm, parseTimeoutFieldMs, type EditFormState } from './mcp/mcpEditForm';
import { useMcpManager } from './mcp/useMcpManager';
import { McpConflictModal } from './mcp/modals/McpConflictModal';
import { McpImportConflictModal } from './mcp/modals/McpImportConflictModal';
import { McpServerEditModal } from './mcp/modals/McpServerEditModal';

export function McpServersPage() {
  const { user, loading: authLoading } = useAuth();
  const isRoot = user?.role === 'root';
  const isReady = !authLoading;
  const isPersonalBuild = import.meta.env.MODE === 'personal';

  const {
    loading,
    scanning,
    saving: mcpSaving,
    applyResults,
    err,
    notice,
    setErr,
    setNotice,
    targetInfo,
    desiredServers,
    unionRows,
    diffSummary,
    conflicts,
    conflictChoice,
    setConflictChoice,
    conflictModalRequested,
    ackConflictModalRequested,
    onConflictModalHidden,
    scanNow,
    saveDesired,
    removeServer,
    confirmConflicts: confirmConflictsCore,
    getDesiredServersSnapshot,
  } = useMcpManager();

  const [importBusy, setImportBusy] = useState(false);
  const saving = mcpSaving || importBusy;

  const [editing, setEditing] = useState<Row | null>(null);
  const [form, setForm] = useState<EditFormState>(() => initForm(null));

  const [createMode, setCreateMode] = useState<'manual' | 'import'>('import');
  const [importSource, setImportSource] = useState<ImportSource>('claude');
  const [importContent, setImportContent] = useState('');
  const [importPending, setImportPending] = useState<null | { desired: Record<string, McpServerV2>; imported: Record<string, McpServerV2> }>(null);
  const [importConflicts, setImportConflicts] = useState<string[]>([]);
  const [importConflictChoice, setImportConflictChoice] = useState<Record<string, ImportPick>>({});

  useEffect(() => {
    if (!conflictModalRequested) return;
    ackConflictModalRequested();
    showModalById('mcpConflictModal');
  }, [conflictModalRequested, ackConflictModalRequested]);

  function openCreate() {
    setEditing(null);
    setForm(initForm(null));
    setCreateMode('import');
    setImportSource('claude');
    setImportContent('');
    showModalById('mcpEditModal');
  }

  function openEditFromUnion(r: UnionRow) {
    const server = (r.desired || r.chosen) as McpServerV2 | undefined;
    if (!server) return;
    setEditing({ id: r.id, server });
    setForm(initForm({ id: r.id, server }));
    showModalById('mcpEditModal');
  }

  async function confirmConflictsAndClose() {
    await confirmConflictsCore();
    closeModalById('mcpConflictModal');
  }

  function setServerTargetEnabled(s: McpServerV2, k: TargetKey, enabled: boolean): McpServerV2 {
    const targets = { ...(s.targets || {}) } as Partial<Record<TargetKey, boolean>>;
    if (enabled) delete targets[k];
    else targets[k] = false;
    const out: McpServerV2 = { ...s };
    if (Object.keys(targets).length === 0) {
      const { targets: _targets, ...rest } = out;
      void _targets;
      return rest as McpServerV2;
    }
    out.targets = targets;
    return out;
  }

  function setServerTarget(id: string, k: TargetKey, enabled: boolean) {
    void (async () => {
      setErr('');
      setNotice('');

      const curDesired = getDesiredServersSnapshot()[id];
      const r = unionRows.find((x) => x.id === id);
      const actualByTarget = r?.actualByTarget || {};

      if (!curDesired) {
        const base = actualByTarget[k] || chooseActualServer(actualByTarget);
        if (!base) {
          setErr('无法找到可用的实际配置');
          return;
        }

        const present: Record<TargetKey, boolean> = {
          codex: !!actualByTarget.codex,
          claude: !!actualByTarget.claude,
          gemini: !!actualByTarget.gemini,
        };
        const disable: Partial<Record<TargetKey, boolean>> = {};
        for (const t of ['codex', 'claude', 'gemini'] as const) {
          const wantEnabled = t === k ? enabled : present[t];
          if (!wantEnabled) disable[t] = false;
        }

        const nextServer = Object.keys(disable).length ? { ...base, targets: disable } : base;
        const baseDesired = getDesiredServersSnapshot();
        const next = { ...(baseDesired || {}) };
        next[id] = nextServer;
        await saveDesired(next);
        return;
      }

      const nextServer = setServerTargetEnabled(curDesired, k, enabled);
      const baseDesired = getDesiredServersSnapshot();
      const next = { ...(baseDesired || {}) };
      next[id] = nextServer;
      await saveDesired(next);
    })();
  }

  function saveFormToDesired() {
    const id = form.id.trim();
    if (!id) {
      setErr('ID 不能为空');
      return;
    }
    const t = form.type;
    if (t === 'stdio' && !form.command.trim()) {
      setErr('command 不能为空');
      return;
    }
    if ((t === 'http' || t === 'sse') && !form.url.trim()) {
      setErr('url 不能为空');
      return;
    }
    const startup = parseTimeoutFieldMs(form.startup_timeout_ms);
    const tool = parseTimeoutFieldMs(form.tool_timeout_ms);
    if (startup < 0 || tool < 0) {
      setErr('timeout 必须是非负整数（毫秒）');
      return;
    }
    const server = buildServer(t, form);

    const next = { ...(desiredServers || {}) };
    next[id] = server;
    closeModalById('mcpEditModal');
    void saveDesired(next);
  }

  function computeImportConflictIDs(desired: Record<string, McpServerV2>, imported: Record<string, McpServerV2>): string[] {
    const out: string[] = [];
    for (const [id, sv] of Object.entries(imported || {})) {
      const cur = desired?.[id];
      if (!cur) continue;
      if (!equalServerCore(cur, sv)) out.push(id);
    }
    return out.sort((a, b) => a.localeCompare(b));
  }

  function buildNextDesiredFromImport(p: NonNullable<typeof importPending>, choices: Record<string, ImportPick>): Record<string, McpServerV2> {
    const conflictIDs = computeImportConflictIDs(p.desired, p.imported);
    const conflictSet = new Set(conflictIDs);

    const out: Record<string, McpServerV2> = { ...(p.desired || {}) };
    for (const [id, sv] of Object.entries(p.imported || {})) {
      if (conflictSet.has(id)) continue;
      const keepTargets = p.desired?.[id]?.targets;
      out[id] = keepTargets ? { ...sv, targets: keepTargets } : sv;
    }
    for (const id of conflictIDs) {
      const pick = choices[id];
      if (pick === 'keep') {
        if (p.desired[id]) out[id] = p.desired[id];
      } else if (pick === 'imported') {
        if (p.imported[id]) {
          const keepTargets = p.desired?.[id]?.targets;
          out[id] = keepTargets ? { ...p.imported[id], targets: keepTargets } : p.imported[id];
        }
      }
    }
    return out;
  }

  const importConfirmDisabled = useMemo(() => {
    if (!importPending) return true;
    for (const id of importConflicts) {
      if (!importConflictChoice[id]) return true;
    }
    return false;
  }, [importPending, importConflicts, importConflictChoice]);

  async function startImport() {
    const content = (importContent || '').trim();
    if (!content) {
      setErr('导入内容不能为空');
      return;
    }
    setErr('');
    setNotice('');
    setImportBusy(true);
    try {
      const res = await parseAdminMcp({ source: importSource, content });
      if (!res.success) throw new Error(res.message || '解析失败');
      const imported = ((res.data?.store?.servers || {}) as Record<string, McpServerV2>) || {};
      const desired = getDesiredServersSnapshot();

      const conflictIDs = computeImportConflictIDs(desired, imported);
      if (conflictIDs.length > 0) {
        setImportPending({ desired, imported });
        setImportConflicts(conflictIDs);
        setImportConflictChoice({});
        closeModalById('mcpEditModal');
        showModalById('mcpImportConflictModal');
        return;
      }

      const next: Record<string, McpServerV2> = { ...(desired || {}) };
      for (const [id, sv] of Object.entries(imported || {})) {
        const keepTargets = desired?.[id]?.targets;
        next[id] = keepTargets ? { ...sv, targets: keepTargets } : sv;
      }
      closeModalById('mcpEditModal');
      await saveDesired(next);
    } catch (e) {
      setErr(e instanceof Error ? e.message : '解析失败');
    } finally {
      setImportBusy(false);
    }
  }

  async function confirmImportConflicts() {
    if (!importPending) return;
    if (importConfirmDisabled) return;
    const next = buildNextDesiredFromImport(importPending, importConflictChoice);
    closeModalById('mcpImportConflictModal');
    setImportPending(null);
    setImportConflicts([]);
    setImportConflictChoice({});
    await saveDesired(next);
  }

  if (!isReady) return null;
  if (!isRoot) return <Navigate to="/dashboard" replace />;

  return (
    <div className="fade-in-up">
      <div className="d-flex justify-content-between align-items-center mb-4">
        <div>
          <h3 className="mb-1 fw-bold">MCP 集成</h3>
          <p className="text-muted small mb-0">自动读取当前生效的工具集成，并保持配置一致。</p>
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

      {/* per-server per-target toggles live in the table below */}

      {diffSummary.nConflict > 0 ? (
        <div className="alert alert-warning d-flex align-items-center mt-3" role="alert">
          <span className="me-2 material-symbols-rounded">report</span>
          <div className="flex-grow-1">检测到冲突，需要确认后才能继续自动对齐。</div>
        </div>
      ) : null}

      <div className="card mt-3">
        <div className="card-body">
          <div className="d-flex justify-content-between align-items-center">
            <div>
              <div className="fw-semibold">MCP 服务器</div>
              <div className="text-muted small">列表展示“当前生效”与“Realms 记录”的合并视图。</div>
            </div>
          </div>

          <div className="table-responsive mt-3">
            <table className="table table-hover align-middle mb-0">
              <thead className="table-light">
                <tr>
                  <th>ID</th>
                  <th>类型</th>
                  <th>命令 / URL</th>
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
                      暂无 MCP。
                    </td>
                  </tr>
                ) : (
                  unionRows.map((r) => {
                    const chosen = r.chosen || r.desired;
                    const t = serverType(chosen);
                    const st = r.status;
                    const statusBadge =
                      st === 'synced'
                        ? 'bg-light text-success border'
                        : st === 'new'
                          ? 'bg-light text-primary border'
                          : st === 'missing' || st === 'disabled'
                            ? 'bg-light text-secondary border'
                            : 'bg-light text-danger border';

                    const cell = (k: TargetKey) => {
                      const desired = r.desired;
                      const enabled = desired ? targetEnabledForServer(desired, k) : !!r.actualByTarget[k];
                      return (
                        <div className="form-check form-switch d-flex justify-content-center m-0">
                          <input
                            className="form-check-input"
                            type="checkbox"
                            role="switch"
                            checked={enabled}
                            disabled={loading || saving}
                            aria-label={`${r.id}:${k}`}
                            onChange={(e) => setServerTarget(r.id, k, e.target.checked)}
                          />
                        </div>
                      );
                    };

                    return (
                      <tr key={r.id}>
                        <td className="font-monospace">{r.id}</td>
                        <td>
                          <span className={typeBadge(t)}>{t}</span>
                        </td>
                        <td className="font-monospace text-truncate" style={{ maxWidth: 520 }}>
                          {mainSummary(chosen)}
                        </td>
                        <td>{cell('codex')}</td>
                        <td>{cell('claude')}</td>
                        <td>{cell('gemini')}</td>
                        <td>
                          <span className={`badge ${statusBadge}`}>{st}</span>
                        </td>
                        <td className="text-end">
                          <button className="btn btn-light border btn-sm me-2" type="button" onClick={() => openEditFromUnion(r)}>
                            编辑
                          </button>
                          <button className="btn btn-light border btn-sm" type="button" onClick={() => removeServer(r.id)}>
                            删除
                          </button>
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
              <div className="text-muted">仅用于排查集成读取失败等问题。</div>
              <div className="mt-2">
                {(['codex', 'claude', 'gemini'] as const).map((k) => {
                  const info = targetInfo[k];
                  return (
                    <div key={k} className="d-flex flex-column flex-md-row justify-content-between align-items-start align-items-md-center py-1">
                      <div className="fw-medium text-capitalize">{k}</div>
                      <div className="text-muted font-monospace">{info.path || '(未解析路径)'}</div>
                      <div className="text-muted">{info.exists ? 'exists' : 'missing'}</div>
                      <div className="text-danger">{(info.parse_error || '').trim() ? 'parse error' : ''}</div>
                    </div>
                  );
                })}
              </div>
            </div>

            {applyResults.length ? (
              <div className="mt-3">
                <div className="fw-semibold mb-2">最近同步结果</div>
                <div className="table-responsive">
                  <table className="table table-sm table-hover align-middle mb-0">
                    <thead className="table-light">
                      <tr>
                        <th>目标</th>
                        <th>状态</th>
                        <th className="text-end">变更</th>
                      </tr>
                    </thead>
                    <tbody>
                      {applyResults.map((r, idx) => (
                        <tr key={idx}>
                          <td className="font-monospace">{r.target}</td>
                          <td>
                            {r.error ? <span className="text-danger">{r.error}</span> : r.enabled ? <span className="text-success">ok</span> : <span className="text-muted">disabled</span>}
                          </td>
                          <td className="text-end">
                            <span className={`badge ${r.changed ? 'bg-light text-primary border' : 'bg-light text-secondary border'}`}>{r.changed ? 'changed' : 'no-op'}</span>
                          </td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              </div>
            ) : null}
          </details>
        </div>
      </div>

      <McpConflictModal
        saving={saving}
        conflicts={conflicts}
        unionRows={unionRows}
        conflictChoice={conflictChoice}
        setConflictChoice={setConflictChoice}
        onConfirm={() => void confirmConflictsAndClose()}
        onHidden={onConflictModalHidden}
      />

      <McpServerEditModal
        isPersonalBuild={isPersonalBuild}
        saving={saving}
        editing={editing}
        setEditing={setEditing}
        form={form}
        setForm={setForm}
        initForm={initForm}
        createMode={createMode}
        setCreateMode={setCreateMode}
        importSource={importSource}
        setImportSource={setImportSource}
        importContent={importContent}
        setImportContent={setImportContent}
        onImport={() => void startImport()}
        onSave={saveFormToDesired}
        onHiddenReset={() => {}}
      />

      <McpImportConflictModal
        saving={saving}
        importPending={importPending}
        importConflicts={importConflicts}
        importConflictChoice={importConflictChoice}
        setImportConflictChoice={setImportConflictChoice}
        importConfirmDisabled={importConfirmDisabled}
        onConfirm={() => void confirmImportConflicts()}
        onHidden={() => {
          setImportPending(null);
          setImportConflicts([]);
          setImportConflictChoice({});
        }}
      />
    </div>
  );
}
