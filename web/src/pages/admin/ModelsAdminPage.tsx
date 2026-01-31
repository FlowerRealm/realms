import { useEffect, useMemo, useState } from 'react';

import { BootstrapModal } from '../../components/BootstrapModal';
import { closeModalById } from '../../components/modal';
import {
  createManagedModelAdmin,
  deleteManagedModelAdmin,
  importModelPricingAdmin,
  listManagedModelsAdmin,
  lookupModelFromLibraryAdmin,
  updateManagedModelAdmin,
  type ImportModelPricingResult,
  type ManagedModel,
} from '../../api/models';

function statusBadge(status: number): { cls: string; label: string } {
  if (status === 1) return { cls: 'badge rounded-pill bg-success bg-opacity-10 text-success px-2', label: '启用' };
  return { cls: 'badge rounded-pill bg-secondary bg-opacity-10 text-secondary px-2', label: '禁用' };
}

type ModelForm = {
  public_id: string;
  owned_by: string;
  input_usd_per_1m: string;
  output_usd_per_1m: string;
  cache_input_usd_per_1m: string;
  cache_output_usd_per_1m: string;
  status: number;
};

function modelToForm(m: ManagedModel): ModelForm {
  return {
    public_id: m.public_id || '',
    owned_by: (m.owned_by || '').toString(),
    input_usd_per_1m: m.input_usd_per_1m || '0',
    output_usd_per_1m: m.output_usd_per_1m || '0',
    cache_input_usd_per_1m: m.cache_input_usd_per_1m || '0',
    cache_output_usd_per_1m: m.cache_output_usd_per_1m || '0',
    status: m.status || 0,
  };
}

