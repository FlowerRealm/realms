import type { McpServerV2 } from '../../../api/admin/mcp';
import { BootstrapModal } from '../../../components/BootstrapModal';

import type { TargetKey, UnionRow } from '../mcpTypes';
import { mainSummary } from '../mcpUtils';

type ConflictPick = TargetKey | 'desired';

export function McpConflictModal(props: {
  saving: boolean;
  conflicts: string[];
  unionRows: UnionRow[];
  conflictChoice: Record<string, ConflictPick>;
  setConflictChoice: (fn: (prev: Record<string, ConflictPick>) => Record<string, ConflictPick>) => void;
  onConfirm: () => void;
  onHidden: () => void;
}) {
  const { saving, conflicts, unionRows, conflictChoice, setConflictChoice, onConfirm, onHidden } = props;

  return (
    <BootstrapModal
      id="mcpConflictModal"
      title="检测到冲突：请选择要保留的版本"
      dialogClassName="modal-lg modal-dialog-scrollable"
      footer={
        <>
          <button type="button" className="btn btn-light" data-bs-dismiss="modal">
            取消
          </button>
          <button type="button" className="btn btn-primary px-4" disabled={saving} onClick={onConfirm}>
            确认
          </button>
        </>
      }
      onHidden={onHidden}
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
                <span className="badge bg-light text-danger border">冲突</span>
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
                      <div className="text-muted small font-monospace text-truncate">{mainSummary(desired as McpServerV2 | undefined)}</div>
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
  );
}
