import { useEffect, useState } from 'react';
import { Link, useNavigate } from 'react-router-dom';

import { createTopupOrder, getTopupPage, type BillingTopupPageResponse } from '../api/billing';

function orderBadge(status: string): string {
  if (status === '已入账') return 'badge bg-success bg-opacity-10 text-success';
  if (status === '待支付') return 'badge bg-warning bg-opacity-10 text-warning';
  if (status === '已取消') return 'badge bg-secondary bg-opacity-10 text-secondary';
  return 'badge bg-secondary bg-opacity-10 text-secondary';
}

export function TopupPage() {
  const navigate = useNavigate();

  const [data, setData] = useState<BillingTopupPageResponse | null>(null);
  const [loading, setLoading] = useState(true);
  const [err, setErr] = useState('');
  const [notice, setNotice] = useState('');

  const [amountCNY, setAmountCNY] = useState('');

  useEffect(() => {
    let mounted = true;
    (async () => {
      setErr('');
      setLoading(true);
      try {
        const res = await getTopupPage();
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

  const orders = data?.topup_orders || [];
  const hasPayment = (data?.payment_channels || []).length > 0;

  async function submitCreate() {
    setErr('');
    setNotice('');
    try {
      const res = await createTopupOrder(amountCNY.trim());
      if (!res.success) throw new Error(res.message || '创建订单失败');
      const orderId = res.data?.order_id;
      if (!orderId) throw new Error('创建订单失败：缺少 order_id');
      navigate(`/pay/topup/${orderId}`);
    } catch (e) {
      setErr(e instanceof Error ? e.message : '创建订单失败');
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

        <div className="col-12">
          <div className="d-flex align-items-center mb-3">
            <h4 className="mb-0 fw-bold">余额</h4>
            {data?.pay_as_you_go_enabled ? (
              <span className="badge bg-success bg-opacity-10 text-success ms-2">按量计费已启用</span>
            ) : (
              <span className="badge bg-secondary bg-opacity-10 text-secondary ms-2">按量计费未启用</span>
            )}
          </div>

          <div className="card border-0">
            <div className="card-body p-4">
              <div className="display-6 fw-bold text-dark">{data?.balance_usd || '-'}</div>
              <div className="text-muted small mt-1">余额用于无订阅/订阅额度不足时的按量计费扣费。</div>
            </div>
          </div>
        </div>

        <div className="col-12">
          <div className="d-flex align-items-center mb-3">
            <h4 className="mb-0 fw-bold">创建充值订单</h4>
          </div>

          <div className="card border-0">
            <div className="card-body p-4">
              {!hasPayment ? (
                <div className="alert alert-warning d-flex align-items-center" role="alert">
                  <span className="me-2 material-symbols-rounded">error</span>
                  <div>当前未配置任何支付渠道，请联系管理员在「管理后台 → 支付渠道」配置。</div>
                </div>
              ) : null}

              <form
                className="row g-3"
                onSubmit={(e) => {
                  e.preventDefault();
                  void submitCreate();
                }}
              >
                <div className="col-md-6">
                  <label className="form-label">充值金额（CNY）</label>
                  <input className="form-control" placeholder="10.00" value={amountCNY} onChange={(e) => setAmountCNY(e.target.value)} />
                  <div className="form-text small text-muted">最低充值：{data?.topup_min_cny || '-'}（示例：10.00）</div>
                </div>

                <div className="col-md-6 d-flex align-items-end">
                  <button type="submit" className="btn btn-primary w-100 fw-bold" disabled={!hasPayment || loading}>
                    创建订单并选择支付方式
                  </button>
                </div>
              </form>
            </div>
          </div>
        </div>

        <div className="col-12 mt-2">
          <div className="d-flex align-items-center mb-3">
            <h4 className="mb-0 fw-bold">我的充值订单</h4>
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
                      <th>金额</th>
                      <th>入账额度</th>
                      <th>状态</th>
                      <th>创建时间</th>
                      <th>支付时间</th>
                      <th></th>
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
                          navigate(`/pay/topup/${o.id}`);
                        }}
                      >
                        <td className="font-monospace">
                          <Link className="link-primary text-decoration-none" to={`/pay/topup/${o.id}`}>
                            #{o.id}
                          </Link>
                        </td>
                        <td className="fw-semibold">{o.amount_cny}</td>
                        <td className="text-muted small">{o.credit_usd}</td>
                        <td>
                          <span className={orderBadge(o.status)}>{o.status}</span>
                        </td>
                        <td className="text-muted small">{o.created_at}</td>
                        <td className="text-muted small">{o.paid_at || '-'}</td>
                        <td className="text-end">
                          {o.status === '待支付' ? (
                            <Link className="btn btn-sm btn-outline-primary" to={`/pay/topup/${o.id}`}>
                              去支付
                            </Link>
                          ) : (
                            <span className="text-muted small">-</span>
                          )}
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            ) : (
              <div className="card-body p-4 text-muted">暂无充值订单。</div>
            )}
          </div>
        </div>
      </div>
    </div>
  );
}
