import { useEffect, useState } from 'react';
import { useParams } from 'react-router-dom';

import { listAdminChannelGroups, type AdminChannelGroup } from '../../api/admin/channelGroups';
import {
  deleteAdminSubscriptionPlan,
  getAdminSubscriptionPlan,
  updateAdminSubscriptionPlan,
  type AdminSubscriptionPlan,
} from '../../api/admin/billing';

export function SubscriptionEditPage() {
  const params = useParams();
  const planId = Number.parseInt((params.id || '').toString(), 10);

  const [plan, setPlan] = useState<AdminSubscriptionPlan | null>(null);
  const [groups, setGroups] = useState<AdminChannelGroup[]>([]);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [err, setErr] = useState('');
  const [notice, setNotice] = useState('');

  const [name, setName] = useState('');
  const [groupName, setGroupName] = useState('default');
  const [priceMultiplier, setPriceMultiplier] = useState('1');
  const [priceCNY, setPriceCNY] = useState('');
  const [durationDays, setDurationDays] = useState('30');
  const [status, setStatus] = useState(1);
  const [limit30d, setLimit30d] = useState('');
  const [limit7d, setLimit7d] = useState('');
  const [limit1d, setLimit1d] = useState('');
  const [limit5h, setLimit5h] = useState('');

  async function refresh() {
    setErr('');
    setNotice('');
    setLoading(true);
    try {
      if (!Number.isFinite(planId) || planId <= 0) throw new Error('参数错误');
      const [planRes, groupsRes] = await Promise.all([getAdminSubscriptionPlan(planId), listAdminChannelGroups()]);
      if (!groupsRes.success) throw new Error(groupsRes.message || '加载分组失败');
      setGroups(groupsRes.data || []);
      if (!planRes.success) throw new Error(planRes.message || '加载失败');
      const p = planRes.data || null;
      setPlan(p);
      if (p) {
        setName(p.name || '');
        setGroupName(p.group_name || 'default');
        setPriceMultiplier(p.price_multiplier || '1');
        setPriceCNY(p.price_cny || '');
        setDurationDays(String(p.duration_days || 30));
        setStatus(p.status || 0);
        setLimit30d(p.limit_30d || '');
        setLimit7d(p.limit_7d || '');
        setLimit1d(p.limit_1d || '');
        setLimit5h(p.limit_5h || '');
      }
    } catch (e) {
      setErr(e instanceof Error ? e.message : '加载失败');
      setPlan(null);
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    void refresh();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [planId]);

  return (
    <div className="fade-in-up">
      <div className="d-flex justify-content-between align-items-center mb-4">
        <div>
          <h3 className="mb-0 fw-bold">编辑套餐</h3>
          {plan ? <div className="text-muted small mt-1">#{plan.id} · code={plan.code}</div> : null}
        </div>
      </div>

      {notice ? (
        <div className="alert alert-success d-flex align-items-center" role="alert">
          <span className="me-2 material-symbols-rounded">check_circle</span>
          <div>{notice}</div>
        </div>
      ) : null}

      {err ? (
        <div className="alert alert-danger d-flex align-items-center" role="alert">
          <span className="me-2 material-symbols-rounded">warning</span>
          <div>{err}</div>
        </div>
      ) : null}

      {loading ? (
        <div className="text-muted">加载中…</div>
      ) : !plan ? (
        <div className="alert alert-warning">未找到该套餐。</div>
      ) : (
        <div className="card border-0">
          <div className="card-body p-4">
            <form
              onSubmit={async (e) => {
                e.preventDefault();
                setErr('');
                setNotice('');
                setSaving(true);
                try {
                  const res = await updateAdminSubscriptionPlan(plan.id, {
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
                  if (!res.success) throw new Error(res.message || '保存失败');
                  setNotice('已保存');
                  await refresh();
                } catch (e) {
                  setErr(e instanceof Error ? e.message : '保存失败');
                } finally {
                  setSaving(false);
                }
              }}
            >
              <div className="row g-3">
                <div className="col-md-6">
                  <label className="form-label">名称</label>
                  <input className="form-control" value={name} onChange={(e) => setName(e.target.value)} />
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
                    <input className="form-control" value={priceMultiplier} onChange={(e) => setPriceMultiplier(e.target.value)} inputMode="decimal" placeholder="1" />
                  </div>
                  <div className="form-text small text-muted">最终计费倍率 = 订阅倍率 × 最终成功分组倍率。</div>
                </div>
                <div className="col-md-4">
                  <label className="form-label">价格（CNY）</label>
                  <div className="input-group">
                    <span className="input-group-text">¥</span>
                    <input className="form-control" value={priceCNY} onChange={(e) => setPriceCNY(e.target.value)} inputMode="decimal" />
                  </div>
                </div>

                <div className="col-md-6">
                  <label className="form-label">有效期（天）</label>
                  <input className="form-control" value={durationDays} onChange={(e) => setDurationDays(e.target.value)} inputMode="numeric" />
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

                <div className="col-12 d-grid d-md-flex justify-content-md-between gap-2">
                  <button
                    type="button"
                    className="btn btn-outline-danger"
                    onClick={async () => {
                      if (!window.confirm('确认删除该套餐？将同时删除该套餐下的订阅/订单记录，此操作不可恢复。')) return;
                      setErr('');
                      setNotice('');
                      try {
                        const res = await deleteAdminSubscriptionPlan(plan.id);
                        if (!res.success) throw new Error(res.message || '删除失败');
                        window.location.href = '/admin/subscriptions';
                      } catch (e) {
                        setErr(e instanceof Error ? e.message : '删除失败');
                      }
                    }}
                  >
                    删除
                  </button>
                  <button className="btn btn-primary" type="submit" disabled={saving}>
                    {saving ? '保存中…' : '保存'}
                  </button>
                </div>
              </div>
            </form>
          </div>
        </div>
      )}
    </div>
  );
}
