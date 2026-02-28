import { BootstrapModal } from '../../../components/BootstrapModal';

import type { SkillV1, SkillsTargetKey } from '../../../api/admin/skills';

export type SkillEditDraft = {
  id: string;
  title: string;
  description: string;
  prompt: string;
  enabledCodex: boolean;
  enabledClaude: boolean;
  enabledGemini: boolean;
};

export function SkillEditModal(props: {
  saving: boolean;

  editing: SkillV1 | null;
  setEditing: (v: SkillV1 | null) => void;

  draft: SkillEditDraft;
  setDraft: (fn: (prev: SkillEditDraft) => SkillEditDraft) => void;
  initDraft: (sk: SkillV1 | null) => SkillEditDraft;

  createMode: 'manual' | 'import';
  setCreateMode: (v: 'manual' | 'import') => void;

  importSource: SkillsTargetKey;
  setImportSource: (v: SkillsTargetKey) => void;
  importMode: 'merge' | 'replace';
  setImportMode: (v: 'merge' | 'replace') => void;
  importApplyAfter: boolean;
  setImportApplyAfter: (v: boolean) => void;

  onImport: () => void;
  onSave: () => void;
}) {
  const {
    saving,
    editing,
    setEditing,
    draft,
    setDraft,
    initDraft,
    createMode,
    setCreateMode,
    importSource,
    setImportSource,
    importMode,
    setImportMode,
    importApplyAfter,
    setImportApplyAfter,
    onImport,
    onSave,
  } = props;

  return (
    <BootstrapModal
      id="skillsEditModal"
      title={editing ? `编辑：${editing.id}` : '新增 Skill'}
      dialogClassName="modal-lg modal-dialog-scrollable"
      footer={
        <>
          <button type="button" className="btn btn-light" data-bs-dismiss="modal">
            取消
          </button>
          {!editing && createMode === 'import' ? (
            <button type="button" className="btn btn-primary px-4" disabled={saving} onClick={onImport}>
              导入并生效
            </button>
          ) : (
            <button type="button" className="btn btn-primary px-4" disabled={saving} onClick={onSave}>
              保存并生效
            </button>
          )}
        </>
      }
      onHidden={() => {
        setEditing(null);
        setDraft(() => initDraft(null));
        setCreateMode('import');
        setImportSource('claude');
        setImportMode('merge');
        setImportApplyAfter(true);
      }}
    >
      {!editing ? (
        <div className="btn-group w-100 mb-3" role="group" aria-label="skills-create-mode">
          <button type="button" className={`btn ${createMode === 'manual' ? 'btn-primary' : 'btn-outline-primary'}`} onClick={() => setCreateMode('manual')}>
            手动
          </button>
          <button type="button" className={`btn ${createMode === 'import' ? 'btn-primary' : 'btn-outline-primary'}`} onClick={() => setCreateMode('import')}>
            导入
          </button>
        </div>
      ) : null}

      {!editing && createMode === 'import' ? (
        <div className="row g-3">
          <div className="col-12 col-lg-6">
            <label className="form-label">来源</label>
            <select className="form-select" value={importSource} onChange={(e) => setImportSource(e.target.value as SkillsTargetKey)} disabled={saving}>
              <option value="codex">codex</option>
              <option value="claude">claude</option>
              <option value="gemini">gemini</option>
            </select>
          </div>
	          <div className="col-12 col-lg-6">
	            <label className="form-label">模式</label>
	            <select
	              className="form-select"
	              value={importMode}
	              onChange={(e) => setImportMode(e.target.value === 'replace' ? 'replace' : 'merge')}
	              disabled={saving}
	            >
	              <option value="merge">merge</option>
	              <option value="replace">replace</option>
	            </select>
	            <div className="form-text">replace 会覆盖 Realms 内的全部 skills。</div>
	          </div>
          <div className="col-12">
            <div className="form-check">
              <input className="form-check-input" type="checkbox" checked={importApplyAfter} onChange={(e) => setImportApplyAfter(e.target.checked)} disabled={saving} />
              <label className="form-check-label">导入后立即应用到目标目录</label>
            </div>
          </div>
        </div>
      ) : (
        <div className="row g-3">
          <div className="col-12 col-md-6">
            <label className="form-label">ID（文件名）</label>
            <input
              className="form-control font-monospace"
              placeholder="e.g. my-skill"
              value={draft.id}
              disabled={!!editing}
              onChange={(e) => setDraft((p) => ({ ...p, id: e.target.value }))}
            />
            <div className="form-text">只能包含字母/数字/._-，且不能包含路径分隔符。</div>
          </div>
          <div className="col-12 col-md-6">
            <label className="form-label">标题</label>
            <input className="form-control" placeholder="显示用标题" value={draft.title} onChange={(e) => setDraft((p) => ({ ...p, title: e.target.value }))} />
          </div>
          <div className="col-12">
            <label className="form-label">描述（可选）</label>
            <input className="form-control" value={draft.description} onChange={(e) => setDraft((p) => ({ ...p, description: e.target.value }))} />
          </div>
          <div className="col-12">
            <label className="form-label">Prompt</label>
            <textarea className="form-control font-monospace" rows={12} value={draft.prompt} onChange={(e) => setDraft((p) => ({ ...p, prompt: e.target.value }))} />
          </div>
          <div className="col-12">
            <div className="fw-semibold mb-2">目标启用</div>
            <div className="d-flex flex-wrap gap-3">
              <label className="form-check">
                <input className="form-check-input" type="checkbox" checked={draft.enabledCodex} onChange={(e) => setDraft((p) => ({ ...p, enabledCodex: e.target.checked }))} />
                <span className="form-check-label">codex</span>
              </label>
              <label className="form-check">
                <input className="form-check-input" type="checkbox" checked={draft.enabledClaude} onChange={(e) => setDraft((p) => ({ ...p, enabledClaude: e.target.checked }))} />
                <span className="form-check-label">claude</span>
              </label>
              <label className="form-check">
                <input className="form-check-input" type="checkbox" checked={draft.enabledGemini} onChange={(e) => setDraft((p) => ({ ...p, enabledGemini: e.target.checked }))} />
                <span className="form-check-label">gemini</span>
              </label>
            </div>
          </div>
        </div>
      )}
    </BootstrapModal>
  );
}
