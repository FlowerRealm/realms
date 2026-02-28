import { useCallback, useEffect, useMemo, useRef, useState } from 'react';

import {
  applyAdminSkills,
  autoAdoptAdminSkills,
  deleteAdminSkills,
  getAdminSkills,
  importAdminSkills,
  scanAdminSkills,
  updateAdminSkills,
  type ApplyAdminSkillsRequest,
  type AdminSkillsState,
  type SkillApplyConflict,
  type SkillApplyResult,
  type SkillV1,
  type SkillsStoreV1,
  type SkillsTargetEnabledV1,
  type SkillsTargetKey,
} from '../../api/admin/skills';

type ScanTarget = NonNullable<Awaited<ReturnType<typeof scanAdminSkills>>['data']>['targets'][string];
type ScanTargets = Partial<Record<SkillsTargetKey, ScanTarget>>;

function emptyStore(): SkillsStoreV1 {
  return { version: 1, skills: {} };
}

function normalizeStore(store: SkillsStoreV1 | undefined): SkillsStoreV1 {
  const skills = (store?.skills || {}) as Record<string, SkillV1>;
  return { version: store?.version || 1, skills };
}

export type UseSkillsManagerResult = {
  loading: boolean;
  scanning: boolean;
  saving: boolean;

  err: string;
  notice: string;
  setErr: (v: string) => void;
  setNotice: (v: string) => void;

  store: SkillsStoreV1;
  targetEnabled: SkillsTargetEnabledV1;
  targets: AdminSkillsState['targets'];
  desiredHashes: Record<string, Partial<Record<SkillsTargetKey, string>>>;

  scanTargets: ScanTargets | null;
  applyResults: SkillApplyResult[];
  conflicts: SkillApplyConflict[];

  refresh: () => Promise<void>;
  scanNow: (silent?: boolean) => Promise<void>;
  saveStore: (next: SkillsStoreV1, applyOnSave?: boolean) => Promise<void>;
  applyNow: (opts?: Pick<ApplyAdminSkillsRequest, 'targets' | 'remove_ids' | 'force' | 'resolutions'>) => Promise<void>;
  importFrom: (source: SkillsTargetKey, mode: 'merge' | 'replace', applyAfter: boolean) => Promise<void>;
  removeSkill: (id: string) => Promise<void>;
};

