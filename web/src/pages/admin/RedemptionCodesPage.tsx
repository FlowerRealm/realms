import { useCallback, useEffect, useMemo, useState } from 'react';

import { listAdminSubscriptionPlans, type AdminSubscriptionPlan } from '../../api/admin/billing';
import {
  createAdminRedemptionCodes,
  disableAdminRedemptionCode,
  exportAdminRedemptionCodes,
  listAdminRedemptionCodes,
  updateAdminRedemptionCode,
  type AdminRedemptionCode,
  type AdminRedemptionDistributionMode,
  type AdminRedemptionRewardType,
} from '../../api/admin/redemptionCodes';
import { BootstrapModal } from '../../components/BootstrapModal';
import { DividedStack } from '../../components/DividedStack';
import { SegmentedFrame } from '../../components/SegmentedFrame';
import { closeModalById, showModalById } from '../../components/modal';

function triggerBlobDownload(blob: Blob, fileName: string) {
  const url = URL.createObjectURL(blob);
  const anchor = document.createElement('a');
  anchor.href = url;
  anchor.download = fileName;
  anchor.style.display = 'none';
  document.body.appendChild(anchor);
  anchor.click();
  document.body.removeChild(anchor);
  window.setTimeout(() => URL.revokeObjectURL(url), 1000);
}

function statusBadge(item: AdminRedemptionCode) {
  if (item.status !== 1) return 'badge rounded-pill bg-secondary bg-opacity-10 text-secondary px-2';
  if (item.redeemed_count >= item.max_redemptions) return 'badge rounded-pill bg-warning bg-opacity-10 text-warning px-2';
  return 'badge rounded-pill bg-success bg-opacity-10 text-success px-2';
}

function distributionLabel(mode: AdminRedemptionDistributionMode) {
  return mode === 'shared' ? '共享码' : '单码';
}

function rewardLabel(item: AdminRedemptionCode) {
  if (item.reward_type === 'subscription') return item.plan_name || '套餐';
  return item.balance_usd ? `$${item.balance_usd}` : '-';
}

function toDateTimeLocal(value?: string) {
  if (!value) return '';
  return value.replace(' ', 'T').slice(0, 16);
}

function initialCreateForm(plans: AdminSubscriptionPlan[]) {
  return {
    batch_name: '',
    distribution_mode: 'single' as AdminRedemptionDistributionMode,
    reward_type: 'subscription' as AdminRedemptionRewardType,
    subscription_plan_id: plans[0]?.id ? String(plans[0].id) : '',
    balance_usd: '',
    generate_count: '1',
    max_redemptions: '1',
    manual_codes: '',
    expires_at: '',
    status: '1',
  };
}

type CreateFormState = ReturnType<typeof initialCreateForm>;

type EditFormState = {
  id: number;
  code: string;
  max_redemptions: string;
  expires_at: string;
  status: string;
} | null;