export function ModelsAdminPage() {
  const [models, setModels] = useState<ManagedModel[]>([]);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [err, setErr] = useState('');
  const [notice, setNotice] = useState('');

  const [createLookupLoading, setCreateLookupLoading] = useState(false);
  const [createLookupErr, setCreateLookupErr] = useState('');
  const [createLookupNotice, setCreateLookupNotice] = useState('');
  const [createIconPreview, setCreateIconPreview] = useState<string | null>(null);

  const [importPricingJSON, setImportPricingJSON] = useState('');
  const [importPricingLoading, setImportPricingLoading] = useState(false);
  const [importPricingErr, setImportPricingErr] = useState('');
  const [importPricingResult, setImportPricingResult] = useState<ImportModelPricingResult | null>(null);

  const [createForm, setCreateForm] = useState<ModelForm>({
    public_id: '',
    owned_by: '',
    input_usd_per_1m: '5',
    output_usd_per_1m: '15',
    cache_input_usd_per_1m: '0',
    cache_output_usd_per_1m: '0',
    status: 1,
  });

  const [editing, setEditing] = useState<ManagedModel | null>(null);
  const [editForm, setEditForm] = useState<ModelForm>({
    public_id: '',
    owned_by: '',
    input_usd_per_1m: '0',
    output_usd_per_1m: '0',
    cache_input_usd_per_1m: '0',
    cache_output_usd_per_1m: '0',
    status: 1,
  });

  const enabledCount = useMemo(() => models.filter((m) => m.status === 1).length, [models]);

  async function refresh() {
    setErr('');
    setNotice('');
    setLoading(true);
    try {
      const res = await listManagedModelsAdmin(1, 1000);
      if (!res.success) throw new Error(res.message || '加载失败');
      setModels(res.data?.items || []);
    } catch (e) {
      setErr(e instanceof Error ? e.message : '加载失败');
      setModels([]);
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    void refresh();
  }, []);

  useEffect(() => {
    if (!editing) return;
    setEditForm(modelToForm(editing));
  }, [editing]);

  return (
    <div className="fade-in-up">
      <div className="row g-4">
        <div className="col-12">
          <div className="card">
            <div className="card-body d-flex flex-column flex-md-row justify-content-between align-items-center">
              <div className="d-flex align-items-center mb-3 mb-md-0">
                <div className="bg-info bg-opacity-10 text-info rounded-circle d-flex align-items-center justify-content-center me-3" style={{ width: 48, height: 48 }}>
                  <span className="fs-4 material-symbols-rounded">smart_toy</span>
                </div>
                <div>
                  <h5 className="mb-1 fw-semibold">模型管理</h5>
                  <p className="mb-0 text-muted small">
                    {enabledCount} 启用 / {models.length} 总计 · 模型目录是对外暴露与强制校验的唯一来源（白名单）
                  </p>
                </div>
              </div>
              <div className="d-flex gap-2">
                <button type="button" className="btn btn-light border btn-sm" data-bs-toggle="modal" data-bs-target="#importPricingModal">
                  <span className="me-1 material-symbols-rounded">upload</span> 导入价格表
                </button>
                <button type="button" className="btn btn-primary btn-sm" data-bs-toggle="modal" data-bs-target="#createModelModal">
                  <span className="me-1 material-symbols-rounded">add</span> 新增模型
                </button>
              </div>
            </div>
          </div>
        </div>

        {notice ? (
          <div className="col-12">
            <div className="alert alert-success d-flex align-items-center" role="alert">
              <span className="me-2 material-symbols-rounded">check_circle</span>
              <div>{notice}</div>
            </div>
          </div>
        ) : null}

        {err ? (
          <div className="col-12">
            <div className="alert alert-danger d-flex align-items-center" role="alert">
              <span className="me-2 material-symbols-rounded">warning</span>
              <div>{err}</div>
            </div>
          </div>
        ) : null}

        <div className="col-12">
          <div className="card overflow-hidden mb-0">
            <div className="table-responsive">
              <table className="table table-hover align-middle mb-0">
                <thead className="table-light">
                  <tr>
                    <th className="ps-4">对外 ID</th>
                    <th>归属方</th>
                    <th>
                      计费 <span className="text-muted small">（每 1M Token）</span>
                    </th>
                    <th>状态</th>
                    <th className="text-end pe-4">操作</th>
                  </tr>
                </thead>
                <tbody>
                  {loading ? (
                    <tr>
                      <td colSpan={5} className="text-center py-5 text-muted">
                        加载中…
                      </td>
                    </tr>
                  ) : models.length === 0 ? (
                    <tr>
                      <td colSpan={5} className="text-center py-5 text-muted">
                        <span className="fs-1 d-block mb-3 material-symbols-rounded">inbox</span>
                        暂无模型，请先新增模型后再对外提供服务。
                      </td>
                    </tr>
                  ) : (
                    models.map((m) => {
                      const st = statusBadge(m.status);
                      return (
                        <tr key={m.id}>
                          <td className="ps-4 fw-semibold text-dark">
                            <div className="d-flex align-items-center gap-2">
                              {m.icon_url ? (
                                <img
                                  className="rlm-model-icon"
                                  src={m.icon_url}
                                  alt={m.owned_by || 'realms'}
                                  title={m.owned_by || 'realms'}
                                  loading="lazy"
                                  onError={(e) => {
                                    (e.currentTarget as HTMLImageElement).style.display = 'none';
                                  }}
                                />
                              ) : null}
                              <span className="font-monospace">{m.public_id}</span>
                            </div>
                          </td>
                          <td>
                            {m.owned_by ? <span className="badge rounded-pill bg-light text-secondary border px-2">{m.owned_by}</span> : <span className="text-muted small">-</span>}
                          </td>
                          <td className="small text-nowrap">
                              <div className="d-flex flex-wrap align-items-center gap-3">
                                <div className="d-flex align-items-center">
                                <span className="text-muted me-2 smaller">输入</span>
                                  <span className="fw-bold text-dark">${m.input_usd_per_1m}</span>
                                </div>
                                <div className="d-flex align-items-center">
                                <span className="text-muted me-2 smaller">输出</span>
                                  <span className="fw-bold text-dark">${m.output_usd_per_1m}</span>
                                </div>
                                <div className="d-flex align-items-center">
                                <span className="text-muted me-2 smaller">缓存输入</span>
                                  <span className="fw-bold text-dark">${m.cache_input_usd_per_1m}</span>
                                </div>
                                <div className="d-flex align-items-center">
                                <span className="text-muted me-2 smaller">缓存输出</span>
                                  <span className="fw-bold text-dark">${m.cache_output_usd_per_1m}</span>
                                </div>
                              </div>
                          </td>
                          <td>
                            <span className={st.cls}>{st.label}</span>
                          </td>
                          <td className="text-end pe-4 text-nowrap">
                            <div className="d-inline-flex gap-1">
                              <button
                                className="btn btn-sm btn-light border text-primary"
                                type="button"
                                title="编辑"
                                data-bs-toggle="modal"
                                data-bs-target="#editModelModal"
                                onClick={() => setEditing(m)}
                              >
                                <i className="ri-settings-3-line"></i>
                              </button>
                              <button
                                className="btn btn-sm btn-light border text-secondary"
                                type="button"
                                title={m.status === 1 ? '禁用' : '启用'}
                                disabled={loading}
                                onClick={async () => {
                                  setErr('');
                                  setNotice('');
                                  try {
                                    const res = await updateManagedModelAdmin({ ...m, status: m.status === 1 ? 0 : 1 }, true);
                                    if (!res.success) throw new Error(res.message || '更新失败');
                                    setNotice('已保存');
                                    await refresh();
                                  } catch (e) {
                                    setErr(e instanceof Error ? e.message : '更新失败');
                                  }
                                }}
                              >
                                {m.status === 1 ? '禁用' : '启用'}
                              </button>
                              <button
                                className="btn btn-sm btn-light border text-danger"
                                type="button"
                                title="删除"
                                disabled={loading}
                                onClick={async () => {
                                  if (!window.confirm(`确认删除该模型？不可恢复。`)) return;
                                  setErr('');
                                  setNotice('');
                                  try {
                                    const res = await deleteManagedModelAdmin(m.id);
                                    if (!res.success) throw new Error(res.message || '删除失败');
                                    setNotice('已删除');
                                    if (editing?.id === m.id) setEditing(null);
                                    await refresh();
                                  } catch (e) {
                                    setErr(e instanceof Error ? e.message : '删除失败');
                                  }
                                }}
                              >
                                <i className="ri-delete-bin-line"></i>
                              </button>
                            </div>
                          </td>
                        </tr>
                      );
                    })
                  )}
                </tbody>
              </table>
            </div>
          </div>
          <div className="text-muted small mt-2">
            提示：模型“是否可用”还取决于是否已在“上游渠道”中绑定（channel_models）。
          </div>
        </div>
      </div>

      <BootstrapModal
        id="createModelModal"
        title="新增模型"
        dialogClassName="modal-dialog-centered modal-lg"
        onHidden={() => {
          setCreateForm({
            public_id: '',
            owned_by: '',
            input_usd_per_1m: '5',
            output_usd_per_1m: '15',
            cache_input_usd_per_1m: '0',
            cache_output_usd_per_1m: '0',
            status: 1,
          });
          setCreateLookupErr('');
          setCreateLookupNotice('');
          setCreateIconPreview(null);
        }}
      >
        <form
          className="row g-3"
          onSubmit={async (e) => {
            e.preventDefault();
            setErr('');
            setNotice('');
            setSaving(true);
            try {
              const res = await createManagedModelAdmin({
                public_id: createForm.public_id.trim(),
                owned_by: createForm.owned_by.trim() ? createForm.owned_by.trim() : null,
                input_usd_per_1m: createForm.input_usd_per_1m,
                output_usd_per_1m: createForm.output_usd_per_1m,
                cache_input_usd_per_1m: createForm.cache_input_usd_per_1m,
                cache_output_usd_per_1m: createForm.cache_output_usd_per_1m,
                status: createForm.status,
              });
              if (!res.success) throw new Error(res.message || '创建失败');
              setNotice('已创建');
              closeModalById('createModelModal');
              await refresh();
            } catch (e) {
              setErr(e instanceof Error ? e.message : '创建失败');
            } finally {
              setSaving(false);
            }
          }}
        >
          <div className="col-md-8">
            <label className="form-label">对外 ID（public_id）</label>
            <div className="input-group">
              <input
                className="form-control font-monospace"
                value={createForm.public_id}
                onChange={(e) => {
                  setCreateForm((p) => ({ ...p, public_id: e.target.value }));
                  setCreateLookupErr('');
                  setCreateLookupNotice('');
                }}
                required
                placeholder="例如：gpt-4.1-mini"
              />
              <button
                className="btn btn-outline-secondary"
                type="button"
                disabled={createLookupLoading || !createForm.public_id.trim()}
                onClick={async () => {
                  const modelID = createForm.public_id.trim();
                  if (!modelID) {
                    setCreateLookupErr('请先填写对外 ID');
                    return;
                  }
                  setCreateLookupErr('');
                  setCreateLookupNotice('');
                  setCreateLookupLoading(true);
                  try {
                    const res = await lookupModelFromLibraryAdmin(modelID);
                    if (!res.success) throw new Error(res.message || '查询失败');
                    const d = res.data;
                    if (d) {
                      setCreateForm((p) => ({
                        ...p,
                        owned_by: d.owned_by || '',
                        input_usd_per_1m: d.input_usd_per_1m || p.input_usd_per_1m,
                        output_usd_per_1m: d.output_usd_per_1m || p.output_usd_per_1m,
                        cache_input_usd_per_1m: d.cache_input_usd_per_1m || p.cache_input_usd_per_1m,
                        cache_output_usd_per_1m: d.cache_output_usd_per_1m || p.cache_output_usd_per_1m,
                      }));
                      setCreateIconPreview(d.icon_url || null);
                    }
                    setCreateLookupNotice(res.message || '已从模型库填充');
                  } catch (e) {
                    setCreateLookupErr(e instanceof Error ? e.message : '查询失败');
                  } finally {
                    setCreateLookupLoading(false);
                  }
                }}
              >
                {createLookupLoading ? '查询中…' : '从模型库填充'}
              </button>
            </div>
            <div className="form-text small text-muted">
              数据来源：models.dev（GitHub 开源模型目录）。将填充“归属方/输入单价/输出单价/缓存输入单价/缓存输出单价”，不会自动保存。如遇多个候选可用{' '}
              <code>openai/gpt-4o</code> 形式指定 provider。
              {createIconPreview ? (
                <img
                  className="rlm-model-icon ms-2"
                  src={createIconPreview}
                  alt="icon"
                  loading="lazy"
                  onError={(e) => {
                    (e.currentTarget as HTMLImageElement).style.display = 'none';
                  }}
                />
              ) : null}
            </div>
            {createLookupNotice ? (
              <div className="text-success small mt-1">
                <i className="ri-checkbox-circle-line me-1"></i>
                {createLookupNotice}
              </div>
            ) : null}
            {createLookupErr ? (
              <div className="text-danger small mt-1">
                <i className="ri-error-warning-line me-1"></i>
                {createLookupErr}
              </div>
            ) : null}
          </div>
          <div className="col-md-4">
            <label className="form-label">状态</label>
            <select className="form-select" value={createForm.status} onChange={(e) => setCreateForm((p) => ({ ...p, status: Number.parseInt(e.target.value, 10) || 0 }))}>
              <option value={1}>启用</option>
              <option value={0}>禁用</option>
            </select>
          </div>
          <div className="col-12">
            <label className="form-label">归属方（owned_by，可选）</label>
            <input className="form-control" value={createForm.owned_by} onChange={(e) => setCreateForm((p) => ({ ...p, owned_by: e.target.value }))} placeholder="例如：openai / anthropic / internal" />
          </div>

          <div className="col-md-6">
            <label className="form-label">输入单价</label>
            <div className="input-group">
              <span className="input-group-text">$</span>
              <input className="form-control" value={createForm.input_usd_per_1m} onChange={(e) => setCreateForm((p) => ({ ...p, input_usd_per_1m: e.target.value }))} inputMode="decimal" />
              <span className="input-group-text">/ 1M Token</span>
            </div>
          </div>
          <div className="col-md-6">
            <label className="form-label">输出单价</label>
            <div className="input-group">
              <span className="input-group-text">$</span>
              <input className="form-control" value={createForm.output_usd_per_1m} onChange={(e) => setCreateForm((p) => ({ ...p, output_usd_per_1m: e.target.value }))} inputMode="decimal" />
              <span className="input-group-text">/ 1M Token</span>
            </div>
          </div>

          <div className="col-md-6">
            <label className="form-label">缓存输入单价</label>
            <div className="input-group">
              <span className="input-group-text">$</span>
              <input className="form-control" value={createForm.cache_input_usd_per_1m} onChange={(e) => setCreateForm((p) => ({ ...p, cache_input_usd_per_1m: e.target.value }))} inputMode="decimal" />
              <span className="input-group-text">/ 1M Token</span>
            </div>
          </div>
          <div className="col-md-6">
            <label className="form-label">缓存输出单价</label>
            <div className="input-group">
              <span className="input-group-text">$</span>
              <input className="form-control" value={createForm.cache_output_usd_per_1m} onChange={(e) => setCreateForm((p) => ({ ...p, cache_output_usd_per_1m: e.target.value }))} inputMode="decimal" />
              <span className="input-group-text">/ 1M Token</span>
            </div>
          </div>

          <div className="alert alert-light border small mb-0 d-flex align-items-start">
            <span className="material-symbols-rounded text-primary me-2 mt-1">info</span>
            <div>单位说明：USD / 1M Token（支持最多 6 位小数）。</div>
          </div>

          <div className="modal-footer border-top-0 px-0 pb-0">
            <button type="button" className="btn btn-light" data-bs-dismiss="modal">
              取消
            </button>
            <button className="btn btn-primary px-4" type="submit" disabled={saving || !createForm.public_id.trim()}>
              保存
            </button>
          </div>
        </form>
      </BootstrapModal>

      <BootstrapModal
        id="importPricingModal"
        title="导入价格表"
        dialogClassName="modal-dialog-centered modal-lg"
        onHidden={() => {
          setImportPricingJSON('');
          setImportPricingErr('');
          setImportPricingResult(null);
        }}
      >
        {importPricingErr ? (
          <div className="alert alert-danger d-flex align-items-center" role="alert">
            <i className="ri-alert-line me-2"></i>
            <div>{importPricingErr}</div>
          </div>
        ) : null}
        {importPricingResult ? (
          <div className="alert alert-success d-flex align-items-center" role="alert">
            <i className="ri-checkbox-circle-line me-2"></i>
            <div>
              导入完成：新增 {importPricingResult.added.length}，更新 {importPricingResult.updated.length}，无变化 {importPricingResult.unchanged.length}，失败{' '}
              {Object.keys(importPricingResult.failed || {}).length}。
            </div>
          </div>
        ) : null}
        <form
          className="row g-3"
          onSubmit={async (e) => {
            e.preventDefault();
            setImportPricingErr('');
            setImportPricingResult(null);
            setErr('');
            setNotice('');
            setImportPricingLoading(true);
            try {
              const res = await importModelPricingAdmin(importPricingJSON);
              if (!res.success) throw new Error(res.message || '导入失败');
              setNotice(res.message || '导入完成');
              setImportPricingResult(res.data || null);
              await refresh();
            } catch (e) {
              setImportPricingErr(e instanceof Error ? e.message : '导入失败');
            } finally {
              setImportPricingLoading(false);
            }
          }}
        >
          <div className="col-12">
            <label className="form-label fw-medium">JSON 文件（可选）</label>
            <input
              className="form-control"
              type="file"
              accept="application/json"
              onChange={(e) => {
                const file = e.target.files?.[0];
                if (!file) return;
                setImportPricingErr('');
                void file
                  .text()
                  .then((txt) => setImportPricingJSON(txt))
                  .catch(() => setImportPricingErr('读取文件失败'));
              }}
            />
            <div className="form-text small text-muted">上传后将读取文件内容并填充到下方文本框。</div>
          </div>

          <div className="col-12">
            <label className="form-label fw-medium">粘贴 JSON</label>
            <textarea
              className="form-control font-monospace"
              rows={10}
              value={importPricingJSON}
              onChange={(e) => setImportPricingJSON(e.target.value)}
              placeholder='{"gpt-4.1-mini":{"input_usd_per_1m":0.15,"output_usd_per_1m":0.60,"cache_input_usd_per_1m":0.00,"cache_output_usd_per_1m":0.00}}'
              required
            />
            <div className="form-text small text-muted">顶层支持对象或数组；支持 usd_per_1m 或 cost_per_token 格式。</div>
          </div>

          <div className="modal-footer border-top-0 px-0 pb-0">
            <button type="button" className="btn btn-light" data-bs-dismiss="modal">
              取消
            </button>
            <button className="btn btn-primary px-4" type="submit" disabled={importPricingLoading || !importPricingJSON.trim()}>
              {importPricingLoading ? '导入中…' : '导入'}
            </button>
          </div>
        </form>

        {importPricingResult ? (
          <div className="mt-3">
            <div className="row g-3">
              <div className="col-md-4">
                <details open>
                  <summary className="fw-semibold">新增（{importPricingResult.added.length}）</summary>
                  {importPricingResult.added.length === 0 ? (
                    <div className="text-muted small mt-2">无</div>
                  ) : (
                    <ul className="mt-2 mb-0 small">
                      {importPricingResult.added.map((id) => (
                        <li key={id}>
                          <code className="user-select-all">{id}</code>
                        </li>
                      ))}
                    </ul>
                  )}
                </details>
              </div>
              <div className="col-md-4">
                <details open>
                  <summary className="fw-semibold">更新（{importPricingResult.updated.length}）</summary>
                  {importPricingResult.updated.length === 0 ? (
                    <div className="text-muted small mt-2">无</div>
                  ) : (
                    <ul className="mt-2 mb-0 small">
                      {importPricingResult.updated.map((id) => (
                        <li key={id}>
                          <code className="user-select-all">{id}</code>
                        </li>
                      ))}
                    </ul>
                  )}
                </details>
              </div>
              <div className="col-md-4">
                <details>
                  <summary className="fw-semibold">无变化（{importPricingResult.unchanged.length}）</summary>
                  {importPricingResult.unchanged.length === 0 ? (
                    <div className="text-muted small mt-2">无</div>
                  ) : (
                    <ul className="mt-2 mb-0 small">
                      {importPricingResult.unchanged.map((id) => (
                        <li key={id}>
                          <code className="user-select-all">{id}</code>
                        </li>
                      ))}
                    </ul>
                  )}
                </details>
              </div>
              <div className="col-12">
                <details open>
                  <summary className="fw-semibold">失败（{Object.keys(importPricingResult.failed || {}).length}）</summary>
                  {Object.keys(importPricingResult.failed || {}).length === 0 ? (
                    <div className="text-muted small mt-2">无</div>
                  ) : (
                    <div className="table-responsive mt-2">
                      <table className="table table-sm table-hover align-middle mb-0">
                        <thead className="table-light">
                          <tr>
                            <th style={{ width: '35%' }}>条目</th>
                            <th>原因</th>
                          </tr>
                        </thead>
                        <tbody>
                          {Object.entries(importPricingResult.failed).map(([key, reason]) => (
                            <tr key={key}>
                              <td>
                                <code className="user-select-all">{key}</code>
                              </td>
                              <td className="text-muted small">{reason}</td>
                            </tr>
                          ))}
                        </tbody>
                      </table>
                    </div>
                  )}
                </details>
                <div className="form-text small text-muted mt-2">
                  提示：失败条目不会影响其他条目导入；建议修复 JSON 后再次导入（支持重复导入，后者覆盖前者）。
                </div>
              </div>
              <div className="col-12 d-flex justify-content-end">
                <button type="button" className="btn btn-outline-secondary" onClick={() => closeModalById('importPricingModal')}>
                  关闭
                </button>
              </div>
            </div>
          </div>
        ) : null}
      </BootstrapModal>

      <BootstrapModal
        id="editModelModal"
        title={editing ? `编辑模型：${editing.public_id}` : '编辑模型'}
        dialogClassName="modal-dialog-centered modal-lg"
        onHidden={() => setEditing(null)}
      >
        {!editing ? (
          <div className="text-muted">未选择模型。</div>
        ) : (
          <form
            className="row g-3"
            onSubmit={async (e) => {
              e.preventDefault();
              if (!editing) return;
              setErr('');
              setNotice('');
              setSaving(true);
              try {
                const res = await updateManagedModelAdmin({
                  ...editing,
                  public_id: editForm.public_id.trim(),
                  owned_by: editForm.owned_by.trim() ? editForm.owned_by.trim() : null,
                  input_usd_per_1m: editForm.input_usd_per_1m,
                  output_usd_per_1m: editForm.output_usd_per_1m,
                  cache_input_usd_per_1m: editForm.cache_input_usd_per_1m,
                  cache_output_usd_per_1m: editForm.cache_output_usd_per_1m,
                  status: editForm.status,
                });
                if (!res.success) throw new Error(res.message || '保存失败');
                setNotice('已保存');
                closeModalById('editModelModal');
                await refresh();
              } catch (e) {
                setErr(e instanceof Error ? e.message : '保存失败');
              } finally {
                setSaving(false);
              }
            }}
          >
            <div className="col-md-8">
              <label className="form-label">对外 ID（public_id）</label>
              <input className="form-control font-monospace" value={editForm.public_id} onChange={(e) => setEditForm((p) => ({ ...p, public_id: e.target.value }))} required />
            </div>
            <div className="col-md-4">
              <label className="form-label">状态</label>
              <select className="form-select" value={editForm.status} onChange={(e) => setEditForm((p) => ({ ...p, status: Number.parseInt(e.target.value, 10) || 0 }))}>
                <option value={1}>启用</option>
                <option value={0}>禁用</option>
              </select>
            </div>
            <div className="col-12">
              <label className="form-label">归属方（owned_by，可选）</label>
              <input className="form-control" value={editForm.owned_by} onChange={(e) => setEditForm((p) => ({ ...p, owned_by: e.target.value }))} />
            </div>

            <div className="col-md-6">
              <label className="form-label">输入单价</label>
              <div className="input-group">
                <span className="input-group-text">$</span>
                <input className="form-control" value={editForm.input_usd_per_1m} onChange={(e) => setEditForm((p) => ({ ...p, input_usd_per_1m: e.target.value }))} inputMode="decimal" />
                <span className="input-group-text">/ 1M Token</span>
              </div>
            </div>
            <div className="col-md-6">
              <label className="form-label">输出单价</label>
              <div className="input-group">
                <span className="input-group-text">$</span>
                <input className="form-control" value={editForm.output_usd_per_1m} onChange={(e) => setEditForm((p) => ({ ...p, output_usd_per_1m: e.target.value }))} inputMode="decimal" />
                <span className="input-group-text">/ 1M Token</span>
              </div>
            </div>
            <div className="col-md-6">
              <label className="form-label">缓存输入单价</label>
              <div className="input-group">
                <span className="input-group-text">$</span>
                <input className="form-control" value={editForm.cache_input_usd_per_1m} onChange={(e) => setEditForm((p) => ({ ...p, cache_input_usd_per_1m: e.target.value }))} inputMode="decimal" />
                <span className="input-group-text">/ 1M Token</span>
              </div>
            </div>
            <div className="col-md-6">
              <label className="form-label">缓存输出单价</label>
              <div className="input-group">
                <span className="input-group-text">$</span>
                <input className="form-control" value={editForm.cache_output_usd_per_1m} onChange={(e) => setEditForm((p) => ({ ...p, cache_output_usd_per_1m: e.target.value }))} inputMode="decimal" />
                <span className="input-group-text">/ 1M Token</span>
              </div>
            </div>

            <div className="modal-footer border-top-0 px-0 pb-0">
              <button type="button" className="btn btn-light" data-bs-dismiss="modal">
                取消
              </button>
              <button className="btn btn-primary px-4" type="submit" disabled={saving}>
                保存
              </button>
            </div>
          </form>
        )}
      </BootstrapModal>
    </div>
  );
}