export function useSkillsManager(): UseSkillsManagerResult {
  const [loading, setLoading] = useState(true);
  const [scanning, setScanning] = useState(false);
  const [saving, setSaving] = useState(false);

  const [err, setErr] = useState('');
  const [notice, setNotice] = useState('');

  const [store, setStore] = useState<SkillsStoreV1>(() => emptyStore());
  const [targetEnabled, setTargetEnabled] = useState<SkillsTargetEnabledV1>({});
  const [targets, setTargets] = useState<AdminSkillsState['targets']>({});
  const [desiredHashes, setDesiredHashes] = useState<Record<string, Partial<Record<SkillsTargetKey, string>>>>({});

  const [scanTargets, setScanTargets] = useState<ScanTargets | null>(null);
  const [applyResults, setApplyResults] = useState<SkillApplyResult[]>([]);
  const [conflicts, setConflicts] = useState<SkillApplyConflict[]>([]);

  const initialScanDone = useRef(false);
  const autoAdoptDone = useRef(false);

  const refresh = useCallback(async () => {
    setErr('');
    setNotice('');
    setLoading(true);
    try {
      const res = await getAdminSkills();
      if (!res.success) throw new Error(res.message || '加载失败');
      const s = normalizeStore(res.data?.store as SkillsStoreV1 | undefined);
      setStore(s);
      setTargetEnabled((res.data?.target_enabled || {}) as SkillsTargetEnabledV1);
      setTargets(res.data?.targets || {});
      setDesiredHashes((res.data?.desired_hashes || {}) as Record<string, Partial<Record<SkillsTargetKey, string>>>);
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
      const res = await scanAdminSkills();
      if (!res.success) throw new Error(res.message || '扫描失败');
      setScanTargets((res.data?.targets || null) as ScanTargets | null);
      if (!silent) setNotice('已刷新实际状态');
    } catch (e) {
      setErr(e instanceof Error ? e.message : '扫描失败');
    } finally {
      setScanning(false);
    }
  }, []);

  const saveStore = useCallback(async (next: SkillsStoreV1, applyOnSave?: boolean) => {
    setErr('');
    setNotice('');
    setSaving(true);
    try {
      const res = await updateAdminSkills({ store: next, apply_on_save: !!applyOnSave });
      if (!res.success) throw new Error(res.message || '保存失败');
      const s = normalizeStore(res.data?.store as SkillsStoreV1 | undefined) || next;
      setStore(s);
      setApplyResults(res.data?.apply_results || []);
      setConflicts(res.data?.conflicts || []);
      await refresh();
      setNotice(applyOnSave ? '已保存并尝试应用' : '已保存');
    } catch (e) {
      setErr(e instanceof Error ? e.message : '保存失败');
    } finally {
      setSaving(false);
    }
  }, [refresh]);

  const applyNow = useCallback(async (opts?: Pick<ApplyAdminSkillsRequest, 'targets' | 'remove_ids' | 'force' | 'resolutions'>) => {
    setErr('');
    setNotice('');
    setSaving(true);
    try {
      const res = await applyAdminSkills({
        targets: opts?.targets,
        remove_ids: opts?.remove_ids,
        force: opts?.force,
        resolutions: opts?.resolutions,
      });
      if (!res.success) throw new Error(res.message || '应用失败');
      setApplyResults(res.data?.apply_results || []);
      setConflicts(res.data?.conflicts || []);
      if (res.data?.store) setStore(normalizeStore(res.data.store as SkillsStoreV1));
      await scanNow(true);
      setNotice((res.data?.conflicts || []).length ? '检测到冲突：需要处理' : '已应用');
    } catch (e) {
      setErr(e instanceof Error ? e.message : '应用失败');
    } finally {
      setSaving(false);
    }
  }, [scanNow]);

  const importFrom = useCallback(async (source: SkillsTargetKey, mode: 'merge' | 'replace', applyAfter: boolean) => {
    setErr('');
    setNotice('');
    setSaving(true);
    try {
      const res = await importAdminSkills({ source, mode, apply_after: applyAfter });
      if (!res.success) throw new Error(res.message || '导入失败');
      if (res.data?.store) setStore(normalizeStore(res.data.store as SkillsStoreV1));
      setApplyResults(res.data?.apply_results || []);
      setConflicts(res.data?.conflicts || []);
      await scanNow(true);
      setNotice('已导入');
    } catch (e) {
      setErr(e instanceof Error ? e.message : '导入失败');
    } finally {
      setSaving(false);
    }
  }, [scanNow]);

  const removeSkill = useCallback(async (id: string) => {
    const sid = (id || '').trim();
    if (!sid) return;
    setErr('');
    setNotice('');
    setSaving(true);
    try {
      const res = await deleteAdminSkills({ id: sid });
      if (!res.success) throw new Error(res.message || '删除失败');
      setApplyResults(res.data?.apply_results || []);
      setConflicts(res.data?.conflicts || []);
      setStore((prev) => {
        const next = normalizeStore(prev);
        const skillsMap = { ...(next.skills || {}) };
        delete skillsMap[sid];
        return { ...next, skills: skillsMap };
      });
      await scanNow(true);
      setNotice('已删除');
    } catch (e) {
      setErr(e instanceof Error ? e.message : '删除失败');
    } finally {
      setSaving(false);
    }
  }, [scanNow]);

  useEffect(() => {
    void refresh();
  }, [refresh]);

  useEffect(() => {
    if (loading) return;
    if (initialScanDone.current) return;
    initialScanDone.current = true;
    void scanNow(true);
  }, [loading, scanNow]);

  useEffect(() => {
    if (loading) return;
    if (autoAdoptDone.current) return;
    if (!scanTargets) return;

    const desired = new Set(Object.keys(store?.skills || {}));
    const scanned = new Set<string>();
    for (const t of ['codex', 'claude', 'gemini'] as const) {
      const m = scanTargets?.[t]?.skills || {};
      for (const id of Object.keys(m)) scanned.add(id);
    }
    let unmanaged = 0;
    for (const id of scanned) {
      if (!desired.has(id)) unmanaged += 1;
    }
    if (unmanaged <= 0) {
      autoAdoptDone.current = true;
      return;
    }

    autoAdoptDone.current = true;
    void (async () => {
      setSaving(true);
      try {
        const res = await autoAdoptAdminSkills();
        if (!res.success) throw new Error(res.message || '自动纳管失败');
        const n = res.data?.adopted_count || 0;
        await refresh();
        await scanNow(true);
        if (n > 0) setNotice(`已自动纳管 ${n} 个 skills`);
      } catch (e) {
        setErr(e instanceof Error ? e.message : '自动纳管失败');
      } finally {
        setSaving(false);
      }
    })();
  }, [loading, scanTargets, store, refresh, scanNow]);

  const memo = useMemo(
    () => ({
      loading,
      scanning,
      saving,
      err,
      notice,
      setErr,
      setNotice,
      store,
      targetEnabled,
      targets,
      desiredHashes,
      scanTargets,
      applyResults,
      conflicts,
      refresh,
      scanNow,
      saveStore,
      applyNow,
      importFrom,
      removeSkill,
    }),
    [loading, scanning, saving, err, notice, store, targetEnabled, targets, desiredHashes, scanTargets, applyResults, conflicts, refresh, scanNow, saveStore, applyNow, importFrom, removeSkill],
  );

  return memo;
}
