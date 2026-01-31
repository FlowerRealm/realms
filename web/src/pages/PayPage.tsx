import { useEffect, useMemo, useState } from 'react';
import { useLocation, useParams } from 'react-router-dom';

import { cancelPayOrder, getPayPage, startPayment, type BillingPayPageResponse, type BillingPaymentChannelView } from '../api/billing';

function statusBadge(status: string): { cls: string; text: string } {
  if (status === '已生效' || status === '已入账') return { cls: 'badge bg-success bg-opacity-10 text-success', text: status };
  if (status === '待支付') return { cls: 'badge bg-warning bg-opacity-10 text-warning', text: status };
  if (status === '已取消') return { cls: 'badge bg-secondary bg-opacity-10 text-secondary', text: status };
  return { cls: 'badge bg-secondary bg-opacity-10 text-secondary', text: status };
}

export function PayPage() {
  const params = useParams();
  const location = useLocation();

  const kind = (params.kind || '').toString().trim();
  const orderId = Number.parseInt((params.orderId || '').toString(), 10);

  const [data, setData] = useState<BillingPayPageResponse | null>(null);
  const [loading, setLoading] = useState(true);
  const [err, setErr] = useState('');
  const [notice, setNotice] = useState('');

  const [epayTypeByChannel, setEpayTypeByChannel] = useState<Record<number, string>>({});
  const [epayTypeFallback, setEpayTypeFallback] = useState('alipay');

  const pathState = useMemo(() => {
    const p = location.pathname;
    if (p.endsWith('/success')) return 'success';
    if (p.endsWith('/cancel')) return 'cancel';
    return '';
  }, [location.pathname]);

  useEffect(() => {
    let mounted = true;
    (async () => {
      setErr('');
      setNotice('');
      setLoading(true);
      try {
        if (!kind || !Number.isFinite(orderId) || orderId <= 0) {
          throw new Error('参数错误');
        }
        const res = await getPayPage(kind, orderId);
        if (!res.success) throw new Error(res.message || '加载失败');
        if (!mounted) return;
        setData(res.data || null);
        if (pathState === 'success') {
          setNotice('支付完成后会自动入账/生效。若页面未立即更新，请稍等并刷新查看状态。');
        } else if (pathState === 'cancel') {
          setErr('支付已取消或未完成。若您已完成支付，请联系管理员处理。');
        }
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
  }, [kind, orderId, pathState]);

  const payOrder = data?.pay_order;
  const badge = statusBadge(payOrder?.status || '');

  const canPay = payOrder?.status === '待支付';

  async function doCancel() {
    if (!canPay) return;
    setErr('');
    setNotice('');
    try {
      const res = await cancelPayOrder(kind, orderId);
      if (!res.success) throw new Error(res.message || '关闭订单失败');
      setNotice(res.message || '订单已取消。若您已完成支付，请联系管理员处理退款。');
      const refreshed = await getPayPage(kind, orderId);
      if (refreshed.success) setData(refreshed.data || null);
    } catch (e) {
      setErr(e instanceof Error ? e.message : '关闭订单失败');
    }
  }

  async function doPayByChannel(ch: BillingPaymentChannelView | null, method: string | null, epayType: string | null) {
    setErr('');
    setNotice('');
    try {
      const req: { payment_channel_id?: number; method?: string; epay_type?: string } = {};
      if (ch) req.payment_channel_id = ch.id;
      if (method) req.method = method;
      if (epayType) req.epay_type = epayType;
      const res = await startPayment(kind, orderId, req);
      if (!res.success) throw new Error(res.message || '发起支付失败');
      const url = (res.data?.redirect_url || '').toString().trim();
      if (!url) throw new Error('发起支付失败：缺少 redirect_url');
      window.location.href = url;
    } catch (e) {
      setErr(e instanceof Error ? e.message : '发起支付失败');
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
          <div className="d-flex align-items-center justify-content-between mb-3">
            <div>
              <h4 className="mb-0 fw-bold">支付</h4>
              {payOrder ? <div className="text-muted small">{payOrder.title}</div> : null}
            </div>
          </div>

          <div className="card border-0">
            <div className="card-body p-4">
              {loading ? (
                <div className="text-muted">加载中…</div>
              ) : payOrder ? (
                <>
                  <div className="row g-3">
                    <div className="col-md-4">
                      <div className="text-muted small">订单号</div>
                      <div className="font-monospace fw-bold">#{payOrder.id}</div>
                    </div>
                    <div className="col-md-4">
                      <div className="text-muted small">金额</div>
                      <div className="fw-bold">{payOrder.amount_cny}</div>
                    </div>
                    <div className="col-md-4">
                      <div className="text-muted small">状态</div>
                      <div>
                        <span className={badge.cls}>{badge.text}</span>
                      </div>
                    </div>

                    {payOrder.credit_usd ? (
                      <div className="col-md-4">
                        <div className="text-muted small">入账额度</div>
                        <div className="text-muted small">{payOrder.credit_usd}</div>
                      </div>
                    ) : null}

                    <div className="col-md-4">
                      <div className="text-muted small">创建时间</div>
                      <div className="text-muted small">{payOrder.created_at}</div>
                    </div>
                  </div>

                  {canPay ? (
                    <div className="d-flex justify-content-end mt-3">
                      <button type="button" className="btn btn-sm btn-outline-danger" onClick={() => void doCancel()}>
                        关闭订单
                      </button>
                    </div>
                  ) : null}

                  {payOrder.status === '已取消' ? (
                    <div className="alert alert-warning mt-4 d-flex align-items-center" role="alert">
                      <span className="me-2 material-symbols-rounded">warning</span>
                      <div>该订单已取消。若您已完成支付，请联系管理员处理退款。</div>
                    </div>
                  ) : payOrder.status !== '待支付' ? (
                    <div className="alert alert-info mt-4 d-flex align-items-center" role="alert">
                      <span className="me-2 material-symbols-rounded">info</span>
                      <div>该订单已不处于待支付状态，无需再次支付。</div>
                    </div>
                  ) : (
                    <div className="mt-4">
                      {data?.payment_channels?.length ? (
                        <>
                          <h5 className="fw-semibold mb-3">选择支付渠道</h5>
                          <div className="row g-3">
                            {data.payment_channels.map((ch) => (
                              <div key={ch.id} className="col-md-6">
                                <div className="card bg-light h-100">
                                  <div className="card-body p-3">
                                    <div className="d-flex align-items-center justify-content-between mb-2">
                                      <div className="fw-semibold">
                                        <span className="me-2 material-symbols-rounded">credit_card</span>
                                        {ch.type_label}
                                      </div>
                                      <span className="badge bg-light text-dark border">{ch.name}</span>
                                    </div>

                                    {ch.type === 'epay' ? (
                                      <>
                                        <label className="form-label small text-muted mb-1">支付类型</label>
                                        <select
                                          className="form-select"
                                          value={epayTypeByChannel[ch.id] || 'alipay'}
                                          onChange={(e) => setEpayTypeByChannel((p) => ({ ...p, [ch.id]: e.target.value }))}
                                        >
                                          <option value="alipay">支付宝</option>
                                          <option value="wxpay">微信</option>
                                          <option value="qqpay">QQ</option>
                                        </select>
                                        <button
                                          type="button"
                                          className="btn btn-outline-primary w-100 mt-2"
                                          onClick={() => void doPayByChannel(ch, null, epayTypeByChannel[ch.id] || 'alipay')}
                                        >
                                          使用该渠道支付
                                        </button>
                                        <div className="form-text small text-muted mt-2">点击后跳转至 EPay 网关完成支付。</div>
                                      </>
                                    ) : ch.type === 'stripe' ? (
                                      <>
                                        <button type="button" className="btn btn-primary w-100" onClick={() => void doPayByChannel(ch, null, null)}>
                                          使用该渠道支付
                                        </button>
                                        <div className="form-text small text-muted mt-2">点击后跳转至 Stripe Checkout 完成支付。</div>
                                      </>
                                    ) : (
                                      <button type="button" className="btn btn-outline-primary w-100" onClick={() => void doPayByChannel(ch, null, null)}>
                                        使用该渠道支付
                                      </button>
                                    )}
                                  </div>
                                </div>
                              </div>
                            ))}
                          </div>
                        </>
                      ) : (
                        <>
                          <h5 className="fw-semibold mb-3">选择支付方式</h5>
                          <div className="row g-3">
                            <div className="col-md-6">
                              <div className="card bg-light h-100">
                                <div className="card-body p-3">
                                  <div className="fw-semibold mb-2">
                                    <span className="me-2 material-symbols-rounded">credit_card</span>Stripe
                                  </div>
                                  <button
                                    type="button"
                                    className="btn btn-primary w-100"
                                    disabled={!data?.payment_stripe_enabled}
                                    onClick={() => void doPayByChannel(null, 'stripe', null)}
                                  >
                                    使用 Stripe 支付
                                  </button>
                                  {data?.payment_stripe_enabled ? (
                                    <div className="form-text small text-muted mt-2">点击后跳转至 Stripe Checkout 完成支付。</div>
                                  ) : (
                                    <div className="form-text small text-muted mt-2">Stripe 未配置或未启用。</div>
                                  )}
                                </div>
                              </div>
                            </div>

                            <div className="col-md-6">
                              <div className="card bg-light h-100">
                                <div className="card-body p-3">
                                  <div className="fw-semibold mb-2">
                                    <span className="me-2 material-symbols-rounded">credit_card</span>EPay
                                  </div>
                                  <label className="form-label small text-muted mb-1">支付类型</label>
                                  <select
                                    className="form-select"
                                    disabled={!data?.payment_epay_enabled}
                                    value={epayTypeFallback}
                                    onChange={(e) => setEpayTypeFallback(e.target.value)}
                                  >
                                    <option value="alipay">支付宝</option>
                                    <option value="wxpay">微信</option>
                                    <option value="qqpay">QQ</option>
                                  </select>
                                  <button
                                    type="button"
                                    className="btn btn-outline-primary w-100 mt-2"
                                    disabled={!data?.payment_epay_enabled}
                                    onClick={() => void doPayByChannel(null, 'epay', epayTypeFallback)}
                                  >
                                    使用 EPay 支付
                                  </button>
                                  {data?.payment_epay_enabled ? (
                                    <div className="form-text small text-muted mt-2">点击后跳转至 EPay 网关完成支付。</div>
                                  ) : (
                                    <div className="form-text small text-muted mt-2">EPay 未配置或未启用。</div>
                                  )}
                                </div>
                              </div>
                            </div>
                          </div>
                        </>
                      )}

                      <div className="text-muted small mt-3">支付完成后会自动入账/生效。若页面未立即更新，请稍等并刷新查看状态。</div>
                    </div>
                  )}
                </>
              ) : (
                <div className="text-muted">订单不存在或已失效。</div>
              )}
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}
