import { useEffect, useMemo, useRef, useState } from 'react';
import { Navigate } from 'react-router-dom';

import { deleteAdminMcp, getAdminMcp, updateAdminMcp, scanAdminMcp, type AdminMcpApplyResult, type McpServerV2, type McpStoreV2 } from '../api/admin/mcp';
import { useAuth } from '../auth/AuthContext';
import { BootstrapModal } from '../components/BootstrapModal';
import { closeModalById } from '../components/modal';
import { SegmentedFrame } from '../components/SegmentedFrame';

type TargetKey = 'codex' | 'claude' | 'gemini';
type McpType = 'stdio' | 'http' | 'sse';

type Row = {
  id: string;
  server: McpServerV2;
};

type PerTarget = Partial<Record<TargetKey, McpServerV2>>;

type UnionRow = {
  id: string;
  desired?: McpServerV2;
  chosen?: McpServerV2;
  actualByTarget: PerTarget;
  hasActual: boolean;
  actualConflict: boolean;
  status: 'synced' | 'new' | 'missing' | 'conflict';
};

function typeBadge(t: string) {
  if (t === 'stdio') return 'badge bg-light text-primary border';
  if (t === 'http') return 'badge bg-light text-success border';
  if (t === 'sse') return 'badge bg-light text-info border';
  return 'badge bg-light text-secondary border';
}

function serverType(s: McpServerV2 | undefined): McpType {
  if (!s) return 'stdio';
  return s.transport;
}

function mainSummary(s: McpServerV2 | undefined): string {
  if (!s) return '-';
  const t = serverType(s);
  if (t === 'stdio') return s.stdio?.command || '-';
  return s.http?.url || '-';
}

function stableStringify(v: unknown): string {
  const seen = new WeakSet<object>();
  const walk = (x: unknown): unknown => {
    if (!x || typeof x !== 'object') return x;
    if (seen.has(x as object)) return null;
    seen.add(x as object);
    if (Array.isArray(x)) return x.map(walk);
    const obj = x as Record<string, unknown>;
    const keys = Object.keys(obj).sort();
    const out: Record<string, unknown> = {};
    for (const k of keys) out[k] = walk(obj[k]);
    return out;
  };
  return JSON.stringify(walk(v));
}

function equalSpec(a: unknown, b: unknown): boolean {
  return stableStringify(a) === stableStringify(b);
}

function stableHash(v: unknown): string {
  // Simple stable hash for "did something materially change" checks.
  // Not cryptographic; just avoids repeated auto-fix loops.
  const s = stableStringify(v);
  let h = 2166136261;
  for (let i = 0; i < s.length; i += 1) {
    h ^= s.charCodeAt(i);
    h = Math.imul(h, 16777619);
  }
  return (h >>> 0).toString(16);
}

