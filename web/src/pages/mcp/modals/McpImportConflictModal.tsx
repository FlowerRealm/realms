import type { McpServerV2 } from '../../../api/admin/mcp';
import { BootstrapModal } from '../../../components/BootstrapModal';

import type { ImportPick } from '../mcpTypes';
import { mainSummary } from '../mcpUtils';

export function McpImportConflictModal(props: {
  saving: boolean;
  importPending: null | { desired: Record<string, McpServerV2>; imported: Record<string, McpServerV2> };
  importConflicts: string[];
  importConflictChoice: Record<string, ImportPick>;
  setImportConflictChoice: (fn: (prev: Record<string, ImportPick>) => Record<string, ImportPick>) => void;
  importConfirmDisabled: boolean;
  onConfirm: () => void;
  onHidden: () => void;
}) {
  const { saving, importPending, importConflicts, importConflictChoice, setImportConflictChoice, importConfirmDisabled, onConfirm, onHidden } = props;

  return (
    <BootstrapModal
      id="mcpImportConflictModal"
      title="导入冲突：逐项选择要保留的版本"
      dialogClassName="modal-lg modal-dialog-scrollable"
      footer={
        <>
          <button type="button" className="btn btn-light" data-bs-dismiss="modal">
            取消
          </button>
          <button type="button" className="btn btn-primary px-4" disabled={saving || importConfirmDisabled} onClick={onConfirm}>
            确认并生效
          </button>
        </>
      }
      onHidden={onHidden}
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
                <span className="badge bg-light text-danger border">冲突</span>
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
  );
}
