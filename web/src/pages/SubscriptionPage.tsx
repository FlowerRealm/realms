import { useEffect, useMemo, useState } from 'react';
import { Link, useNavigate } from 'react-router-dom';

import { getSubscriptionPage, purchaseSubscription, type BillingPlanView, type BillingSubscriptionPageResponse } from '../api/billing';

function subscriptionBadge(statusText: string): string {
  if (statusText === '已生效') return 'badge bg-success bg-opacity-10 text-success';
  if (statusText === '待支付') return 'badge bg-warning bg-opacity-10 text-warning';
  if (statusText === '已取消') return 'badge bg-secondary bg-opacity-10 text-secondary';
  return 'badge bg-secondary bg-opacity-10 text-secondary';
}

function percentBarClass(percent: number): string {
  if (percent > 90) return 'bg-danger';
  if (percent > 70) return 'bg-warning';
  return 'bg-primary';
}

export function SubscriptionPage() {
  const navigate = useNavigate();

  const [data, setData] = useState<BillingSubscriptionPageResponse | null>(null);
  const [loading, setLoading] = useState(true);
  const [err, setErr] = useState('');
  const [notice, setNotice] = useState('');

  useEffect(() => {
    let mounted = true;
    (async () => {
      setErr('');
      setLoading(true);
      try {
        const res = await getSubscriptionPage();
        if (!res.success) throw new Error(res.message || '加载失败');
        if (mounted) setData(res.data || null);
      } catch (e) {
        if (mounted) {
          setErr(e instanceof Error ? e.message : '加载失败');
          setData(null);
        }
      } finally {
        if (mounted) setLoading(false);
      }
    })();
    return () => {
      mounted = false;
    };
  }, []);

  const orders = data?.subscription_orders || [];
  const subscriptions = data?.subscriptions || [];
  const plans = data?.plans || [];

  const hasActive = useMemo(() => !!data?.subscription, [data?.subscription]);

  async function buy(plan: BillingPlanView) {
    setErr('');
    setNotice('');
    try {
      const res = await purchaseSubscription(plan.id);
      if (!res.success) throw new Error(res.message || '下单失败');
      const orderId = res.data?.order_id;
      if (!orderId) throw new Error('下单失败：缺少 order_id');
      navigate(`/pay/subscription/${orderId}`);
    } catch (e) {
      setErr(e instanceof Error ? e.message : '下单失败');
    }
  }

  return (
    <div className="fade-in-up">
      <div className="row g-4">
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

        <div className="col-12 mt-4">
          <div className="d-flex align-items-center mb-3">
            <h4 className="mb-0 fw-bold">我的订单</h4>
          </div>

          <div className="card border-0 overflow-hidden">
            {loading ? (
              <div className="card-body p-4 text-muted">加载中…</div>
            ) : orders.length ? (
              <div className="table-responsive">
                <table className="table table-hover align-middle mb-0">
                  <thead className="table-light">
                    <tr>
                      <th>订单号</th>
                      <th>套餐</th>
                      <th>金额</th>
                      <th>状态</th>
                      <th>创建时间</th>
                      <th>支付时间</th>
                      <th>批准时间</th>
                    </tr>
                  </thead>
                  <tbody>
                    {orders.map((o) => (
                      <tr
                        key={o.id}
                        style={{ cursor: 'pointer' }}
                        onClick={(e) => {
                          const target = e.target as HTMLElement | null;
                          if (target?.closest('a, button, input, select, textarea, label, form')) return;
                          navigate(`/pay/subscription/${o.id}`);
                        }}
                      >
                        <td className="font-monospace">
                          <Link className="link-primary text-decoration-none" to={`/pay/subscription/${o.id}`}>
                            #{o.id}
                          </Link>
                        </td>
                        <td>{o.plan_name}</td>
                        <td className="fw-semibold">{o.amount_cny}</td>
                        <td>
                          <span className={subscriptionBadge(o.status)}>{o.status}</span>
                        </td>
                        <td className="text-muted small">{o.created_at}</td>
                        <td className="text-muted small">{o.paid_at || '-'}</td>
                        <td className="text-muted small">{o.approved_at || '-'}</td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            ) : (
              <div className="card-body p-4 text-muted">暂无订单。购买订阅后会先创建订单，支付后自动生效。</div>
            )}
          </div>
        </div>

        <div className="col-12">
          <div className="d-flex align-items-center mb-3">
            <h4 className="mb-0 fw-bold">我的订阅</h4>
            {hasActive ? <span className="badge bg-success ms-2">活跃</span> : subscriptions.length ? <span className="badge bg-primary ms-2">待生效</span> : <span className="badge bg-secondary ms-2">无活跃订阅</span>}
          </div>

          {data?.subscription ? (
            <div className="text-muted small">
              当前订阅：<strong>{data.subscription.plan_name}</strong>（{data.subscription.start_at} 至 {data.subscription.end_at}）｜组：<code>{data.subscription.group_name}</code>
            </div>
          ) : subscriptions.length ? (
            <div className="text-muted small">已购买 {subscriptions.length} 个订阅（当前无活跃订阅）。</div>
          ) : null}
        </div>

        {loading ? (
          <div className="col-12 text-muted">加载中…</div>
        ) : subscriptions.length ? (
          subscriptions.map((s, idx) => (
            <div key={`${s.plan_name}-${idx}`} className="col-md-6 col-xl-4">
              <div className="card border-0 h-100 overflow-hidden">
                <div className="card-header border-0 pt-4 px-4 pb-0">
                  <div className="d-flex justify-content-between align-items-center">
                    <h5 className="fw-bold mb-0">{s.plan_name}</h5>
                    <span className="text-primary font-monospace fw-bold">{s.price_cny}</span>
                  </div>
                </div>
                <div className="card-body p-4">
                  <div className="mb-4">
                    <div className="text-muted small mb-1">
                      <span className="me-2 material-symbols-rounded">group_work</span>组
                    </div>
                    <div className="small fw-medium">
                      <code>{s.group_name}</code>
                    </div>
                  </div>
                  <div className="mb-4">
                    <div className="text-muted small mb-1">
                      <span className="me-2 material-symbols-rounded">calendar_today</span>有效期
                    </div>
                    <div className="small fw-medium">{s.start_at} 至</div>
                    <div className="small fw-medium">{s.end_at}</div>
                  </div>

                  {s.usage_windows?.length ? (
                    <div className="usage-bars mb-4">
                      {s.usage_windows.map((w) => (
                        <div key={w.window} className="mb-3">
                          <div className="d-flex justify-content-between mb-1">
                            <span className="text-muted smaller fw-bold">{w.window}</span>
                            <span className="smaller fw-bold">{w.used_percent}%</span>
                          </div>
                          <div className="progress" style={{ height: 6 }}>
                            <div
                              className={`progress-bar ${percentBarClass(w.used_percent)}`}
                              role="progressbar"
                              style={{ width: `${w.used_percent}%` }}
                              aria-valuenow={w.used_percent}
                              aria-valuemin={0}
                              aria-valuemax={100}
                            ></div>
                          </div>
                          <div className="d-flex justify-content-between mt-1">
                            <span className="smaller text-muted">{w.used_usd}</span>
                            <span className="smaller text-muted">/ {w.limit_usd}</span>
                          </div>
                        </div>
                      ))}
                    </div>
                  ) : null}

                  {s.active ? (
                    <div className="d-flex align-items-center text-success small">
                      <span className="me-2 material-symbols-rounded">check_circle</span> 订阅生效中
                    </div>
                  ) : (
                    <div className="d-flex align-items-center text-primary small">
                      <span className="me-2 material-symbols-rounded">schedule</span> 待生效
                    </div>
                  )}
                </div>
              </div>
            </div>
          ))
        ) : (
          <div className="col-12">
            <div className="card border-0 bg-light">
              <div className="card-body p-5 text-center">
                <div className="mb-3">
                  <span className="fs-1 text-muted material-symbols-rounded">credit_card</span>
                </div>
                <h5>尚未购买任何订阅</h5>
                <p className="text-muted">下单并完成支付后即可获得 API 访问限额。</p>
                <a href="#plans" className="btn btn-primary mt-2">
                  浏览订阅
                </a>
              </div>
            </div>
          </div>
        )}

        <div className="col-12 mt-5" id="plans">
          <h4 className="mb-4 fw-bold">购买新订阅</h4>
          {loading ? (
            <div className="text-muted">加载中…</div>
          ) : (
            <div className="row g-4">
              {plans.map((p) => (
                <div key={p.id} className="col-md-6 col-xl-3">
                  <div className="card border-0 h-100 hover-shadow transition-all">
                    <div className="card-body p-4 d-flex flex-column">
                      <div className="mb-3">
                        <h5 className="fw-bold mb-1">{p.name}</h5>
                        <div className="d-flex gap-2 flex-wrap">
                          <div className="badge bg-primary bg-opacity-10 text-primary">{p.duration_days} 天有效期</div>
                          <div className="badge bg-secondary bg-opacity-10 text-secondary">组：{p.group_name}</div>
                        </div>
                      </div>

                      <div className="my-3">
                        <div className="display-6 fw-bold text-dark">{p.price_cny}</div>
                        <div className="text-muted small">单次购买</div>
                      </div>

                      <ul className="list-unstyled mb-4 small flex-grow-1">
                        <li className="mb-2">
                          <span className="text-success me-2 material-symbols-rounded">check</span>30天限额: <strong>{p.limit_30d}</strong>
                        </li>
                        <li className="mb-2">
                          <span className="text-success me-2 material-symbols-rounded">check</span>7天限额: <strong>{p.limit_7d}</strong>
                        </li>
                        <li className="mb-2">
                          <span className="text-success me-2 material-symbols-rounded">check</span>1天限额: <strong>{p.limit_1d}</strong>
                        </li>
                        <li className="mb-2">
                          <span className="text-success me-2 material-symbols-rounded">check</span>5小时限额: <strong>{p.limit_5h}</strong>
                        </li>
                      </ul>

                      <button className="btn btn-outline-primary w-100 fw-bold" type="button" disabled={loading} onClick={() => void buy(p)}>
                        下单
                      </button>
                    </div>
                  </div>
                </div>
              ))}
            </div>
          )}
        </div>
      </div>
    </div>
  );
}

