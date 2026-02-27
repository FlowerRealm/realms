import { BootstrapModal } from '../../../components/BootstrapModal';

import type { ImportSource, McpType, Row } from '../mcpTypes';
import type { EditFormState } from '../mcpEditForm';

export function McpServerEditModal(props: {
  isPersonalBuild: boolean;
  saving: boolean;

  editing: Row | null;
  setEditing: (v: Row | null) => void;

  form: EditFormState;
  setForm: (fn: (prev: EditFormState) => EditFormState) => void;
  initForm: (row: Row | null) => EditFormState;

  createMode: 'manual' | 'import';
  setCreateMode: (v: 'manual' | 'import') => void;
  importSource: ImportSource;
  setImportSource: (v: ImportSource) => void;
  importContent: string;
  setImportContent: (v: string) => void;

  onImport: () => void;
  onSave: () => void;
  onHiddenReset: () => void;
}) {
  const {
    isPersonalBuild,
    saving,
    editing,
    setEditing,
    form,
    setForm,
    initForm,
    createMode,
    setCreateMode,
    importSource,
    setImportSource,
    importContent,
    setImportContent,
    onImport,
    onSave,
    onHiddenReset,
  } = props;

  return (
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
        setForm(() => initForm(null));
        setCreateMode('import');
        setImportSource('claude');
        setImportContent('');
        onHiddenReset();
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
            <label className="form-label">来源</label>
            <select className="form-select" value={importSource} onChange={(e) => setImportSource(e.target.value as ImportSource)} disabled={saving}>
              <option value="claude">claude (JSON)</option>
              <option value="codex">codex (TOML/JSON)</option>
              <option value="gemini">gemini (JSON)</option>
              <option value="realms">realms (StoreV2 JSON)</option>
            </select>
          </div>
          <div className="col-12">
            <label className="form-label">内容</label>
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
            <label className="form-label">ID</label>
            <input className="form-control font-monospace" value={form.id} onChange={(e) => setForm((p) => ({ ...p, id: e.target.value }))} disabled={!!editing} placeholder="my-mcp" />
          </div>
          <div className="col-12">
            <label className="form-label">类型</label>
            <select className="form-select" value={form.type} onChange={(e) => setForm((p) => ({ ...p, type: e.target.value as McpType }))}>
              <option value="stdio">stdio</option>
              <option value="http">http</option>
              <option value="sse">sse</option>
            </select>
          </div>

          {form.type === 'stdio' ? (
            <>
              <div className="col-12">
                <label className="form-label">命令</label>
                <input className="form-control font-monospace" value={form.command} onChange={(e) => setForm((p) => ({ ...p, command: e.target.value }))} placeholder="npx @xxx/mcp" />
              </div>
              <div className="col-12">
                <label className="form-label">参数（可选）</label>
                <input
                  className="form-control font-monospace"
                  value={(form.args || []).join(' ')}
                  onChange={(e) => setForm((p) => ({ ...p, args: (e.target.value || '').split(' ').filter(Boolean) }))}
                  placeholder="--foo bar"
                />
              </div>
              <div className="col-12">
                <label className="form-label">工作目录（可选）</label>
                <input className="form-control font-monospace" value={form.cwd} onChange={(e) => setForm((p) => ({ ...p, cwd: e.target.value }))} placeholder="/path/to/project" />
              </div>
              <div className="col-12">
                <label className="form-label">环境变量（可选）</label>
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
                          placeholder="键"
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
                          placeholder="值"
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
                <label className="form-label">URL</label>
                <input className="form-control font-monospace" value={form.url} onChange={(e) => setForm((p) => ({ ...p, url: e.target.value }))} placeholder="https://example.com/mcp" />
              </div>
              <div className="col-12">
                <label className="form-label">Bearer Token 环境变量（可选）</label>
                <input className="form-control font-monospace" value={form.bearer_token_env_var} onChange={(e) => setForm((p) => ({ ...p, bearer_token_env_var: e.target.value }))} placeholder="MY_TOKEN_ENV" />
              </div>
              <div className="col-12">
                <label className="form-label">HTTP Headers（可选）</label>
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
                          placeholder="键"
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
                          placeholder="值"
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
                  <button
                    type="button"
                    className="btn btn-light border btn-sm align-self-start"
                    onClick={() => setForm((p) => ({ ...p, http_headers: [...(p.http_headers || []), { k: '', v: '' }] }))}
                  >
                    + 添加请求头
                  </button>
                </div>
              </div>
            </>
          )}

          <div className="col-12">
            <details>
              <summary className="text-muted small">高级：超时（毫秒）</summary>
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
  );
}