function chooseActualServer(actualByTarget: PerTarget): McpServerV2 | undefined {
  for (const k of ['codex', 'claude', 'gemini'] as const) {
    const s = actualByTarget[k];
    if (s) return s;
  }
  return undefined;
}

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

  const [loading, setLoading] = useState(true);
  const [scanning, setScanning] = useState(false);
  const [saving, setSaving] = useState(false);
  const [applyResults, setApplyResults] = useState<AdminMcpApplyResult[]>([]);
  const initialScanDone = useRef(false);
  const lastAutoFixSig = useRef<string>('');
  const conflictModalOpen = useRef(false);
  const desiredServersRef = useRef<Record<string, McpServerV2>>({});
  const targetEnabledRef = useRef<Record<TargetKey, boolean>>({ codex: true, claude: true, gemini: true });

  const [err, setErr] = useState('');
  const [notice, setNotice] = useState('');

  const [targetEnabled, setTargetEnabled] = useState<Record<TargetKey, boolean>>({ codex: true, claude: true, gemini: true });
  const [targetInfo, setTargetInfo] = useState<Record<TargetKey, { path: string; exists: boolean; parse_error?: string; server_count?: number }>>({
    codex: { path: '', exists: false },
    claude: { path: '', exists: false },
    gemini: { path: '', exists: false },
  });
  const [scannedTargets, setScannedTargets] = useState<Record<TargetKey, { servers?: Record<string, McpServerV2> }>>({
    codex: {},
    claude: {},
    gemini: {},
  });

  const [desiredServers, setDesiredServers] = useState<Record<string, McpServerV2>>({});

  const [editing, setEditing] = useState<Row | null>(null);
  const [form, setForm] = useState<EditFormState>(() => initForm(null));

  const [conflicts, setConflicts] = useState<string[]>([]);
  const [conflictChoice, setConflictChoice] = useState<Record<string, 'codex' | 'claude' | 'gemini' | 'desired'>>({});

  useEffect(() => {
    desiredServersRef.current = desiredServers || {};
  }, [desiredServers]);

  useEffect(() => {
    targetEnabledRef.current = targetEnabled;
  }, [targetEnabled]);

  const unionRows = useMemo(() => {
    const ids = new Set<string>();
    for (const id of Object.keys(desiredServers || {})) ids.add(id);
    for (const t of ['codex', 'claude', 'gemini'] as const) {
      const servers = (scannedTargets[t]?.servers || {}) as Record<string, McpServerV2>;
      for (const id of Object.keys(servers)) ids.add(id);
    }

    const out: UnionRow[] = [];
    for (const id of Array.from(ids)) {
      const desired = desiredServers[id];
      const actualByTarget: PerTarget = {};
      for (const t of ['codex', 'claude', 'gemini'] as const) {
        const servers = (scannedTargets[t]?.servers || {}) as Record<string, McpServerV2>;
        const sv = servers[id];
        if (sv && typeof sv === 'object') actualByTarget[t] = sv;
      }
      const chosen = chooseActualServer(actualByTarget);
      const hasActual = !!chosen;
      const actualSpecs = Object.values(actualByTarget);
      const actualConflict = (() => {
        if (actualSpecs.length <= 1) return false;
        const first = actualSpecs[0];
        for (const s of actualSpecs.slice(1)) {
          if (!equalSpec(first, s)) return true;
        }
        return false;
      })();

      let status: UnionRow['status'] = 'synced';
      if (!hasActual && desired) status = 'missing';
      else if (hasActual && !desired) status = 'new';
      else if (!hasActual && !desired) continue;
      else if (actualConflict) status = 'conflict';
      else if (desired && chosen && !equalSpec(desired, chosen)) status = 'conflict';
      else status = 'synced';

      out.push({
        id,
        desired,
        chosen,
        actualByTarget,
        hasActual,
        actualConflict,
        status,
      });
    }
    return out.sort((a, b) => a.id.localeCompare(b.id));
  }, [desiredServers, scannedTargets]);

  const diffSummary = useMemo(() => {
    let nNew = 0;
    let nMissing = 0;
    let nConflict = 0;
    for (const r of unionRows) {
      if (r.status === 'new') nNew += 1;
      if (r.status === 'missing') nMissing += 1;
      if (r.status === 'conflict') nConflict += 1;
    }
    return { nNew, nMissing, nConflict };
  }, [unionRows]);

  function getParseOKTargets(info: Record<TargetKey, { parse_error?: string }>): Record<TargetKey, boolean> {
    return {
      codex: !(info.codex?.parse_error || '').trim(),
      claude: !(info.claude?.parse_error || '').trim(),
      gemini: !(info.gemini?.parse_error || '').trim(),
    };
  }

  function buildActualByID(
    targets: Record<TargetKey, { servers?: Record<string, McpServerV2> }>,
    parseOK: Record<TargetKey, boolean>,
  ): Record<string, PerTarget> {
    const out: Record<string, PerTarget> = {};
    for (const t of ['codex', 'claude', 'gemini'] as const) {
      if (!parseOK[t]) continue;
      const servers = (targets[t]?.servers || {}) as Record<string, McpServerV2>;
      for (const [id, sv] of Object.entries(servers)) {
        if (!id.trim()) continue;
        if (!sv || typeof sv !== 'object') continue;
        out[id] = out[id] || {};
        out[id][t] = sv;
      }
    }
    return out;
  }

  function computeConflictsAndAutoMerge(
    desired: Record<string, McpServerV2>,
    targets: Record<TargetKey, { servers?: Record<string, McpServerV2> }>,
    info: Record<TargetKey, { parse_error?: string }>,
  ): { conflictIDs: string[]; nextDesired: Record<string, McpServerV2>; defaultChoice: Record<string, 'codex' | 'claude' | 'gemini' | 'desired'>; hasAnyFix: boolean } {
    const parseOK = getParseOKTargets(info);
    const actualByID = buildActualByID(targets, parseOK);

    const nextDesired: Record<string, McpServerV2> = { ...(desired || {}) };
    const conflictIDs: string[] = [];
    const defaultChoice: Record<string, 'codex' | 'claude' | 'gemini' | 'desired'> = {};

    // Merge new IDs (no user action).
    for (const [id, per] of Object.entries(actualByID)) {
      if (Object.prototype.hasOwnProperty.call(nextDesired, id)) continue;
      const chosen = chooseActualServer(per);
      if (chosen) nextDesired[id] = chosen;
    }

    // Conflicts: (1) actual differs across targets; (2) actual differs from desired.
    for (const [id, per] of Object.entries(actualByID)) {
      const actualSpecs = Object.values(per);
      const desiredSpec = nextDesired[id];
      let conflict = false;
      if (actualSpecs.length > 1) {
        const first = actualSpecs[0];
        for (const s of actualSpecs.slice(1)) {
          if (!equalSpec(first, s)) {
            conflict = true;
            break;
          }
        }
      }
      const chosen = chooseActualServer(per);
      if (!conflict && chosen && desiredSpec && !equalSpec(desiredSpec, chosen)) conflict = true;
      if (!conflict) continue;
      conflictIDs.push(id);
      if (per.codex) defaultChoice[id] = 'codex';
      else if (per.claude) defaultChoice[id] = 'claude';
      else if (per.gemini) defaultChoice[id] = 'gemini';
      else defaultChoice[id] = 'desired';
    }

    // Missing-only differences: desired has IDs not in parse-ok actual; these can be fixed by applying desired to enabled targets.
    // We don't need to modify desired, but we still need an apply. We mark hasAnyFix so caller can decide.
    let hasMissing = false;
    for (const id of Object.keys(desired || {})) {
      if (Object.prototype.hasOwnProperty.call(actualByID, id)) continue;
      hasMissing = true;
      break;
    }

    const hasNew = stableHash(nextDesired) !== stableHash(desired || {});
    const hasAnyFix = hasNew || hasMissing;
    return { conflictIDs, nextDesired, defaultChoice, hasAnyFix };
  }

  async function refresh() {
    setErr('');
    setNotice('');
    setLoading(true);
    try {
      const res = await getAdminMcp();
      if (!res.success) throw new Error(res.message || '加载失败');
      const d = res.data;
      setApplyResults(d?.apply_results || []);

      const targets = d?.targets || {};
      setTargetEnabled({
        codex: typeof targets.codex?.enabled === 'boolean' ? targets.codex.enabled : true,
        claude: typeof targets.claude?.enabled === 'boolean' ? targets.claude.enabled : true,
        gemini: typeof targets.gemini?.enabled === 'boolean' ? targets.gemini.enabled : true,
      });
      setTargetInfo({
        codex: { path: targets.codex?.path || '', exists: !!targets.codex?.exists },
        claude: { path: targets.claude?.path || '', exists: !!targets.claude?.exists },
        gemini: { path: targets.gemini?.path || '', exists: !!targets.gemini?.exists },
      });

      const servers = ((d?.store?.servers || {}) as Record<string, McpServerV2>) || {};
      setDesiredServers(servers);
    } catch (e) {
      setErr(e instanceof Error ? e.message : '加载失败');
    } finally {
      setLoading(false);
    }
  }

  async function scanNow(silent?: boolean) {
    if (!silent) {
      setErr('');
      setNotice('');
    }
    setScanning(true);
    try {
      const res = await scanAdminMcp();
      if (!res.success) throw new Error(res.message || '刷新失败');
      const t = (res.data?.targets || {}) as any;
      const nextInfo: Record<TargetKey, { path: string; exists: boolean; parse_error?: string; server_count?: number }> = {
        codex: { path: t.codex?.path || targetInfo.codex.path, exists: !!t.codex?.exists, parse_error: t.codex?.parse_error || '', server_count: t.codex?.server_count || 0 },
        claude: { path: t.claude?.path || targetInfo.claude.path, exists: !!t.claude?.exists, parse_error: t.claude?.parse_error || '', server_count: t.claude?.server_count || 0 },
        gemini: { path: t.gemini?.path || targetInfo.gemini.path, exists: !!t.gemini?.exists, parse_error: t.gemini?.parse_error || '', server_count: t.gemini?.server_count || 0 },
      };
      const parseOK = getParseOKTargets(nextInfo as any);
      const nextTargets: Record<TargetKey, { servers?: Record<string, McpServerV2> }> = {
        codex: { servers: parseOK.codex ? (t.codex?.servers || {}) : {} },
        claude: { servers: parseOK.claude ? (t.claude?.servers || {}) : {} },
        gemini: { servers: parseOK.gemini ? (t.gemini?.servers || {}) : {} },
      };

      setScannedTargets(nextTargets);
      setTargetInfo(nextInfo);

      // User-side "无感": auto-fix anything that doesn't require a decision.
      // Only show UI when there are real conflicts to resolve.
      if (!saving) {
        const desired = desiredServersRef.current || {};
        const { conflictIDs, nextDesired, defaultChoice, hasAnyFix } = computeConflictsAndAutoMerge(desired, nextTargets, nextInfo as any);
        if (conflictIDs.length > 0) {
          if (!conflictModalOpen.current) {
            conflictModalOpen.current = true;
            setConflicts(conflictIDs);
            setConflictChoice(defaultChoice);
            showModal('mcpConflictModal');
          }
        } else if (hasAnyFix) {
          const sig = stableHash({ desired, nextDesired, targets: nextTargets });
          if (sig && sig !== lastAutoFixSig.current) {
            lastAutoFixSig.current = sig;
            // Silent apply: keep UI quiet unless there's an error.
            void saveDesired(nextDesired, undefined, true);
          }
        }
      }

      if (!silent) setNotice('已刷新集成状态');
    } catch (e) {
      setErr(e instanceof Error ? e.message : '刷新失败');
    } finally {
      setScanning(false);
    }
  }

  async function saveDesired(next: Record<string, McpServerV2>, nextTargetEnabled?: Record<TargetKey, boolean>, silent?: boolean) {
    if (!silent) {
      setErr('');
      setNotice('');
    }
    setSaving(true);
    try {
      const res = await updateAdminMcp({
        store: { version: 2, servers: next || {} } satisfies McpStoreV2,
        target_enabled: nextTargetEnabled,
        apply_on_save: true,
      });
      if (!res.success) throw new Error(res.message || '保存失败');
      setApplyResults(res.data?.apply_results || []);
      setDesiredServers(((res.data?.store?.servers || next) as Record<string, McpServerV2>) || {});
      await scanNow(true);
      if (!silent) setNotice('已生效');
    } catch (e) {
      setErr(e instanceof Error ? e.message : '保存失败');
    } finally {
      setSaving(false);
    }
  }

  useEffect(() => {
    void refresh();
  }, []);

  useEffect(() => {
    if (loading) return;
    if (initialScanDone.current) return;
    initialScanDone.current = true;
    void scanNow(true);
  }, [loading]);

  function openCreate() {
    setEditing(null);
    setForm(initForm(null));
    showModal('mcpEditModal');
  }

  function openEditFromUnion(r: UnionRow) {
    const server = (r.desired || r.chosen) as McpServerV2 | undefined;
    if (!server) return;
    setEditing({ id: r.id, server });
    setForm(initForm({ id: r.id, server }));
    showModal('mcpEditModal');
  }

  function removeServer(id: string) {
    void (async () => {
      if (!window.confirm(`确认删除 MCP：${id}？`)) return;
      setErr('');
      setNotice('');
      setSaving(true);
      try {
        const targets: string[] = [];
        if (targetEnabled.codex) targets.push('codex');
        if (targetEnabled.claude) targets.push('claude');
        if (targetEnabled.gemini) targets.push('gemini');
        const res = await deleteAdminMcp({ id, targets });
        if (!res.success) throw new Error(res.message || '删除失败');
        setApplyResults(res.data?.apply_results || []);
        setDesiredServers((prev) => {
          const next = { ...(prev || {}) };
          delete next[id];
          return next;
        });
        await scanNow(true);
        setNotice('已删除并同步生效');
      } catch (e) {
        setErr(e instanceof Error ? e.message : '删除失败');
      } finally {
        setSaving(false);
      }
    })();
  }

  async function confirmConflicts() {
    const desiredMap = desiredServers as Record<string, McpServerV2>;
    const next: Record<string, McpServerV2> = { ...(desiredMap || {}) };

    for (const r of unionRows) {
      if (!r.hasActual) continue;
      if (!r.chosen) continue;
      if (!r.desired) {
        next[r.id] = r.chosen;
        continue;
      }
      if (!(r.actualConflict || !equalSpec(r.desired, r.chosen))) continue;

      const pick = conflictChoice[r.id] || 'desired';
      if (pick === 'desired') continue;
      const actual = r.actualByTarget[pick];
      if (actual) next[r.id] = actual;
    }

    await saveDesired(next);
    closeModalById('mcpConflictModal');
  }

  async function toggleTarget(k: TargetKey, v: boolean) {
    const next = { ...targetEnabled, [k]: v };
    setTargetEnabled(next);
    await saveDesired(desiredServersRef.current || {}, next);
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

      <SegmentedFrame>
        <div className="row g-3">
          {(['codex', 'claude', 'gemini'] as const).map((k) => {
            const info = targetInfo[k];
            const enabled = !!targetEnabled[k];
            const parseError = (info.parse_error || '').trim();
            const count = typeof info.server_count === 'number' ? info.server_count : null;
            const statusLabel = parseError ? '读取失败' : '正常';
            const statusBadge = parseError ? 'bg-light text-danger border' : 'bg-light text-success border';
            return (
              <div key={k} className="col-12 col-lg-4">
                <div className="card h-100">
                  <div className="card-body">
                    <div className="d-flex justify-content-between align-items-start">
                      <div>
                        <div className="fw-semibold text-capitalize">{k}</div>
                        <div className="text-muted small">当前生效的 MCP</div>
                      </div>
                      <div className="form-check form-switch">
                        <input
                          className="form-check-input"
                          type="checkbox"
                          role="switch"
                          checked={enabled}
                          disabled={loading || saving}
                          onChange={(e) => void toggleTarget(k, e.target.checked)}
                        />
                      </div>
                    </div>
                    <div className="border-top mt-3 pt-3">
                      <div className="d-flex gap-2">
                        <span className={`badge ${statusBadge}`}>{statusLabel}</span>
                        {count !== null ? <span className="badge bg-light text-primary border">{count} servers</span> : null}
                      </div>
                      {parseError ? <div className="text-danger small mt-2 text-truncate">{parseError}</div> : null}
                    </div>
                  </div>
                </div>
              </div>
            );
          })}
        </div>
      </SegmentedFrame>

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
                          : st === 'missing'
                            ? 'bg-light text-secondary border'
                            : 'bg-light text-danger border';

                    const cell = (k: TargetKey) => {
                      const sv = r.actualByTarget[k];
                      if (!sv) return <span className="text-muted">—</span>;
                      if (!chosen) return <span className="badge bg-light text-primary border">√</span>;
                      return equalSpec(sv, chosen) ? (
                        <span className="badge bg-light text-success border">√</span>
                      ) : (
                        <span className="badge bg-light text-danger border">!</span>
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
            <button type="button" className="btn btn-primary px-4" disabled={saving} onClick={() => void confirmConflicts()}>
              确认
            </button>
          </>
        }
        onHidden={() => {
          setConflicts([]);
          setConflictChoice({});
          conflictModalOpen.current = false;
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
            <button type="button" className="btn btn-primary px-4" disabled={saving} onClick={() => saveFormToDesired()}>
              保存并生效
            </button>
          </>
        }
        onHidden={() => {
          setEditing(null);
          setForm(initForm(null));
        }}
      >
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
      </BootstrapModal>
    </div>
  );
}
