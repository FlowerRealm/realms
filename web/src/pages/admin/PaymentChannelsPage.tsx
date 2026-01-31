import { useEffect, useMemo, useState } from 'react';

import { BootstrapModal } from '../../components/BootstrapModal';
import { closeModalById } from '../../components/modal';
import {
  createAdminPaymentChannel,
  deleteAdminPaymentChannel,
  listAdminPaymentChannels,
  updateAdminPaymentChannel,
  type AdminPaymentChannel,
} from '../../api/admin/paymentChannels';

function statusBadge(status: number): { cls: string; label: string } {
  if (status === 1) return { cls: 'badge rounded-pill bg-success bg-opacity-10 text-success px-2', label: '启用' };
  return { cls: 'badge rounded-pill bg-secondary bg-opacity-10 text-secondary px-2', label: '禁用' };
}

function usableBadge(usable: boolean): { cls: string; label: string } {
  if (usable) return { cls: 'badge rounded-pill bg-success bg-opacity-10 text-success px-2', label: '可用' };
  return { cls: 'badge rounded-pill bg-warning bg-opacity-10 text-warning px-2', label: '待完善' };
}

type PaymentType = 'stripe' | 'epay';

export function PaymentChannelsPage() {
  const [items, setItems] = useState<AdminPaymentChannel[]>([]);
  const [loading, setLoading] = useState(true);
  const [err, setErr] = useState('');
  const [notice, setNotice] = useState('');

  const [createType, setCreateType] = useState<PaymentType>('stripe');
  const [createName, setCreateName] = useState('');
  const [createEnabled, setCreateEnabled] = useState(true);
  const [createStripeCurrency, setCreateStripeCurrency] = useState('cny');
  const [createStripeSecret, setCreateStripeSecret] = useState('');
  const [createStripeWebhookSecret, setCreateStripeWebhookSecret] = useState('');
  const [createEPayGateway, setCreateEPayGateway] = useState('');
  const [createEPayPartnerID, setCreateEPayPartnerID] = useState('');
  const [createEPayKey, setCreateEPayKey] = useState('');

  const [editing, setEditing] = useState<AdminPaymentChannel | null>(null);
  const [editName, setEditName] = useState('');
  const [editEnabled, setEditEnabled] = useState(true);
  const [editStripeCurrency, setEditStripeCurrency] = useState('');
  const [editStripeSecret, setEditStripeSecret] = useState('');
  const [editStripeWebhookSecret, setEditStripeWebhookSecret] = useState('');
  const [editEPayGateway, setEditEPayGateway] = useState('');
  const [editEPayPartnerID, setEditEPayPartnerID] = useState('');
  const [editEPayKey, setEditEPayKey] = useState('');

  const enabledCount = useMemo(() => items.filter((x) => x.status === 1).length, [items]);

  async function refresh() {
    setErr('');
    setNotice('');
    setLoading(true);
    try {
      const res = await listAdminPaymentChannels();
      if (!res.success) throw new Error(res.message || '加载失败');
      setItems(res.data || []);
    } catch (e) {
      setErr(e instanceof Error ? e.message : '加载失败');
      setItems([]);
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    void refresh();
  }, []);

  useEffect(() => {
    if (!editing) return;
    setEditName(editing.name || '');
    setEditEnabled(editing.status === 1);
    setEditStripeCurrency(editing.stripe_currency || 'cny');
    setEditStripeSecret('');
    setEditStripeWebhookSecret('');
    setEditEPayGateway(editing.epay_gateway || '');
    setEditEPayPartnerID(editing.epay_partner_id || '');
    setEditEPayKey('');
  }, [editing]);

  return (
    <div className="fade-in-up">
      <div className="row g-4">
        <div className="col-12">
          <div className="card">
            <div className="card-body d-flex flex-column flex-md-row justify-content-between align-items-center">
              <div className="d-flex align-items-center mb-3 mb-md-0">
                <div
                  className="bg-warning bg-opacity-10 text-warning rounded-circle d-flex align-items-center justify-content-center me-3"
                  style={{ width: 48, height: 48 }}
                >
                  <span className="fs-4 material-symbols-rounded">credit_card</span>
                </div>
                <div>
                  <h5 className="mb-1 fw-semibold">支付渠道</h5>
                  <p className="mb-0 text-muted small">
                    {enabledCount} 启用 / {items.length} 总计
                  </p>
                </div>
              </div>

              <div className="d-flex gap-2">
                <button type="button" className="btn btn-primary btn-sm" data-bs-toggle="modal" data-bs-target="#createPaymentChannelModal">
                  <span className="material-symbols-rounded me-1">add</span> 新建支付渠道
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
          {loading ? (
            <div className="text-muted">加载中…</div>
          ) : items.length === 0 ? (
            <div className="text-center py-5 text-muted">
              <span className="fs-1 d-block mb-3 material-symbols-rounded">inbox</span>
              暂无支付渠道。
            </div>
          ) : (
            <div className="card border-0">
              <div className="card-body">
                <h5 className="fw-semibold mb-3">渠道列表</h5>
                <div className="table-responsive">
                  <table className="table table-hover align-middle mb-0">
                    <thead className="table-light">
                      <tr>
                        <th className="ps-3">渠道详情</th>
                        <th>状态</th>
                        <th>健康/配置</th>
                        <th>创建时间</th>
                        <th className="text-end pe-3">操作</th>
                      </tr>
                    </thead>
                    <tbody>
                      {items.map((ch) => {
                        const st = statusBadge(ch.status);
                        const usable = usableBadge(ch.usable);
                        return (
                          <tr key={ch.id}>
                            <td className="ps-3">
                              <div className="d-flex flex-column">
                                <span className="fw-bold text-dark">{ch.name}</span>
                                <span className="badge bg-light text-secondary border smaller mt-1" style={{ width: 'fit-content' }}>
                                  {ch.type_label}
                                </span>
                              </div>
                            </td>
                            <td>
                              <span className={st.cls}>{st.label}</span>
                            </td>
                            <td>
                              <span className={usable.cls} title={ch.usable ? '配置完整且已启用' : '缺少关键配置或已禁用'}>
                                {usable.label}
                              </span>
                            </td>
                            <td className="text-muted small">{ch.created_at}</td>
                            <td className="text-end pe-3 text-nowrap">
                              <div className="d-inline-flex gap-1">
                                <button
                                  type="button"
                                  className="btn btn-sm btn-light border text-primary"
                                  title="编辑配置"
                                  data-bs-toggle="modal"
                                  data-bs-target="#editPaymentChannelModal"
                                  onClick={() => setEditing(ch)}
                                >
                                  <i className="ri-settings-3-line"></i>
                                </button>
                                <button
                                  type="button"
                                  className="btn btn-sm btn-light border text-danger"
                                  title="删除渠道"
                                  onClick={async () => {
                                    if (!window.confirm(`确认删除支付渠道 ${ch.name}? 此操作不可撤销。`)) return;
                                    setErr('');
                                    setNotice('');
                                    try {
                                      const res = await deleteAdminPaymentChannel(ch.id);
                                      if (!res.success) throw new Error(res.message || '删除失败');
                                      setNotice(res.message || '已删除');
                                      if (editing?.id === ch.id) setEditing(null);
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
                      })}
                    </tbody>
                  </table>
                </div>
              </div>
            </div>
          )}
        </div>
      </div>

      <BootstrapModal
        id="createPaymentChannelModal"
        title="新建支付渠道"
        dialogClassName="modal-dialog-centered modal-lg"
        onHidden={() => {
          setCreateName('');
          setCreateEnabled(true);
          setCreateStripeCurrency('cny');
          setCreateStripeSecret('');
          setCreateStripeWebhookSecret('');
          setCreateEPayGateway('');
          setCreateEPayPartnerID('');
          setCreateEPayKey('');
          setErr('');
        }}
      >
        <form
          className="row g-3"
          onSubmit={async (e) => {
            e.preventDefault();
            setErr('');
            setNotice('');
            try {
              const res = await createAdminPaymentChannel({
                type: createType,
                name: createName.trim(),
                enabled: createEnabled,
                stripe_currency: createType === 'stripe' ? createStripeCurrency.trim() : undefined,
                stripe_secret_key: createType === 'stripe' ? createStripeSecret.trim() : undefined,
                stripe_webhook_secret: createType === 'stripe' ? createStripeWebhookSecret.trim() : undefined,
                epay_gateway: createType === 'epay' ? createEPayGateway.trim() : undefined,
                epay_partner_id: createType === 'epay' ? createEPayPartnerID.trim() : undefined,
                epay_key: createType === 'epay' ? createEPayKey.trim() : undefined,
              });
              if (!res.success) throw new Error(res.message || '创建失败');
              setNotice(res.message || '已创建');
              closeModalById('createPaymentChannelModal');
              await refresh();
            } catch (e) {
              setErr(e instanceof Error ? e.message : '创建失败');
            }
          }}
        >
          <div className="col-md-8">
            <label className="form-label fw-medium">渠道名称</label>
            <input className="form-control" value={createName} onChange={(e) => setCreateName(e.target.value)} placeholder="例如：Stripe-主商户" required />
          </div>
          <div className="col-md-4">
            <label className="form-label fw-medium">渠道类型</label>
            <select className="form-select" value={createType} onChange={(e) => setCreateType(e.target.value as PaymentType)}>
              <option value="stripe">Stripe</option>
              <option value="epay">EPay (易支付)</option>
            </select>
          </div>

          <div className="col-12">
            <div className="form-check form-switch">
              <input className="form-check-input" type="checkbox" role="switch" checked={createEnabled} onChange={(e) => setCreateEnabled(e.target.checked)} />
              <label className="form-check-label">立即启用</label>
            </div>
          </div>

          <hr className="my-2" />

          {createType === 'stripe' ? (
            <>
              <div className="col-md-3">
                <label className="form-label small">currency</label>
                <input className="form-control form-control-sm" value={createStripeCurrency} onChange={(e) => setCreateStripeCurrency(e.target.value)} placeholder="cny" />
                <div className="form-text small text-muted">留空将使用默认值（cny）。</div>
              </div>
              <div className="col-md-9">
                <label className="form-label small">Secret Key (sk_...)</label>
                <input className="form-control form-control-sm" value={createStripeSecret} onChange={(e) => setCreateStripeSecret(e.target.value)} type="password" autoComplete="new-password" />
              </div>
              <div className="col-12">
                <label className="form-label small">Webhook Secret (whsec_...)</label>
                <input className="form-control form-control-sm" value={createStripeWebhookSecret} onChange={(e) => setCreateStripeWebhookSecret(e.target.value)} type="password" autoComplete="new-password" />
              </div>
            </>
          ) : (
            <>
              <div className="col-12">
                <label className="form-label small">gateway</label>
                <input className="form-control form-control-sm" value={createEPayGateway} onChange={(e) => setCreateEPayGateway(e.target.value)} placeholder="https://epay.example.com" />
              </div>
              <div className="col-md-6">
                <label className="form-label small">partner_id</label>
                <input className="form-control form-control-sm" value={createEPayPartnerID} onChange={(e) => setCreateEPayPartnerID(e.target.value)} placeholder="10001" />
              </div>
              <div className="col-md-6">
                <label className="form-label small">key</label>
                <input className="form-control form-control-sm" value={createEPayKey} onChange={(e) => setCreateEPayKey(e.target.value)} type="password" autoComplete="new-password" />
              </div>
            </>
          )}

          <div className="modal-footer border-top-0 px-0 pb-0">
            <button type="button" className="btn btn-light" data-bs-dismiss="modal">
              取消
            </button>
            <button className="btn btn-primary px-4" type="submit" disabled={loading}>
              创建
            </button>
          </div>
        </form>
      </BootstrapModal>

      <BootstrapModal
        id="editPaymentChannelModal"
        title={
          editing ? (
            <span>
              编辑支付渠道：<span className="fw-bold">{editing.name}</span> <span className="text-muted">#{editing.id}</span>
            </span>
          ) : (
            '编辑支付渠道'
          )
        }
        dialogClassName="modal-dialog-centered modal-lg"
        onHidden={() => {
          setEditing(null);
          setEditStripeSecret('');
          setEditStripeWebhookSecret('');
          setEditEPayKey('');
          setErr('');
        }}
      >
        {!editing ? (
          <div className="text-muted">未选择支付渠道。</div>
        ) : (
          <form
            className="row g-3"
            onSubmit={async (e) => {
              e.preventDefault();
              if (!editing) return;
              setErr('');
              setNotice('');
              try {
                const res = await updateAdminPaymentChannel(editing.id, {
                  name: editName.trim(),
                  enabled: editEnabled,
                  stripe_currency: editing.type === 'stripe' ? editStripeCurrency.trim() || undefined : undefined,
                  stripe_secret_key: editing.type === 'stripe' && editStripeSecret.trim() ? editStripeSecret.trim() : undefined,
                  stripe_webhook_secret: editing.type === 'stripe' && editStripeWebhookSecret.trim() ? editStripeWebhookSecret.trim() : undefined,
                  epay_gateway: editing.type === 'epay' ? editEPayGateway.trim() || undefined : undefined,
                  epay_partner_id: editing.type === 'epay' ? editEPayPartnerID.trim() || undefined : undefined,
                  epay_key: editing.type === 'epay' && editEPayKey.trim() ? editEPayKey.trim() : undefined,
                });
                if (!res.success) throw new Error(res.message || '保存失败');
                setNotice(res.message || '已保存');
                closeModalById('editPaymentChannelModal');
                await refresh();
              } catch (e) {
                setErr(e instanceof Error ? e.message : '保存失败');
              }
            }}
          >
            <div className="col-md-8">
              <label className="form-label fw-medium">渠道名称</label>
              <input className="form-control" value={editName} onChange={(e) => setEditName(e.target.value)} required />
            </div>
            <div className="col-md-4">
              <label className="form-label fw-medium">渠道类型</label>
              <input className="form-control bg-light" value={editing.type_label} disabled />
            </div>
            <div className="col-12">
              <div className="d-flex align-items-end justify-content-between">
                <div className="form-check form-switch">
                  <input className="form-check-input" type="checkbox" role="switch" checked={editEnabled} onChange={(e) => setEditEnabled(e.target.checked)} />
                  <label className="form-check-label">启用</label>
                </div>
                <div className="text-muted small">
                  Webhook URL：<code className="user-select-all">{editing.webhook_url || '-'}</code>
                </div>
              </div>
            </div>

            <hr className="my-2" />

            {editing.type === 'stripe' ? (
              <>
                <div className="col-md-3">
                  <label className="form-label small">currency</label>
                  <input className="form-control form-control-sm" value={editStripeCurrency} onChange={(e) => setEditStripeCurrency(e.target.value)} placeholder="cny" />
                  <div className="form-text small text-muted">留空将使用默认值（cny）。</div>
                </div>
                <div className="col-md-9">
                  <label className="form-label small">Secret Key (sk_...)</label>
                  <input
                    className="form-control form-control-sm"
                    value={editStripeSecret}
                    onChange={(e) => setEditStripeSecret(e.target.value)}
                    placeholder="留空表示保持不变"
                    type="password"
                    autoComplete="new-password"
                  />
                  <div className="form-text small text-muted">当前：{editing.stripe_secret_key_set ? '已设置' : '未设置'}（密钥不回显）。</div>
                </div>
                <div className="col-12">
                  <label className="form-label small">Webhook Secret (whsec_...)</label>
                  <input
                    className="form-control form-control-sm"
                    value={editStripeWebhookSecret}
                    onChange={(e) => setEditStripeWebhookSecret(e.target.value)}
                    placeholder="留空表示保持不变"
                    type="password"
                    autoComplete="new-password"
                  />
                  <div className="form-text small text-muted">当前：{editing.stripe_webhook_secret_set ? '已设置' : '未设置'}（密钥不回显）。</div>
                </div>
              </>
            ) : (
              <>
                <div className="col-12">
                  <label className="form-label small">gateway</label>
                  <input className="form-control form-control-sm" value={editEPayGateway} onChange={(e) => setEditEPayGateway(e.target.value)} placeholder="https://epay.example.com" />
                </div>
                <div className="col-md-6">
                  <label className="form-label small">partner_id</label>
                  <input className="form-control form-control-sm" value={editEPayPartnerID} onChange={(e) => setEditEPayPartnerID(e.target.value)} placeholder="10001" />
                </div>
                <div className="col-md-6">
                  <label className="form-label small">key</label>
                  <input
                    className="form-control form-control-sm"
                    value={editEPayKey}
                    onChange={(e) => setEditEPayKey(e.target.value)}
                    placeholder="留空表示保持不变"
                    type="password"
                    autoComplete="new-password"
                  />
                  <div className="form-text small text-muted">当前：{editing.epay_key_set ? '已设置' : '未设置'}（密钥不回显）。</div>
                </div>
              </>
            )}

            <div className="modal-footer border-top-0 px-0 pb-0">
              <button type="button" className="btn btn-light" data-bs-dismiss="modal">
                取消
              </button>
              <button className="btn btn-primary px-4" type="submit">
                保存
              </button>
            </div>
          </form>
        )}
      </BootstrapModal>
    </div>
  );
}
