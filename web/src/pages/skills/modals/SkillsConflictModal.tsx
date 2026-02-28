import { BootstrapModal } from '../../../components/BootstrapModal';
import type { SkillApplyConflict } from '../../../api/admin/skills';

export type ConflictPick = { action: 'keep' | 'overwrite' | 'rename'; name?: string };

export function SkillsConflictModal(props: {
  saving: boolean;
  conflicts: SkillApplyConflict[];
  picks: Record<string, ConflictPick>;
  setPick: (key: string, pick: ConflictPick) => void;
  onConfirm: () => void;
  onHidden: () => void;
}) {
  const { saving, conflicts, picks, setPick, onConfirm, onHidden } = props;

  return (
    <BootstrapModal
      id="skillsConflictModal"
      title="检测到冲突：请选择处理方式"
      dialogClassName="modal-lg modal-dialog-scrollable"
      footer={
        <>
          <button type="button" className="btn btn-light" data-bs-dismiss="modal">
            取消
          </button>
          <button type="button" className="btn btn-primary px-4" disabled={saving} onClick={onConfirm}>
            确认并重试
          </button>
        </>
      }
      onHidden={onHidden}
    >
      <div className="text-muted small mb-2">默认选择“保留目标文件（keep）”。重命名会写入新名字并更新 Realms 记录。</div>
      <div className="d-flex flex-column gap-3">
        {conflicts.map((c) => {
          const k = `${c.target}:${c.id}`;
          const pick = picks[k] || { action: 'keep' as const };
          return (
            <div key={k} className="border rounded-3 p-3">
              <div className="d-flex justify-content-between align-items-center">
                <div className="fw-semibold">
                  <span className="font-monospace">{c.id}</span> <span className="text-muted">→</span> <span className="text-capitalize">{c.target}</span>
                </div>
                <span className="badge bg-light text-danger border">冲突</span>
              </div>
              <div className="text-muted small mt-1 font-monospace text-truncate">{c.path}</div>
              {c.reason ? <div className="text-muted small mt-1">{c.reason}</div> : null}
              <div className="row g-2 mt-2">
                <div className="col-12 col-md-4">
                  <label className="form-check d-flex gap-2 align-items-start">
                    <input className="form-check-input mt-1" type="radio" checked={pick.action === 'keep'} onChange={() => setPick(k, { action: 'keep' })} />
                    <div>
                      <div className="fw-medium">Keep</div>
                      <div className="text-muted small">不覆盖目标文件</div>
                    </div>
                  </label>
                </div>
                <div className="col-12 col-md-4">
                  <label className="form-check d-flex gap-2 align-items-start">
                    <input className="form-check-input mt-1" type="radio" checked={pick.action === 'overwrite'} onChange={() => setPick(k, { action: 'overwrite' })} />
                    <div>
                      <div className="fw-medium">Overwrite</div>
                      <div className="text-muted small">用 Realms 覆盖</div>
                    </div>
                  </label>
                </div>
                <div className="col-12 col-md-4">
                  <label className="form-check d-flex gap-2 align-items-start">
                    <input
                      className="form-check-input mt-1"
                      type="radio"
                      checked={pick.action === 'rename'}
                      onChange={() => setPick(k, { action: 'rename', name: pick.name || '' })}
                    />
                    <div className="flex-grow-1">
                      <div className="fw-medium">Rename</div>
                      <div className="text-muted small">写到新名字</div>
                      {pick.action === 'rename' ? (
                        <input
                          className="form-control form-control-sm mt-2 font-monospace"
                          placeholder="new-name"
                          value={pick.name || ''}
                          onChange={(e) => setPick(k, { action: 'rename', name: e.target.value })}
                        />
                      ) : null}
                    </div>
                  </label>
                </div>
              </div>
            </div>
          );
        })}
      </div>
    </BootstrapModal>
  );
}

