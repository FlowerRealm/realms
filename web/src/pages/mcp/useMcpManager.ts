import type { Dispatch, SetStateAction } from 'react';
import { useCallback, useEffect, useMemo, useRef, useState } from 'react';

import { deleteAdminMcp, getAdminMcp, scanAdminMcp, updateAdminMcp, type AdminMcpApplyResult, type McpServerV2, type McpStoreV2 } from '../../api/admin/mcp';

import type { PerTarget, ScannedTargets, TargetInfo, TargetKey, UnionRow } from './mcpTypes';
import { chooseActualServer, chooseActualServerForTargets, equalServerCore, stableHash, targetEnabledForServer } from './mcpUtils';

type ConflictPick = 'codex' | 'claude' | 'gemini' | 'desired';

type SaveDesiredFn = (next: Record<string, McpServerV2>, silent?: boolean) => Promise<void>;

function getParseOKTargets(info: Record<TargetKey, { parse_error?: string }>): Record<TargetKey, boolean> {
  return {
    codex: !(info.codex?.parse_error || '').trim(),
    claude: !(info.claude?.parse_error || '').trim(),
    gemini: !(info.gemini?.parse_error || '').trim(),
  };
}

function buildActualByID(targets: Record<TargetKey, { servers?: Record<string, McpServerV2> }>, parseOK: Record<TargetKey, boolean>): Record<string, PerTarget> {
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
): { conflictIDs: string[]; nextDesired: Record<string, McpServerV2>; defaultChoice: Record<string, ConflictPick>; hasAnyFix: boolean } {
  const parseOK = getParseOKTargets(info);
  const actualByID = buildActualByID(targets, parseOK);

  const nextDesired: Record<string, McpServerV2> = { ...(desired || {}) };
  const conflictIDs: string[] = [];
  const defaultChoice: Record<string, ConflictPick> = {};

  for (const [id, per] of Object.entries(actualByID)) {
    if (Object.prototype.hasOwnProperty.call(nextDesired, id)) continue;
    const chosen = chooseActualServer(per);
    if (!chosen) continue;
    const present: Record<TargetKey, boolean> = {
      codex: !!per.codex,
      claude: !!per.claude,
      gemini: !!per.gemini,
    };
    const disable: Partial<Record<TargetKey, boolean>> = {};
    for (const t of ['codex', 'claude', 'gemini'] as const) {
      if (!present[t]) disable[t] = false;
    }
    nextDesired[id] = Object.keys(disable).length ? { ...chosen, targets: disable } : chosen;
  }

  for (const [id, per] of Object.entries(actualByID)) {
    const desiredSpec = nextDesired[id];
    const enabled: Record<TargetKey, boolean> = {
      codex: desiredSpec ? targetEnabledForServer(desiredSpec, 'codex') : true,
      claude: desiredSpec ? targetEnabledForServer(desiredSpec, 'claude') : true,
      gemini: desiredSpec ? targetEnabledForServer(desiredSpec, 'gemini') : true,
    };
    if (!enabled.codex && !enabled.claude && !enabled.gemini) continue;

    const actualSpecs: McpServerV2[] = [];
    for (const t of ['codex', 'claude', 'gemini'] as const) {
      if (!enabled[t]) continue;
      const sv = per[t];
      if (sv) actualSpecs.push(sv);
    }
    let conflict = false;
    if (actualSpecs.length > 1) {
      const first = actualSpecs[0];
      for (const s of actualSpecs.slice(1)) {
        if (!equalServerCore(first, s)) {
          conflict = true;
          break;
        }
      }
    }
    const chosen = chooseActualServerForTargets(per, enabled);
    if (!conflict && chosen && desiredSpec && !equalServerCore(desiredSpec, chosen)) conflict = true;
    if (!conflict) continue;
    conflictIDs.push(id);
    if (enabled.codex && per.codex) defaultChoice[id] = 'codex';
    else if (enabled.claude && per.claude) defaultChoice[id] = 'claude';
    else if (enabled.gemini && per.gemini) defaultChoice[id] = 'gemini';
    else if (per.codex) defaultChoice[id] = 'codex';
    else if (per.claude) defaultChoice[id] = 'claude';
    else if (per.gemini) defaultChoice[id] = 'gemini';
    else defaultChoice[id] = 'desired';
  }

  let hasMissing = false;
  for (const [id, ds] of Object.entries(desired || {})) {
    const enabled: Record<TargetKey, boolean> = {
      codex: targetEnabledForServer(ds, 'codex'),
      claude: targetEnabledForServer(ds, 'claude'),
      gemini: targetEnabledForServer(ds, 'gemini'),
    };
    if (!enabled.codex && !enabled.claude && !enabled.gemini) continue;
    const per = actualByID[id] || {};
    for (const t of ['codex', 'claude', 'gemini'] as const) {
      if (!enabled[t]) continue;
      if (!per[t]) {
        hasMissing = true;
        break;
      }
    }
    if (hasMissing) break;
  }

  const hasNew = stableHash(nextDesired) !== stableHash(desired || {});
  const hasAnyFix = hasNew || hasMissing;
  return { conflictIDs, nextDesired, defaultChoice, hasAnyFix };
}

