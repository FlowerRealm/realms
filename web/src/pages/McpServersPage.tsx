import { useEffect, useMemo, useState } from 'react';
import { Navigate } from 'react-router-dom';

import { parseAdminMcp, type McpServerV2 } from '../api/admin/mcp';
import { useAuth } from '../auth/AuthContext';
import { BootstrapModal } from '../components/BootstrapModal';
import { closeModalById } from '../components/modal';

import type { ImportPick, ImportSource, McpType, Row, TargetKey, UnionRow } from './mcp/mcpTypes';
import { chooseActualServer, equalServerCore, mainSummary, serverType, targetEnabledForServer, typeBadge } from './mcp/mcpUtils';
import { useMcpManager } from './mcp/useMcpManager';

type KVRow = { k: string; v: string };

type EditFormState = {
  id: string;
  type: McpType;

  command: string;
  args: string[];
  cwd: string;
  env: KVRow[];

  url: string;
  bearer_token_env_var: string;
  http_headers: KVRow[];

  startup_timeout_ms: string;
  tool_timeout_ms: string;
};

function parseTimeoutFieldMs(raw: string): number {
  const v = raw.trim();
  if (!v) return 0;
  if (!/^[0-9]+$/.test(v)) return -1;
  const n = Number(v);
  if (!Number.isFinite(n) || n < 0) return -1;
  return Math.floor(n);
}

function buildServer(type: McpType, form: EditFormState): McpServerV2 {
  const startup = parseTimeoutFieldMs(form.startup_timeout_ms);
  const tool = parseTimeoutFieldMs(form.tool_timeout_ms);
  const timeouts =
    (startup > 0 || tool > 0) && startup >= 0 && tool >= 0 ? { startup_ms: startup > 0 ? startup : undefined, tool_ms: tool > 0 ? tool : undefined } : undefined;

  if (type === 'stdio') {
    const args = form.args.map((x) => x.trim()).filter(Boolean);
    const env: Record<string, string> = {};
    for (const row of form.env) {
      const k = row.k.trim();
      const v = row.v;
      if (!k) continue;
      env[k] = v;
    }
    return {
      transport: 'stdio',
      stdio: {
        command: form.command.trim(),
        args: args.length ? args : undefined,
        cwd: form.cwd.trim() ? form.cwd.trim() : undefined,
        env: Object.keys(env).length ? env : undefined,
      },
      timeouts,
    };
  }

  const headers: Record<string, string> = {};
  for (const row of form.http_headers) {
    const k = row.k.trim();
    const v = row.v;
    if (!k) continue;
    headers[k] = v;
  }
  return {
    transport: type,
    http: {
      url: form.url.trim(),
      bearer_token_env_var: form.bearer_token_env_var.trim() ? form.bearer_token_env_var.trim() : undefined,
      headers: Object.keys(headers).length ? headers : undefined,
    },
    timeouts,
  };
}

function initForm(row: Row | null): EditFormState {
  const s = row?.server;
  const t = serverType(s);
  const env = s?.transport === 'stdio' ? s.stdio?.env : undefined;
  const headers = s?.transport === 'http' || s?.transport === 'sse' ? s.http?.headers : undefined;
  return {
    id: row?.id || '',
    type: t,

    command: s?.stdio?.command || '',
    args: s?.stdio?.args || [],
    cwd: s?.stdio?.cwd || '',
    env: Object.entries(env || {}).map(([k, v]) => ({ k, v })),

    url: s?.http?.url || '',
    bearer_token_env_var: s?.http?.bearer_token_env_var || '',
    http_headers: Object.entries(headers || {}).map(([k, v]) => ({ k, v })),

    startup_timeout_ms: s?.timeouts?.startup_ms ? String(s.timeouts.startup_ms) : '',
    tool_timeout_ms: s?.timeouts?.tool_ms ? String(s.timeouts.tool_ms) : '',
  };
}

