import { useEffect, useMemo, useState } from 'react';
import { Link } from 'react-router-dom';

import { BootstrapModal } from '../../components/BootstrapModal';
import { closeModalById } from '../../components/modal';
import { listAdminChannelGroups, type AdminChannelGroup } from '../../api/admin/channelGroups';
import {
  createAdminSubscriptionPlan,
  deleteAdminSubscriptionPlan,
  listAdminSubscriptionPlans,
  type AdminSubscriptionPlan,
} from '../../api/admin/billing';

function statusBadge(status: number): { cls: string; label: string } {
  if (status === 1) return { cls: 'badge rounded-pill bg-success bg-opacity-10 text-success px-2', label: '启用' };
  return { cls: 'badge rounded-pill bg-secondary bg-opacity-10 text-secondary px-2', label: '禁用' };
}

function limitLabel(v?: string): string {
  const s = (v || '').trim();
  if (!s) return '不限';
  return `$${s}`;
}

export function SubscriptionsPage() {
  const [plans, setPlans] = useState<AdminSubscriptionPlan[]>([]);
  const [groups, setGroups] = useState<AdminChannelGroup[]>([]);
  const [loading, setLoading] = useState(true);
  const [err, setErr] = useState('');
  const [notice, setNotice] = useState('');

  const [name, setName] = useState('');
  const [groupName, setGroupName] = useState('default');
  const [priceMultiplier, setPriceMultiplier] = useState('1');
  const [priceCNY, setPriceCNY] = useState('12');
  const [durationDays, setDurationDays] = useState('30');
  const [status, setStatus] = useState(1);
  const [limit30d, setLimit30d] = useState('');
  const [limit7d, setLimit7d] = useState('');
  const [limit1d, setLimit1d] = useState('');
  const [limit5h, setLimit5h] = useState('');

  const enabledCount = useMemo(() => plans.filter((p) => p.status === 1).length, [plans]);
  const defaultGroupName = useMemo(() => {
    const byDefault = (groups.find((g) => g.is_default)?.name || '').trim();
    if (byDefault) return byDefault;
    const byFirst = (groups[0]?.name || '').trim();
    if (byFirst) return byFirst;
    return 'default';
  }, [groups]);

  async function refresh() {
    setErr('');
    setNotice('');
    setLoading(true);
    try {
      const [plansRes, groupsRes] = await Promise.all([listAdminSubscriptionPlans(), listAdminChannelGroups()]);
      if (!groupsRes.success) throw new Error(groupsRes.message || '加载分组失败');
      const nextGroups = groupsRes.data || [];
      setGroups(nextGroups);
      if (!plansRes.success) throw new Error(plansRes.message || '加载套餐失败');
      setPlans(plansRes.data || []);
      const nextDefault = (nextGroups.find((g) => g.is_default)?.name || '').trim() || (nextGroups[0]?.name || '').trim() || 'default';
      setGroupName((prev) => {
        const cur = (prev || '').trim();
        if (cur && nextGroups.some((g) => g.name === cur)) return cur;
        return nextDefault;
      });
    } catch (e) {
      setErr(e instanceof Error ? e.message : '加载失败');
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    void refresh();
  }, []);

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
                  <span className="fs-4 material-symbols-rounded">workspace_premium</span>
                </div>
                <div>
                  <h5 className="mb-1 fw-semibold">订阅套餐</h5>
                  <p className="mb-0 text-muted small">
                    {enabledCount} 启用 / {plans.length} 总计
                  </p>
                </div>
              </div>

              <div className="d-flex gap-2">
                <button type="button" className="btn btn-primary btn-sm" data-bs-toggle="modal" data-bs-target="#createSubPlanModal">
                  <span className="material-symbols-rounded me-1">add</span> 新增套餐
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
          ) : plans.length === 0 ? (
            <div className="text-center py-5 text-muted">
              <span className="fs-1 d-block mb-3 material-symbols-rounded">inbox</span>
              暂无套餐，请先新增套餐后再允许用户购买。
            </div>
          ) : (
            <div className="card overflow-hidden">
              <div className="table-responsive">
                <table className="table table-hover align-middle mb-0">
                  <thead className="table-light">
                    <tr>
                      <th className="ps-4">名称</th>
                      <th>组</th>
                      <th>倍率</th>
                      <th>价格</th>
                      <th>有效期</th>
                      <th>额度窗口（USD）</th>
                      <th>状态</th>
                      <th className="text-end pe-4">操作</th>
                    </tr>
                  </thead>
                  <tbody>
                    {plans.map((p) => {
                      const st = statusBadge(p.status);
                      return (
                        <tr key={p.id}>
                          <td className="ps-4 fw-bold text-dark">{p.name}</td>
                          <td>
                            <span className="badge bg-light text-secondary border fw-normal">{(p.group_name || '').trim() || defaultGroupName}</span>
                          </td>
                          <td className="text-muted small">
                            <span className="badge bg-light text-dark border fw-normal">×{(p.price_multiplier || '1').trim() || '1'}</span>
                          </td>
                          <td className="fw-bold text-dark">¥{p.price_cny}</td>
                          <td className="text-muted small">{p.duration_days} 天</td>
                          <td className="text-muted small">
                            <div className="d-flex flex-wrap gap-2">
                              <span title="30天限额">
                                <span className="text-muted opacity-50">30d:</span> {limitLabel(p.limit_30d)}
                              </span>
                              <span title="7天限额">
                                <span className="text-muted opacity-50">7d:</span> {limitLabel(p.limit_7d)}
                              </span>
                              <span title="1天限额">
                                <span className="text-muted opacity-50">1d:</span> {limitLabel(p.limit_1d)}
                              </span>
                              <span title="5小时限额">
                                <span className="text-muted opacity-50">5h:</span> {limitLabel(p.limit_5h)}
                              </span>
                            </div>
                          </td>
                          <td>
                            <span className={st.cls}>{st.label}</span>
                          </td>
                          <td className="text-end pe-4 text-nowrap">
                            <div className="d-inline-flex gap-1">
                              <Link to={`/admin/subscriptions/${p.id}`} className="btn btn-sm btn-light border text-primary" title="编辑套餐">
                                <i className="ri-settings-3-line"></i>
                              </Link>
                              <button
                                type="button"
                                className="btn btn-sm btn-light border text-danger"
                                title="删除套餐"
                                onClick={async () => {
                                  if (!window.confirm('确认删除该套餐？将同时删除该套餐下的订阅/订单记录，此操作不可恢复。')) return;
                                  setErr('');
                                  setNotice('');
                                  try {
                                    const res = await deleteAdminSubscriptionPlan(p.id);
                                    if (!res.success) throw new Error(res.message || '删除失败');
                                    setNotice('已删除');
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
          )}
        </div>
      </div>

      <BootstrapModal
        id="createSubPlanModal"
        title="新增套餐"
        dialogClassName="modal-dialog-centered modal-lg"
        onHidden={() => {
          setErr('');
          setName('');
          setGroupName(defaultGroupName);
          setPriceMultiplier('1');
          setPriceCNY('12');
          setDurationDays('30');
          setStatus(1);
          setLimit30d('');
          setLimit7d('');
          setLimit1d('');
          setLimit5h('');
        }}
      >
        <form
          className="row g-3"
          onSubmit={async (e) => {
            e.preventDefault();
            setErr('');
            setNotice('');
            try {
              const res = await createAdminSubscriptionPlan({
                name: name.trim(),
                group_name: groupName,
                price_multiplier: priceMultiplier.trim() || undefined,
                price_cny: priceCNY.trim(),
                duration_days: Number.parseInt(durationDays, 10) || 30,
                status,
                limit_30d: limit30d.trim() || undefined,
                limit_7d: limit7d.trim() || undefined,
                limit_1d: limit1d.trim() || undefined,
                limit_5h: limit5h.trim() || undefined,
              });
              if (!res.success) throw new Error(res.message || '创建失败');
              setNotice('已创建');
              closeModalById('createSubPlanModal');
              await refresh();
            } catch (e) {
              setErr(e instanceof Error ? e.message : '创建失败');
            }
          }}
        >
          <div className="col-md-6">
            <label className="form-label">名称</label>
            <input className="form-control" value={name} onChange={(e) => setName(e.target.value)} placeholder="例如：基础订阅" required />
          </div>
          <div className="col-md-6">
            <label className="form-label">状态</label>
            <select className="form-select" value={status} onChange={(e) => setStatus(Number.parseInt(e.target.value, 10) || 0)}>
              <option value={1}>启用</option>
              <option value={0}>禁用</option>
            </select>
          </div>

          <div className="col-md-4">
            <label className="form-label">组</label>
            <select className="form-select" value={groupName} onChange={(e) => setGroupName(e.target.value)}>
              {groups.map((g) => (
                <option key={g.id} value={g.name} disabled={g.status !== 1}>
                  {g.name}
                  {g.status !== 1 ? '（已禁用）' : ''}
                </option>
              ))}
            </select>
          </div>
          <div className="col-md-4">
            <label className="form-label">订阅倍率</label>
            <div className="input-group">
              <span className="input-group-text">×</span>
              <input
                className="form-control"
                value={priceMultiplier}
                onChange={(e) => setPriceMultiplier(e.target.value)}
                inputMode="decimal"
                placeholder="1"
              />
            </div>
            <div className="form-text small text-muted">最终计费倍率 = 订阅倍率 × 最终成功分组倍率。</div>
          </div>
          <div className="col-md-4">
            <label className="form-label">价格（CNY）</label>
            <div className="input-group">
              <span className="input-group-text">¥</span>
              <input className="form-control" value={priceCNY} onChange={(e) => setPriceCNY(e.target.value)} inputMode="decimal" placeholder="12.00" required />
            </div>
          </div>

          <div className="col-md-6">
            <label className="form-label">有效期（天）</label>
            <input className="form-control" value={durationDays} onChange={(e) => setDurationDays(e.target.value)} inputMode="numeric" placeholder="30" required />
          </div>

          <div className="col-md-3">
            <label className="form-label">30d 限额（USD）</label>
            <div className="input-group">
              <span className="input-group-text">$</span>
              <input className="form-control" value={limit30d} onChange={(e) => setLimit30d(e.target.value)} inputMode="decimal" placeholder="留空=不限" />
            </div>
          </div>
          <div className="col-md-3">
            <label className="form-label">7d 限额（USD）</label>
            <div className="input-group">
              <span className="input-group-text">$</span>
              <input className="form-control" value={limit7d} onChange={(e) => setLimit7d(e.target.value)} inputMode="decimal" placeholder="留空=不限" />
            </div>
          </div>
          <div className="col-md-3">
            <label className="form-label">1d 限额（USD）</label>
            <div className="input-group">
              <span className="input-group-text">$</span>
              <input className="form-control" value={limit1d} onChange={(e) => setLimit1d(e.target.value)} inputMode="decimal" placeholder="留空=不限" />
            </div>
          </div>
          <div className="col-md-3">
            <label className="form-label">5h 限额（USD）</label>
            <div className="input-group">
              <span className="input-group-text">$</span>
              <input className="form-control" value={limit5h} onChange={(e) => setLimit5h(e.target.value)} inputMode="decimal" placeholder="留空=不限" />
            </div>
          </div>

          <div className="col-12">
            <div className="form-text small text-muted">额度单位为 USD（最多 6 位小数）。额度按模型定价估算成本后扣减。</div>
          </div>

          <div className="modal-footer border-top-0 px-0 pb-0">
            <button type="button" className="btn btn-light" data-bs-dismiss="modal">
              取消
            </button>
            <button className="btn btn-primary px-4" type="submit" disabled={loading}>
              保存
            </button>
          </div>
        </form>
      </BootstrapModal>
    </div>
  );
}