export type UseMcpManagerResult = {
  loading: boolean;
  scanning: boolean;
  saving: boolean;
  applyResults: AdminMcpApplyResult[];

  err: string;
  notice: string;
  setErr: (v: string) => void;
  setNotice: (v: string) => void;

  targetInfo: TargetInfo;
  scannedTargets: ScannedTargets;
  desiredServers: Record<string, McpServerV2>;

  unionRows: UnionRow[];
  diffSummary: { nNew: number; nMissing: number; nConflict: number };

  conflicts: string[];
  conflictChoice: Record<string, ConflictPick>;
  setConflictChoice: Dispatch<SetStateAction<Record<string, ConflictPick>>>;
  conflictModalRequested: boolean;
  ackConflictModalRequested: () => void;
  onConflictModalHidden: () => void;

  refresh: () => Promise<void>;
  scanNow: (silent?: boolean) => Promise<void>;
  saveDesired: (next: Record<string, McpServerV2>, silent?: boolean) => Promise<void>;
  removeServer: (id: string) => void;
  confirmConflicts: () => Promise<void>;
  getDesiredServersSnapshot: () => Record<string, McpServerV2>;
};

export function useMcpManager(): UseMcpManagerResult {
  const [loading, setLoading] = useState(true);
  const [scanning, setScanning] = useState(false);
  const [saving, setSaving] = useState(false);
  const [applyResults, setApplyResults] = useState<AdminMcpApplyResult[]>([]);

  const initialScanDone = useRef(false);
  const lastAutoFixSig = useRef<string>('');
  const conflictModalOpen = useRef(false);
  const desiredServersRef = useRef<Record<string, McpServerV2>>({});
  const saveDesiredRef = useRef<SaveDesiredFn>(async () => {});

  const [err, setErr] = useState('');
  const [notice, setNotice] = useState('');

  const [targetInfo, setTargetInfo] = useState<TargetInfo>({
    codex: { path: '', exists: false },
    claude: { path: '', exists: false },
    gemini: { path: '', exists: false },
  });
  const [scannedTargets, setScannedTargets] = useState<ScannedTargets>({
    codex: {},
    claude: {},
    gemini: {},
  });

  const [desiredServers, setDesiredServers] = useState<Record<string, McpServerV2>>({});

  const [conflicts, setConflicts] = useState<string[]>([]);
  const [conflictChoice, setConflictChoice] = useState<Record<string, ConflictPick>>({});
  const [conflictModalRequested, setConflictModalRequested] = useState(false);

  useEffect(() => {
    desiredServersRef.current = desiredServers || {};
  }, [desiredServers]);

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
      const hasActual = Object.keys(actualByTarget).length > 0;

      const desiredEnabled: Record<TargetKey, boolean> = {
        codex: desired ? targetEnabledForServer(desired, 'codex') : true,
        claude: desired ? targetEnabledForServer(desired, 'claude') : true,
        gemini: desired ? targetEnabledForServer(desired, 'gemini') : true,
      };
      const enabledTargets: Record<TargetKey, boolean> = desired
        ? desiredEnabled
        : {
            codex: !!actualByTarget.codex,
            claude: !!actualByTarget.claude,
            gemini: !!actualByTarget.gemini,
          };

      const chosen = chooseActualServerForTargets(actualByTarget, enabledTargets);

      const enabledActualSpecs: McpServerV2[] = [];
      for (const t of ['codex', 'claude', 'gemini'] as const) {
        if (!enabledTargets[t]) continue;
        const sv = actualByTarget[t];
        if (sv) enabledActualSpecs.push(sv);
      }
      const actualConflict = (() => {
        if (enabledActualSpecs.length <= 1) return false;
        const first = enabledActualSpecs[0];
        for (const s of enabledActualSpecs.slice(1)) {
          if (!equalServerCore(first, s)) return true;
        }
        return false;
      })();

      let status: UnionRow['status'] = 'synced';
      if (!desired && hasActual) {
        status = 'new';
      } else if (!desired && !hasActual) {
        continue;
      } else if (desired && !enabledTargets.codex && !enabledTargets.claude && !enabledTargets.gemini) {
        status = 'disabled';
      } else if (desired) {
        let missing = false;
        for (const t of ['codex', 'claude', 'gemini'] as const) {
          if (!enabledTargets[t]) continue;
          if (!actualByTarget[t]) {
            missing = true;
            break;
          }
        }
        if (missing) {
          status = 'missing';
        } else if (actualConflict) {
          status = 'conflict';
        } else if (chosen && !equalServerCore(desired, chosen)) {
          status = 'conflict';
        } else {
          status = 'synced';
        }
      }

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

  const refresh = useCallback(async () => {
    setErr('');
    setNotice('');
    setLoading(true);
    try {
      const res = await getAdminMcp();
      if (!res.success) throw new Error(res.message || '加载失败');
      const d = res.data;
      setApplyResults(d?.apply_results || []);

      const targets = d?.targets || {};
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
  }, []);

  const scanNow = useCallback(async (silent?: boolean) => {
    if (!silent) {
      setErr('');
      setNotice('');
    }
    setScanning(true);
    try {
      const res = await scanAdminMcp();
      if (!res.success) throw new Error(res.message || '刷新失败');
      type ScanTarget = { path: string; exists: boolean; parse_error?: string; server_count: number; servers?: Record<string, McpServerV2> };
      const t = (res.data?.targets || {}) as Partial<Record<TargetKey, ScanTarget>>;
      const nextInfo: TargetInfo = {
        codex: { path: t.codex?.path || targetInfo.codex.path, exists: !!t.codex?.exists, parse_error: t.codex?.parse_error || '', server_count: t.codex?.server_count || 0 },
        claude: { path: t.claude?.path || targetInfo.claude.path, exists: !!t.claude?.exists, parse_error: t.claude?.parse_error || '', server_count: t.claude?.server_count || 0 },
        gemini: { path: t.gemini?.path || targetInfo.gemini.path, exists: !!t.gemini?.exists, parse_error: t.gemini?.parse_error || '', server_count: t.gemini?.server_count || 0 },
      };
      const parseOK = getParseOKTargets(nextInfo);
      const nextTargets: ScannedTargets = {
        codex: { servers: parseOK.codex ? (t.codex?.servers || {}) : {} },
        claude: { servers: parseOK.claude ? (t.claude?.servers || {}) : {} },
        gemini: { servers: parseOK.gemini ? (t.gemini?.servers || {}) : {} },
      };

      setScannedTargets(nextTargets);
      setTargetInfo(nextInfo);

      if (!saving) {
        const desired = desiredServersRef.current || {};
        const { conflictIDs, nextDesired, defaultChoice, hasAnyFix } = computeConflictsAndAutoMerge(desired, nextTargets, nextInfo);
        if (conflictIDs.length > 0) {
          if (!conflictModalOpen.current) {
            conflictModalOpen.current = true;
            setConflicts(conflictIDs);
            setConflictChoice(defaultChoice);
            setConflictModalRequested(true);
          }
        } else if (hasAnyFix) {
          const sig = stableHash({ desired, nextDesired, targets: nextTargets });
          if (sig && sig !== lastAutoFixSig.current) {
            lastAutoFixSig.current = sig;
            void saveDesiredRef.current(nextDesired, true);
          }
        }
      }

      if (!silent) setNotice('已刷新集成状态');
    } catch (e) {
      setErr(e instanceof Error ? e.message : '刷新失败');
    } finally {
      setScanning(false);
    }
  }, [saving, targetInfo]);

  const saveDesired = useCallback(async (next: Record<string, McpServerV2>, silent?: boolean) => {
    if (!silent) {
      setErr('');
      setNotice('');
    }
    setSaving(true);
    try {
      const res = await updateAdminMcp({
        store: { version: 2, servers: next || {} } satisfies McpStoreV2,
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
  }, [scanNow]);

  useEffect(() => {
    saveDesiredRef.current = saveDesired;
  }, [saveDesired]);

  function removeServer(id: string) {
    void (async () => {
      if (!window.confirm(`确认删除 MCP：${id}？`)) return;
      setErr('');
      setNotice('');
      setSaving(true);
      try {
        const res = await deleteAdminMcp({ id });
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
        const present: Record<TargetKey, boolean> = {
          codex: !!r.actualByTarget.codex,
          claude: !!r.actualByTarget.claude,
          gemini: !!r.actualByTarget.gemini,
        };
        const disable: Partial<Record<TargetKey, boolean>> = {};
        for (const t of ['codex', 'claude', 'gemini'] as const) {
          if (!present[t]) disable[t] = false;
        }
        next[r.id] = Object.keys(disable).length ? { ...r.chosen, targets: disable } : r.chosen;
        continue;
      }
      if (!(r.actualConflict || !equalServerCore(r.desired, r.chosen))) continue;

      const pick = conflictChoice[r.id] || 'desired';
      if (pick === 'desired') continue;
      const actual = r.actualByTarget[pick];
      if (actual) {
        const targets = r.desired.targets;
        next[r.id] = targets ? { ...actual, targets } : actual;
      }
    }

    await saveDesired(next);
  }

  function ackConflictModalRequested() {
    setConflictModalRequested(false);
  }

  function onConflictModalHidden() {
    setConflicts([]);
    setConflictChoice({});
    conflictModalOpen.current = false;
  }

  function getDesiredServersSnapshot(): Record<string, McpServerV2> {
    return desiredServersRef.current || {};
  }

  useEffect(() => {
    void refresh();
  }, [refresh]);

  useEffect(() => {
    if (loading) return;
    if (initialScanDone.current) return;
    initialScanDone.current = true;
    void scanNow(true);
  }, [loading, scanNow]);

  return {
    loading,
    scanning,
    saving,
    applyResults,

    err,
    notice,
    setErr,
    setNotice,

    targetInfo,
    scannedTargets,
    desiredServers,

    unionRows,
    diffSummary,

    conflicts,
    conflictChoice,
    setConflictChoice,
    conflictModalRequested,
    ackConflictModalRequested,
    onConflictModalHidden,

    refresh,
    scanNow,
    saveDesired,
    removeServer,
    confirmConflicts,
    getDesiredServersSnapshot,
  };
}