function showModal(id: string) {
  const modalRoot = document.getElementById(id);
  const modalCtor = (window as Window & { bootstrap?: { Modal?: { getOrCreateInstance: (el: Element) => { show: () => void } } } }).bootstrap?.Modal;
  if (!modalRoot || !modalCtor?.getOrCreateInstance) return;
  modalCtor.getOrCreateInstance(modalRoot).show();
}

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
    showModal('mcpConflictModal');
  }, [conflictModalRequested, ackConflictModalRequested]);

  function openCreate() {
    setEditing(null);
    setForm(initForm(null));
    setCreateMode('import');
    setImportSource('claude');
    setImportContent('');
    showModal('mcpEditModal');
  }

  function openEditFromUnion(r: UnionRow) {
    const server = (r.desired || r.chosen) as McpServerV2 | undefined;
    if (!server) return;
    setEditing({ id: r.id, server });
    setForm(initForm({ id: r.id, server }));
    showModal('mcpEditModal');
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
        showModal('mcpImportConflictModal');
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

      <BootstrapModal
        id="mcpConflictModal"
        title="检测到冲突：请选择要保留的版本"
        dialogClassName="modal-lg modal-dialog-scrollable"
        footer={
          <>
            <button type="button" className="btn btn-light" data-bs-dismiss="modal">
              取消
            </button>
            <button type="button" className="btn btn-primary px-4" disabled={saving} onClick={() => void confirmConflictsAndClose()}>
              确认
            </button>
          </>
        }
        onHidden={() => {
          onConflictModalHidden();
        }}
      >
        <div className="text-muted small mb-2">默认选择“实际”（优先 codex，其次 claude、gemini）。</div>
        <div className="d-flex flex-column gap-3">
          {conflicts.map((id) => {
            const r = unionRows.find((x) => x.id === id);
            if (!r) return null;
            const desired = r.desired;
            const codex = r.actualByTarget.codex;
            const claude = r.actualByTarget.claude;
            const gemini = r.actualByTarget.gemini;
            const pick = conflictChoice[id] || 'desired';
            return (
              <div key={id} className="border rounded-3 p-3">
                <div className="d-flex justify-content-between align-items-center">
                  <div className="fw-semibold font-monospace">{id}</div>
                  <span className="badge bg-light text-danger border">conflict</span>
                </div>
                <div className="row g-2 mt-2">
                  {(['codex', 'claude', 'gemini'] as const).map((k) => {
                    const s = r.actualByTarget[k];
                    if (!s) return null;
                    return (
                      <div key={k} className="col-12 col-md-6">
                        <label className="form-check d-flex gap-2 align-items-start">
                          <input
                            className="form-check-input mt-1"
                            type="radio"
                            name={`conf-${id}`}
                            checked={pick === k}
                            onChange={() => setConflictChoice((p) => ({ ...p, [id]: k }))}
                          />
                          <div className="flex-grow-1">
                            <div className="fw-medium text-capitalize">{k} 实际</div>
                            <div className="text-muted small font-monospace text-truncate">{mainSummary(s)}</div>
                          </div>
                        </label>
                      </div>
                    );
                  })}
                  <div className="col-12">
                    <label className="form-check d-flex gap-2 align-items-start">
                      <input
                        className="form-check-input mt-1"
                        type="radio"
                        name={`conf-${id}`}
                        checked={pick === 'desired'}
                        onChange={() => setConflictChoice((p) => ({ ...p, [id]: 'desired' }))}
                      />
                      <div className="flex-grow-1">
                        <div className="fw-medium">Realms 记录</div>
                        <div className="text-muted small font-monospace text-truncate">{mainSummary(desired)}</div>
                      </div>
                    </label>
                  </div>
                  {!codex && !claude && !gemini ? <div className="text-muted small">未找到实际项，保留 Realms 记录。</div> : null}
                </div>
              </div>
            );
          })}
        </div>
      </BootstrapModal>

      <BootstrapModal
        id="mcpEditModal"
        title={editing ? `编辑：${editing.id}` : '新增 MCP'}
        dialogClassName="modal-lg modal-dialog-scrollable"
        footer={
          <>
            <button type="button" className="btn btn-light" data-bs-dismiss="modal">
              取消
            </button>
            {isPersonalBuild && !editing && createMode === 'import' ? (
              <button type="button" className="btn btn-primary px-4" disabled={saving} onClick={() => void startImport()}>
                导入并生效
              </button>
            ) : (
              <button type="button" className="btn btn-primary px-4" disabled={saving} onClick={() => saveFormToDesired()}>
                保存并生效
              </button>
            )}
          </>
        }
        onHidden={() => {
          setEditing(null);
          setForm(initForm(null));
          setCreateMode('import');
          setImportSource('claude');
          setImportContent('');
        }}
      >
        {isPersonalBuild && !editing ? (
          <div className="btn-group w-100 mb-3" role="group" aria-label="mcp-create-mode">
            <button type="button" className={`btn ${createMode === 'manual' ? 'btn-primary' : 'btn-outline-primary'}`} onClick={() => setCreateMode('manual')}>
              手动
            </button>
            <button type="button" className={`btn ${createMode === 'import' ? 'btn-primary' : 'btn-outline-primary'}`} onClick={() => setCreateMode('import')}>
              导入
            </button>
          </div>
        ) : null}

        {isPersonalBuild && !editing && createMode === 'import' ? (
          <div className="row g-3">
            <div className="col-12 col-lg-6">
              <label className="form-label">source</label>
              <select className="form-select" value={importSource} onChange={(e) => setImportSource(e.target.value as ImportSource)} disabled={saving}>
                <option value="claude">claude (JSON)</option>
                <option value="codex">codex (TOML/JSON)</option>
                <option value="gemini">gemini (JSON)</option>
                <option value="realms">realms (StoreV2 JSON)</option>
              </select>
            </div>
            <div className="col-12">
              <label className="form-label">content</label>
              <textarea
                className="form-control font-monospace"
                rows={12}
                value={importContent}
                onChange={(e) => setImportContent(e.target.value)}
                disabled={saving}
                placeholder={
                  importSource === 'codex'
                    ? `[mcp_servers.my-mcp]
command = "npx"
args = ["-y", "..."]`
                    : importSource === 'realms'
                      ? `{
  "version": 2,
  "servers": {
    "my-mcp": {
      "transport": "sse",
      "http": { "url": "http://127.0.0.1:9999/sse" }
    }
  }
}`
                      : `{
  "mcpServers": {
    "my-mcp": {
      "type": "stdio",
      "command": "npx",
      "args": ["-y", "..."]
    }
  }
}`
                }
              />
              <div className="form-text">默认合并（merge）。只导入 MCP servers；其它字段忽略。冲突会逐项要求你选择。</div>
            </div>
          </div>
        ) : (
          <div className="row g-3">
          <div className="col-12">
            <label className="form-label">id</label>
            <input className="form-control font-monospace" value={form.id} onChange={(e) => setForm((p) => ({ ...p, id: e.target.value }))} disabled={!!editing} placeholder="my-mcp" />
          </div>
          <div className="col-12">
            <label className="form-label">type</label>
            <select className="form-select" value={form.type} onChange={(e) => setForm((p) => ({ ...p, type: e.target.value as McpType }))}>
              <option value="stdio">stdio</option>
              <option value="http">http</option>
              <option value="sse">sse</option>
            </select>
          </div>

          {form.type === 'stdio' ? (
            <>
              <div className="col-12">
                <label className="form-label">command</label>
                <input className="form-control font-monospace" value={form.command} onChange={(e) => setForm((p) => ({ ...p, command: e.target.value }))} placeholder="npx @xxx/mcp" />
              </div>
              <div className="col-12">
                <label className="form-label">args（可选）</label>
                <input
                  className="form-control font-monospace"
                  value={(form.args || []).join(' ')}
                  onChange={(e) => setForm((p) => ({ ...p, args: (e.target.value || '').split(' ').filter(Boolean) }))}
                  placeholder="--foo bar"
                />
              </div>
              <div className="col-12">
                <label className="form-label">cwd（可选）</label>
                <input className="form-control font-monospace" value={form.cwd} onChange={(e) => setForm((p) => ({ ...p, cwd: e.target.value }))} placeholder="/path/to/project" />
              </div>
              <div className="col-12">
                <label className="form-label">env（可选）</label>
                <div className="d-flex flex-column gap-2">
                  {(form.env.length ? form.env : [{ k: '', v: '' }]).map((row, idx) => (
                    <div key={idx} className="row g-2 align-items-center">
                      <div className="col-md-5">
                        <input
                          className="form-control font-monospace"
                          value={row.k}
                          onChange={(e) =>
                            setForm((p) => {
                              const base = p.env.length ? p.env : [{ k: '', v: '' }];
                              const next = [...base];
                              next[idx] = { ...next[idx], k: e.target.value };
                              return { ...p, env: next };
                            })
                          }
                          placeholder="KEY"
                        />
                      </div>
                      <div className="col-md-5">
                        <input
                          className="form-control font-monospace"
                          value={row.v}
                          onChange={(e) =>
                            setForm((p) => {
                              const base = p.env.length ? p.env : [{ k: '', v: '' }];
                              const next = [...base];
                              next[idx] = { ...next[idx], v: e.target.value };
                              return { ...p, env: next };
                            })
                          }
                          placeholder="value"
                        />
                      </div>
                      <div className="col-md-2 d-grid">
                        <button
                          type="button"
                          className="btn btn-light border"
                          onClick={() =>
                            setForm((p) => {
                              const next = [...p.env];
                              next.splice(idx, 1);
                              return { ...p, env: next };
                            })
                          }
                        >
                          删除
                        </button>
                      </div>
                    </div>
                  ))}
                  <button type="button" className="btn btn-light border btn-sm align-self-start" onClick={() => setForm((p) => ({ ...p, env: [...(p.env || []), { k: '', v: '' }] }))}>
                    + 添加环境变量
                  </button>
                </div>
              </div>
            </>
          ) : (
            <>
              <div className="col-12">
                <label className="form-label">url</label>
                <input className="form-control font-monospace" value={form.url} onChange={(e) => setForm((p) => ({ ...p, url: e.target.value }))} placeholder="https://example.com/mcp" />
              </div>
              <div className="col-12">
                <label className="form-label">bearer_token_env_var（可选）</label>
                <input className="form-control font-monospace" value={form.bearer_token_env_var} onChange={(e) => setForm((p) => ({ ...p, bearer_token_env_var: e.target.value }))} placeholder="MY_TOKEN" />
              </div>
              <div className="col-12">
                <label className="form-label">http_headers（可选）</label>
                <div className="d-flex flex-column gap-2">
                  {(form.http_headers.length ? form.http_headers : [{ k: '', v: '' }]).map((row, idx) => (
                    <div key={idx} className="row g-2 align-items-center">
                      <div className="col-md-5">
                        <input
                          className="form-control font-monospace"
                          value={row.k}
                          onChange={(e) =>
                            setForm((p) => {
                              const base = p.http_headers.length ? p.http_headers : [{ k: '', v: '' }];
                              const next = [...base];
                              next[idx] = { ...next[idx], k: e.target.value };
                              return { ...p, http_headers: next };
                            })
                          }
                          placeholder="Header-Name"
                        />
                      </div>
                      <div className="col-md-5">
                        <input
                          className="form-control font-monospace"
                          value={row.v}
                          onChange={(e) =>
                            setForm((p) => {
                              const base = p.http_headers.length ? p.http_headers : [{ k: '', v: '' }];
                              const next = [...base];
                              next[idx] = { ...next[idx], v: e.target.value };
                              return { ...p, http_headers: next };
                            })
                          }
                          placeholder="value"
                        />
                      </div>
                      <div className="col-md-2 d-grid">
                        <button
                          type="button"
                          className="btn btn-light border"
                          onClick={() =>
                            setForm((p) => {
                              const next = [...p.http_headers];
                              next.splice(idx, 1);
                              return { ...p, http_headers: next };
                            })
                          }
                        >
                          删除
                        </button>
                      </div>
                    </div>
                  ))}
                  <button type="button" className="btn btn-light border btn-sm align-self-start" onClick={() => setForm((p) => ({ ...p, http_headers: [...(p.http_headers || []), { k: '', v: '' }] }))}>
                    + 添加 Header
                  </button>
                </div>
              </div>
            </>
          )}

          <div className="col-12">
            <details>
              <summary className="text-muted small">高级：Timeout（毫秒）</summary>
              <div className="row g-2 mt-1">
                <div className="col-12 col-md-6">
                  <label className="form-label small text-muted mb-1">startup_timeout_ms</label>
                  <input
                    className="form-control font-monospace"
                    value={form.startup_timeout_ms}
                    onChange={(e) => setForm((p) => ({ ...p, startup_timeout_ms: e.target.value }))}
                    placeholder="例如 60000"
                    disabled={saving}
                  />
                </div>
                <div className="col-12 col-md-6">
                  <label className="form-label small text-muted mb-1">tool_timeout_ms</label>
                  <input
                    className="form-control font-monospace"
                    value={form.tool_timeout_ms}
                    onChange={(e) => setForm((p) => ({ ...p, tool_timeout_ms: e.target.value }))}
                    placeholder="例如 600000"
                    disabled={saving}
                  />
                </div>
              </div>
            </details>
          </div>
        </div>
        )}
      </BootstrapModal>

      <BootstrapModal
        id="mcpImportConflictModal"
        title="导入冲突：逐项选择要保留的版本"
        dialogClassName="modal-lg modal-dialog-scrollable"
        footer={
          <>
            <button type="button" className="btn btn-light" data-bs-dismiss="modal">
              取消
            </button>
            <button type="button" className="btn btn-primary px-4" disabled={saving || importConfirmDisabled} onClick={() => void confirmImportConflicts()}>
              确认并生效
            </button>
          </>
        }
        onHidden={() => {
          setImportPending(null);
          setImportConflicts([]);
          setImportConflictChoice({});
        }}
      >
        <div className="text-muted small mb-2">没有默认选项：每一项都必须明确选择。</div>
        <div className="d-flex flex-column gap-3">
          {importConflicts.map((id) => {
            const desired = importPending?.desired?.[id];
            const imported = importPending?.imported?.[id];
            if (!desired || !imported) return null;
            const pick = importConflictChoice[id] || '';
            return (
              <div key={id} className="border rounded-3 p-3">
                <div className="d-flex justify-content-between align-items-center">
                  <div className="fw-semibold font-monospace">{id}</div>
                  <span className="badge bg-light text-danger border">conflict</span>
                </div>
                <div className="row g-2 mt-2">
                  <div className="col-12 col-md-6">
                    <label className="form-check d-flex gap-2 align-items-start">
                      <input
                        className="form-check-input mt-1"
                        type="radio"
                        name={`imp-${id}`}
                        checked={pick === 'keep'}
                        onChange={() => setImportConflictChoice((p) => ({ ...p, [id]: 'keep' }))}
                      />
                      <div className="flex-grow-1">
                        <div className="fw-medium">保留现有</div>
                        <div className="text-muted small font-monospace text-truncate">{mainSummary(desired)}</div>
                      </div>
                    </label>
                  </div>
                  <div className="col-12 col-md-6">
                    <label className="form-check d-flex gap-2 align-items-start">
                      <input
                        className="form-check-input mt-1"
                        type="radio"
                        name={`imp-${id}`}
                        checked={pick === 'imported'}
                        onChange={() => setImportConflictChoice((p) => ({ ...p, [id]: 'imported' }))}
                      />
                      <div className="flex-grow-1">
                        <div className="fw-medium">使用导入</div>
                        <div className="text-muted small font-monospace text-truncate">{mainSummary(imported)}</div>
                      </div>
                    </label>
                  </div>
                </div>
              </div>
            );
          })}
        </div>
      </BootstrapModal>
    </div>
  );
}
