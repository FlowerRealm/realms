import type { SkillV1, SkillApplyConflict, SkillsTargetKey } from '../../api/admin/skills';

export type ScannedSkill = { name: string; path: string; sha256: string };

export type SkillsUnionRow = {
  id: string;
  desired?: SkillV1;
  actualByTarget: Partial<Record<SkillsTargetKey, ScannedSkill>>;
  enabledByTarget: Record<SkillsTargetKey, boolean>;
  status: 'synced' | 'new' | 'missing' | 'disabled' | 'conflict' | 'unmanaged';
};

export function skillEnabled(desired: SkillV1 | undefined, k: SkillsTargetKey): boolean {
  if (!desired) return false;
  const v = desired.per_target?.[k]?.enabled;
  if (v === undefined) return true;
  return !!v;
}

export function skillPromptSummary(desired: SkillV1 | undefined): string {
  const p = (desired?.prompt || '').trim();
  if (!p) return '';
  const first = p.split('\n')[0] || '';
  return first.length > 120 ? first.slice(0, 120) + '…' : first;
}

export function buildUnionRows(opts: {
  desiredSkills: Record<string, SkillV1>;
  scanTargets?: Partial<Record<SkillsTargetKey, { skills?: Record<string, ScannedSkill> }>>;
  desiredHashes?: Record<string, Partial<Record<SkillsTargetKey, string>>>;
  includeUnmanaged?: boolean;
}): { unionRows: SkillsUnionRow[]; diffSummary: { nConflict: number; nMissing: number; nNew: number; nUnmanaged: number } } {
  const desiredSkills = opts.desiredSkills || {};
  const scanTargets = opts.scanTargets || {};
  const desiredHashes = opts.desiredHashes || {};
  const includeUnmanaged = opts.includeUnmanaged !== false;

  const ids = new Set<string>();
  for (const id of Object.keys(desiredSkills)) ids.add(id);
  if (includeUnmanaged) {
    for (const t of ['codex', 'claude', 'gemini'] as const) {
      const m = (scanTargets?.[t]?.skills || {}) as Record<string, ScannedSkill>;
      for (const id of Object.keys(m)) ids.add(id);
    }
  }

  const list = Array.from(ids).sort((a, b) => a.localeCompare(b));
  const unionRows: SkillsUnionRow[] = [];

  let nConflict = 0;
  let nMissing = 0;
  let nNew = 0;
  let nUnmanaged = 0;

  for (const id of list) {
    const desired = desiredSkills[id];
    const actualByTarget: Partial<Record<SkillsTargetKey, ScannedSkill>> = {
      codex: (scanTargets?.codex?.skills || {})[id],
      claude: (scanTargets?.claude?.skills || {})[id],
      gemini: (scanTargets?.gemini?.skills || {})[id],
    };

    const enabledByTarget: Record<SkillsTargetKey, boolean> = {
      codex: skillEnabled(desired, 'codex'),
      claude: skillEnabled(desired, 'claude'),
      gemini: skillEnabled(desired, 'gemini'),
    };

    let status: SkillsUnionRow['status'] = 'synced';
    if (!desired) {
      status = (actualByTarget.codex || actualByTarget.claude || actualByTarget.gemini) ? 'unmanaged' : 'unmanaged';
      nUnmanaged += 1;
    } else if (!enabledByTarget.codex && !enabledByTarget.claude && !enabledByTarget.gemini) {
      status = 'disabled';
    } else {
      let enabledTargets = 0;
      let missing = 0;
      let conflict = 0;
      for (const t of ['codex', 'claude', 'gemini'] as const) {
        if (!enabledByTarget[t]) continue;
        enabledTargets += 1;
        const actual = actualByTarget[t];
        if (!actual) {
          missing += 1;
          continue;
        }
        const dh = desiredHashes?.[id]?.[t];
        if (dh && actual.sha256 && dh !== actual.sha256) {
          conflict += 1;
        }
      }
      if (conflict > 0) {
        status = 'conflict';
        nConflict += 1;
      } else if (missing === enabledTargets && enabledTargets > 0) {
        status = 'new';
        nNew += 1;
      } else if (missing > 0) {
        status = 'missing';
        nMissing += 1;
      } else {
        status = 'synced';
      }
    }

    unionRows.push({ id, desired, actualByTarget, enabledByTarget, status });
  }

  return { unionRows, diffSummary: { nConflict, nMissing, nNew, nUnmanaged } };
}

export function conflictsFromUnion(unionRows: SkillsUnionRow[], desiredHashes?: Record<string, Partial<Record<SkillsTargetKey, string>>>): SkillApplyConflict[] {
  const out: SkillApplyConflict[] = [];
  const dh = desiredHashes || {};
  for (const r of unionRows) {
    if (!r.desired) continue;
    for (const t of ['codex', 'claude', 'gemini'] as const) {
      if (!r.enabledByTarget[t]) continue;
      const actual = r.actualByTarget[t];
      if (!actual) continue;
      const want = dh?.[r.id]?.[t];
      if (want && actual.sha256 && want !== actual.sha256) {
        out.push({ id: r.id, target: t, path: actual.path, existing_sha256: actual.sha256, desired_sha256: want, reason: 'content differs' });
      }
    }
  }
  return out;
}