export function RedemptionCodesPage() {
  const [items, setItems] = useState<AdminRedemptionCode[]>([]);
  const [plans, setPlans] = useState<AdminSubscriptionPlan[]>([]);
  const [loading, setLoading] = useState(true);
  const [submitting, setSubmitting] = useState(false);
  const [err, setErr] = useState('');
  const [notice, setNotice] = useState('');
  const [createdCodes, setCreatedCodes] = useState<string[]>([]);
  const [createForm, setCreateForm] = useState<CreateFormState>(() => initialCreateForm([]));
  const [editForm, setEditForm] = useState<EditFormState>(null);

  const summary = useMemo(() => {
    const enabled = items.filter((item) => item.status === 1).length;
    const shared = items.filter((item) => item.distribution_mode === 'shared').length;
    return { enabled, shared };
  }, [items]);

  const refresh = useCallback(async (nextNotice?: string) => {
    setErr('');
    setLoading(true);
    try {
      const [codesRes, plansRes] = await Promise.all([listAdminRedemptionCodes(), listAdminSubscriptionPlans()]);
      if (!codesRes.success) throw new Error(codesRes.message || '加载兑换码失败');
      if (!plansRes.success) throw new Error(plansRes.message || '加载套餐失败');

      const nextPlans = plansRes.data || [];
      setPlans(nextPlans);
      setItems(codesRes.data || []);
      setCreateForm((prev) => {
        if (prev.subscription_plan_id || nextPlans.length === 0) return prev;
        return { ...prev, subscription_plan_id: String(nextPlans[0].id) };
      });
      if (nextNotice) setNotice(nextNotice);
    } catch (error) {
      setErr(error instanceof Error ? error.message : '加载失败');
      setItems([]);
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void refresh();
  }, [refresh]);

  const manualCodes = useMemo(
    () =>
      createForm.manual_codes
        .split(/\r?\n/)
        .map((item) => item.trim().toUpperCase())
        .filter(Boolean),
    [createForm.manual_codes],
  );

  return (
    <div className="fade-in-up">
      <SegmentedFrame>
        <DividedStack>
          <div className="card mb-0">
            <div className="card-body d-flex flex-column flex-md-row justify-content-between align-items-center">
              <div className="d-flex align-items-center mb-3 mb-md-0">
                <div
                  className="bg-primary bg-opacity-10 text-primary rounded-circle d-flex align-items-center justify-content-center me-3"
                  style={{ width: 48, height: 48 }}
                >
                  <span className="fs-4 material-symbols-rounded">redeem</span>
                </div>
                <div>
                  <h5 className="mb-1 fw-semibold">兑换码管理</h5>
                  <p className="mb-0 text-muted small">
                    {summary.enabled} 启用 / {items.length} 总计 · {summary.shared} 个共享码
                  </p>
                </div>
              </div>

              <div className="d-flex gap-2">
                <button
                  type="button"
                  className="btn btn-light btn-sm border"
                  onClick={async () => {
                    setErr('');
                    setNotice('');
                    try {
                      const exported = await exportAdminRedemptionCodes();
                      triggerBlobDownload(exported.blob, exported.fileName);
                      setNotice('导出已开始');
                    } catch (error) {
                      setErr(error instanceof Error ? error.message : '导出失败');
                    }
                  }}
                >
                  <i className="ri-download-2-line me-1"></i> 导出
                </button>
                <button type="button" className="btn btn-primary btn-sm" data-bs-toggle="modal" data-bs-target="#createRedemptionCodeModal">
                  <span className="material-symbols-rounded me-1">add</span> 新建兑换码
                </button>
              </div>
            </div>
          </div>

          {notice ? (
            <div className="alert alert-success d-flex align-items-center mb-0" role="alert">
              <span className="me-2 material-symbols-rounded">check_circle</span>
              <div>{notice}</div>
            </div>
          ) : null}

          {err ? (
            <div className="alert alert-danger d-flex align-items-center mb-0" role="alert">
              <span className="me-2 material-symbols-rounded">warning</span>
              <div>{err}</div>
            </div>
          ) : null}

          {loading ? (
            <div className="text-muted">加载中…</div>
          ) : items.length === 0 ? (
            <div className="text-center py-5 text-muted">
              <span className="fs-1 d-block mb-3 material-symbols-rounded">inbox</span>
              暂无兑换码，先创建一批再发给用户。
            </div>
          ) : (
            <div className="card overflow-hidden mb-0">
              <div className="table-responsive">
                <table className="table table-hover align-middle mb-0">
                  <thead className="table-light">
                    <tr>
                      <th className="ps-4">兑换码</th>
                      <th>批次</th>
                      <th>发码模式</th>
                      <th>奖励</th>
                      <th>使用情况</th>
                      <th>状态</th>
                      <th>过期时间</th>
                      <th className="text-end pe-4">操作</th>
                    </tr>
                  </thead>
                  <tbody>
                    {items.map((item) => (
                      <tr key={item.id}>
                        <td className="ps-4">
                          <div className="fw-bold font-monospace">{item.code}</div>
                        </td>
                        <td className="text-muted small">{item.batch_name || '-'}</td>
                        <td>
                          <span className="badge bg-light text-dark border">{distributionLabel(item.distribution_mode)}</span>
                        </td>
                        <td className="text-muted small">{rewardLabel(item)}</td>
                        <td className="text-muted small">
                          {item.redeemed_count} / {item.max_redemptions}
                        </td>
                        <td>
                          <span className={statusBadge(item)}>{item.status === 1 ? '启用' : '停用'}</span>
                        </td>
                        <td className="text-muted small">{item.expires_at || '-'}</td>
                        <td className="text-end pe-4 text-nowrap">
                          <div className="d-inline-flex gap-1">
                            {item.distribution_mode === 'shared' ? (
                              <button
                                type="button"
                                className="btn btn-sm btn-light border text-primary"
                                data-bs-toggle="modal"
                                data-bs-target="#editRedemptionCodeModal"
                                onClick={() =>
                                  setEditForm({
                                    id: item.id,
                                    code: item.code,
                                    max_redemptions: String(item.max_redemptions),
                                    expires_at: toDateTimeLocal(item.expires_at),
                                    status: String(item.status),
                                  })
                                }
                                title="编辑共享码"
                              >
                                <i className="ri-settings-3-line"></i>
                              </button>
                            ) : null}
                            <button
                              type="button"
                              className="btn btn-sm btn-light border text-danger"
                              title="停用兑换码"
                              onClick={async () => {
                                if (!window.confirm('确认停用该兑换码？停用后用户将无法继续兑换。')) return;
                                setErr('');
                                setNotice('');
                                try {
                                  const res = await disableAdminRedemptionCode(item.id);
                                  if (!res.success) throw new Error(res.message || '停用失败');
                                  await refresh(res.message || '已停用');
                                } catch (error) {
                                  setErr(error instanceof Error ? error.message : '停用失败');
                                }
                              }}
                            >
                              <i className="ri-stop-circle-line"></i>
                            </button>
                          </div>
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            </div>
          )}
        </DividedStack>
      </SegmentedFrame>

      <BootstrapModal
        id="createRedemptionCodeModal"
        title="新建兑换码"
        dialogClassName="modal-dialog-centered modal-lg"
        onHidden={() => {
          setCreateForm(initialCreateForm(plans));
        }}
      >
        <form
          className="row g-3"
          onSubmit={async (event) => {
            event.preventDefault();
            setErr('');
            setNotice('');
            setCreatedCodes([]);
            setSubmitting(true);
            try {
              const res = await createAdminRedemptionCodes({
                batch_name: createForm.batch_name,
                codes: manualCodes.length > 0 ? manualCodes : undefined,
                count: manualCodes.length > 0 ? undefined : Number.parseInt(createForm.generate_count, 10) || 1,
                distribution_mode: createForm.distribution_mode,
                reward_type: createForm.reward_type,
                subscription_plan_id:
                  createForm.reward_type === 'subscription'
                    ? Number.parseInt(createForm.subscription_plan_id, 10) || undefined
                    : undefined,
                balance_usd: createForm.reward_type === 'balance' ? createForm.balance_usd : undefined,
                max_redemptions: createForm.distribution_mode === 'shared' ? Number.parseInt(createForm.max_redemptions, 10) || 1 : 1,
                expires_at: createForm.expires_at || undefined,
                status: Number.parseInt(createForm.status, 10) || 1,
              });
              if (!res.success) throw new Error(res.message || '创建失败');
              const nextCodes = res.data?.codes || [];
              setCreatedCodes(nextCodes);
              closeModalById('createRedemptionCodeModal');
              await refresh(res.message || '已创建');
              if (nextCodes.length > 0) {
                showModalById('createdRedemptionCodesModal');
              }
            } catch (error) {
              setErr(error instanceof Error ? error.message : '创建失败');
            } finally {
              setSubmitting(false);
            }
          }}
        >
          <div className="col-md-6">
            <label className="form-label">批次名</label>
            <input
              className="form-control"
              value={createForm.batch_name}
              onChange={(event) => setCreateForm((prev) => ({ ...prev, batch_name: event.target.value }))}
              placeholder="例如：春季活动 / 人工补偿 / KOL 合作"
            />
          </div>
          <div className="col-md-3">
            <label className="form-label">发码模式</label>
            <select
              className="form-select"
              value={createForm.distribution_mode}
              onChange={(event) =>
                setCreateForm((prev) => ({ ...prev, distribution_mode: event.target.value as AdminRedemptionDistributionMode }))
              }
            >
              <option value="single">单码</option>
              <option value="shared">共享码</option>
            </select>
          </div>
          <div className="col-md-3">
            <label className="form-label">状态</label>
            <select
              className="form-select"
              value={createForm.status}
              onChange={(event) => setCreateForm((prev) => ({ ...prev, status: event.target.value }))}
            >
              <option value="1">启用</option>
              <option value="0">停用</option>
            </select>
          </div>

          <div className="col-md-4">
            <label className="form-label">奖励类型</label>
            <select
              className="form-select"
              value={createForm.reward_type}
              onChange={(event) =>
                setCreateForm((prev) => ({ ...prev, reward_type: event.target.value as AdminRedemptionRewardType }))
              }
            >
              <option value="subscription">套餐</option>
              <option value="balance">余额</option>
            </select>
          </div>
          {createForm.reward_type === 'subscription' ? (
            <div className="col-md-8">
              <label className="form-label">订阅套餐</label>
              <select
                className="form-select"
                value={createForm.subscription_plan_id}
                onChange={(event) => setCreateForm((prev) => ({ ...prev, subscription_plan_id: event.target.value }))}
              >
                {plans.map((plan) => (
                  <option key={plan.id} value={plan.id}>
                    {plan.name}
                  </option>
                ))}
              </select>
            </div>
          ) : (
            <div className="col-md-8">
              <label className="form-label">入账余额（USD）</label>
              <input
                className="form-control"
                value={createForm.balance_usd}
                onChange={(event) => setCreateForm((prev) => ({ ...prev, balance_usd: event.target.value }))}
                placeholder="例如：5 或 12.500000"
              />
            </div>
          )}

          <div className="col-md-4">
            <label className="form-label">自动生成数量</label>
            <input
              className="form-control"
              value={createForm.generate_count}
              onChange={(event) => setCreateForm((prev) => ({ ...prev, generate_count: event.target.value }))}
              placeholder="1"
            />
            <div className="form-text small text-muted">
              仅在未手填兑换码时生效。单码模式可一次自动生成多枚兑换码。
            </div>
          </div>
          <div className="col-md-8">
            <label className="form-label">过期时间</label>
            <input
              className="form-control"
              type="datetime-local"
              value={createForm.expires_at}
              onChange={(event) => setCreateForm((prev) => ({ ...prev, expires_at: event.target.value }))}
            />
          </div>
          {createForm.distribution_mode === 'shared' ? (
            <div className="col-md-4">
              <label className="form-label">共享码可兑换次数</label>
              <input
                className="form-control"
                value={createForm.max_redemptions}
                onChange={(event) => setCreateForm((prev) => ({ ...prev, max_redemptions: event.target.value }))}
                placeholder="1"
              />
            </div>
          ) : null}

          <div className="col-12">
            <label className="form-label">手动指定兑换码（每行一个，可选）</label>
            <textarea
              className="form-control font-monospace"
              rows={6}
              value={createForm.manual_codes}
              onChange={(event) => setCreateForm((prev) => ({ ...prev, manual_codes: event.target.value.toUpperCase() }))}
              placeholder="留空则按上方数量自动生成。&#10;SAMPLE-001&#10;SAMPLE-002"
            />
            <div className="form-text small text-muted">
              单码模式可一次创建多个不同兑换码；共享码模式通常只填一行，或直接留空自动生成一个共享码。
            </div>
          </div>

          <div className="modal-footer border-top-0 px-0 pb-0">
            <button type="button" className="btn btn-light" data-bs-dismiss="modal" disabled={submitting}>
              取消
            </button>
            <button className="btn btn-primary px-4" type="submit" disabled={submitting}>
              {submitting ? '提交中…' : '创建'}
            </button>
          </div>
        </form>
      </BootstrapModal>

      <BootstrapModal
        id="editRedemptionCodeModal"
        title="编辑共享码"
        dialogClassName="modal-dialog-centered"
        onHidden={() => {
          setEditForm(null);
        }}
      >
        <form
          className="row g-3"
          onSubmit={async (event) => {
            event.preventDefault();
            if (!editForm) return;
            setErr('');
            setNotice('');
            setSubmitting(true);
            try {
              const res = await updateAdminRedemptionCode(editForm.id, {
                max_redemptions: Number.parseInt(editForm.max_redemptions, 10) || 1,
                expires_at: editForm.expires_at || undefined,
                status: Number.parseInt(editForm.status, 10) || 0,
              });
              if (!res.success) throw new Error(res.message || '保存失败');
              closeModalById('editRedemptionCodeModal');
              await refresh(res.message || '已保存');
            } catch (error) {
              setErr(error instanceof Error ? error.message : '保存失败');
            } finally {
              setSubmitting(false);
            }
          }}
        >
          <div className="col-12">
            <label className="form-label">兑换码</label>
            <input className="form-control font-monospace" value={editForm?.code || ''} disabled />
          </div>
          <div className="col-md-6">
            <label className="form-label">可兑换次数</label>
            <input
              className="form-control"
              value={editForm?.max_redemptions || ''}
              onChange={(event) => setEditForm((prev) => (prev ? { ...prev, max_redemptions: event.target.value } : prev))}
            />
          </div>
          <div className="col-md-6">
            <label className="form-label">状态</label>
            <select
              className="form-select"
              value={editForm?.status || '1'}
              onChange={(event) => setEditForm((prev) => (prev ? { ...prev, status: event.target.value } : prev))}
            >
              <option value="1">启用</option>
              <option value="0">停用</option>
            </select>
          </div>
          <div className="col-12">
            <label className="form-label">过期时间</label>
            <input
              className="form-control"
              type="datetime-local"
              value={editForm?.expires_at || ''}
              onChange={(event) => setEditForm((prev) => (prev ? { ...prev, expires_at: event.target.value } : prev))}
            />
          </div>
          <div className="modal-footer border-top-0 px-0 pb-0">
            <button type="button" className="btn btn-light" data-bs-dismiss="modal" disabled={submitting}>
              取消
            </button>
            <button className="btn btn-primary px-4" type="submit" disabled={submitting || !editForm}>
              {submitting ? '保存中…' : '保存'}
            </button>
          </div>
        </form>
      </BootstrapModal>

      <BootstrapModal id="createdRedemptionCodesModal" title="已创建兑换码" dialogClassName="modal-dialog-centered">
        <div className="d-flex flex-column gap-3">
          <div className="text-muted small">以下是本次创建成功的兑换码，请按需分发：</div>
          <textarea className="form-control font-monospace" rows={10} value={createdCodes.join('\n')} readOnly />
          <div className="modal-footer border-top-0 px-0 pb-0">
            <button type="button" className="btn btn-primary px-4" data-bs-dismiss="modal">
              关闭
            </button>
          </div>
        </div>
      </BootstrapModal>
    </div>
  );
}
