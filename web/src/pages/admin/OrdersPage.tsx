import { useEffect, useMemo, useState } from 'react';

import { approveAdminSubscriptionOrder, listAdminSubscriptionOrders, rejectAdminSubscriptionOrder, type AdminSubscriptionOrder } from '../../api/admin/billing';

function orderStatusBadge(status: number): string {
  if (status === 1) return 'badge rounded-pill bg-success bg-opacity-10 text-success px-2';
  if (status === 0) return 'badge rounded-pill bg-warning bg-opacity-10 text-warning px-2';
  return 'badge rounded-pill bg-secondary bg-opacity-10 text-secondary px-2';
}

export function OrdersPage() {
  const [items, setItems] = useState<AdminSubscriptionOrder[]>([]);
  const [loading, setLoading] = useState(true);
  const [err, setErr] = useState('');
  const [notice, setNotice] = useState('');

  const pendingCount = useMemo(() => items.filter((o) => o.status === 0).length, [items]);

  async function refresh() {
    setErr('');
    setNotice('');
    setLoading(true);
    try {
      const res = await listAdminSubscriptionOrders();
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
                  <span className="fs-4 material-symbols-rounded">receipt_long</span>
                </div>
                <div>
                  <h5 className="mb-1 fw-semibold">订单</h5>
                  <p className="mb-0 text-muted small">
                    {pendingCount} 待处理 / {items.length} 总计
                  </p>
                </div>
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
              暂无订单。
            </div>
          ) : (
            <div className="card overflow-hidden mb-0">
              <div className="table-responsive">
                <table className="table table-hover align-middle mb-0">
                  <thead className="table-light">
                    <tr>
                      <th className="ps-4">订单</th>
                      <th>套餐/组</th>
                      <th>用户</th>
                      <th>金额</th>
                      <th>状态</th>
                      <th>时间</th>
                      <th className="text-end pe-4">操作</th>
                    </tr>
                  </thead>
                  <tbody>
                    {items.map((o) => (
                      <tr key={o.id} style={o.status === 2 ? { opacity: 0.6 } : undefined}>
                        <td className="ps-4">
                          <div className="font-monospace text-muted small">#{o.id}</div>
                        </td>
                        <td>
                          <div className="fw-bold text-dark">{o.plan_name}</div>
                          <div>
                            <span className="badge bg-light text-secondary border fw-normal smaller">
                              {o.group_name || 'default'}
                            </span>
                          </div>
                        </td>
                        <td className="text-muted small">{o.user_email}</td>
                        <td className="fw-bold text-dark">¥{o.amount_cny}</td>
                        <td>
                          <span className={orderStatusBadge(o.status)}>{o.status_text}</span>
                        </td>
                        <td className="text-muted small" style={{ lineHeight: 1.2 }}>
                          <div title="创建时间">{o.created_at}</div>
                          {o.paid_at ? (
                            <div className="text-success" title="支付时间">
                              <i className="ri-bank-card-line me-1"></i>
                              {o.paid_at}
                            </div>
                          ) : null}
                          {o.approved_at ? (
                            <div className="text-primary" title="批准时间">
                              <i className="ri-check-double-line me-1"></i>
                              {o.approved_at}
                            </div>
                          ) : null}
                        </td>
                        <td className="text-end pe-4 text-nowrap">
                          <div className="d-inline-flex gap-1">
                            <button
                              type="button"
                              className="btn btn-sm btn-light border text-success"
                              title="批准生效"
                              disabled={o.status === 2}
                              onClick={async () => {
                                if (!window.confirm('确认批准并生效？该订单将被处理。')) return;
                                setErr('');
                                setNotice('');
                                try {
                                  const res = await approveAdminSubscriptionOrder(o.id);
                                  if (!res.success) throw new Error(res.message || '批准失败');
                                  setNotice(res.message || '订单已批准并生效');
                                  await refresh();
                                } catch (e) {
                                  setErr(e instanceof Error ? e.message : '批准失败');
                                }
                              }}
                            >
                              <i className="ri-checkbox-circle-line"></i>
                            </button>
                            <button
                              type="button"
                              className="btn btn-sm btn-light border text-danger"
                              title="不批准"
                              disabled={o.status === 2}
                              onClick={async () => {
                                if (!window.confirm('确认不批准？订单将不会生效。')) return;
                                setErr('');
                                setNotice('');
                                try {
                                  const res = await rejectAdminSubscriptionOrder(o.id);
                                  if (!res.success) throw new Error(res.message || '拒绝失败');
                                  setNotice(res.message || '订单已拒绝');
                                  await refresh();
                                } catch (e) {
                                  setErr(e instanceof Error ? e.message : '拒绝失败');
                                }
                              }}
                            >
                              <i className="ri-close-circle-line"></i>
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
        </div>
      </div>
    </div>
  );
}
