import { useEffect, useMemo, useRef, useState, type MutableRefObject } from 'react';
import { Link } from 'react-router-dom';

import { getDashboard, type DashboardData } from '../api/dashboard';

type ChartInstance = {
  destroy?: () => void;
};

type ChartConstructor = new (ctx: CanvasRenderingContext2D, config: unknown) => ChartInstance;

function subscriptionProgressBarClass(percent: number): string {
  if (percent > 90) return 'bg-danger';
  if (percent > 70) return 'bg-warning';
  return 'bg-success';
}

const fallbackModelIcon =
  "data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' viewBox='0 0 24 24' fill='none' stroke='%236366f1' stroke-width='2' stroke-linecap='round' stroke-linejoin='round'%3E%3Cpath d='M12 2a10 10 0 1 0 10 10H12V2z'/%3E%3Cpath d='M12 12L2.5 12'/%3E%3Cpath d='M12 12l9.5 0'/%3E%3Cpath d='M12 12l-6.7 6.7'/%3E%3Cpath d='M12 12l6.7 6.7'/%3E%3C/svg%3E";

export function DashboardPage() {
  const [data, setData] = useState<DashboardData | null>(null);
  const [loading, setLoading] = useState(true);
  const [err, setErr] = useState('');

  const apiBaseURL = useMemo(() => `${window.location.origin}/v1`, []);

  const modelPieRef = useRef<HTMLCanvasElement | null>(null);
  const tokenBarRef = useRef<HTMLCanvasElement | null>(null);
  const billingBarRef = useRef<HTMLCanvasElement | null>(null);

  const modelPieChartRef = useRef<ChartInstance | null>(null);
  const tokenBarChartRef = useRef<ChartInstance | null>(null);
  const billingBarChartRef = useRef<ChartInstance | null>(null);

  useEffect(() => {
    let mounted = true;
    (async () => {
      setErr('');
      setLoading(true);
      try {
        const res = await getDashboard();
        if (!res.success) {
          throw new Error(res.message || '加载失败');
        }
        if (mounted) {
          setData(res.data || null);
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
  }, []);

  useEffect(() => {
    const onShown = () => {
      window.dispatchEvent(new Event('resize'));
    };

    const btns = Array.from(document.querySelectorAll<HTMLButtonElement>('button[data-bs-toggle="tab"]'));
    for (const btn of btns) {
      btn.addEventListener('shown.bs.tab', onShown);
    }
    return () => {
      for (const btn of btns) {
        btn.removeEventListener('shown.bs.tab', onShown);
      }
    };
  }, []);

  useEffect(() => {
    const ChartCtor = (window as unknown as { Chart?: ChartConstructor }).Chart;

    const destroy = (ref: MutableRefObject<ChartInstance | null>) => {
      try {
        ref.current?.destroy?.();
      } catch {
        // ignore
      }
      ref.current = null;
    };

    destroy(modelPieChartRef);
    destroy(tokenBarChartRef);
    destroy(billingBarChartRef);

    if (!ChartCtor || !data) return;

    const modelCtx = modelPieRef.current?.getContext('2d');
    if (modelCtx) {
      const modelLabels = (data.charts.model_stats || []).map((s) => s.model);
      const modelCosts = (data.charts.model_stats || []).map((s) => {
        const n = Number.parseFloat(String(s.committed_usd || '0'));
        return Number.isFinite(n) ? n : 0;
      });
      const modelColors = (data.charts.model_stats || []).map((s) => s.color);

      modelPieChartRef.current = new ChartCtor(modelCtx, {
        type: 'doughnut',
        data: {
          labels: modelLabels,
          datasets: [
            {
              data: modelCosts,
              backgroundColor: modelColors,
              borderWidth: 0,
            },
          ],
        },
        options: {
          responsive: true,
          maintainAspectRatio: false,
          plugins: {
            legend: { display: false },
          },
          cutout: '70%',
        },
      });
    }

    const timeLabels = (data.charts.time_series_stats || []).map((s) => s.label);

    const tokenCtx = tokenBarRef.current?.getContext('2d');
    if (tokenCtx) {
      tokenBarChartRef.current = new ChartCtor(tokenCtx, {
        type: 'bar',
        data: {
          labels: timeLabels,
          datasets: [
            {
              label: 'Tokens',
              data: (data.charts.time_series_stats || []).map((s) => s.tokens),
              backgroundColor: 'rgba(16, 185, 129, 0.6)',
              borderRadius: 4,
            },
          ],
        },
        options: {
          responsive: true,
          maintainAspectRatio: false,
          plugins: { legend: { display: false } },
          scales: {
            y: { beginAtZero: true, grid: { display: false } },
            x: { grid: { display: false } },
          },
        },
      });
    }

    const billingCtx = billingBarRef.current?.getContext('2d');
    if (billingCtx) {
      billingBarChartRef.current = new ChartCtor(billingCtx, {
        type: 'bar',
        data: {
          labels: timeLabels,
          datasets: [
            {
              label: '费用 (USD)',
              data: (data.charts.time_series_stats || []).map((s) => s.committed_usd),
              backgroundColor: 'rgba(99, 102, 241, 0.6)',
              borderRadius: 4,
            },
          ],
        },
        options: {
          responsive: true,
          maintainAspectRatio: false,
          plugins: { legend: { display: false } },
          scales: {
            y: {
              beginAtZero: true,
              grid: { display: false },
              ticks: { callback: (v: unknown) => `$${String(v)}` },
            },
            x: { grid: { display: false } },
          },
        },
      });
    }

    return () => {
      destroy(modelPieChartRef);
      destroy(tokenBarChartRef);
      destroy(billingBarChartRef);
    };
  }, [data]);

  const todayUsageUSD = data?.today_usage_usd || '-';
  const todayRequests = data ? String(data.today_requests) : '-';
  const todayRPM = data?.today_rpm || '-';
  const todayTokens = data ? String(data.today_tokens) : '-';
  const todayTPM = data?.today_tpm || '-';
  const unreadAnnouncementsCount = data?.unread_announcements_count || 0;

  const subscription = data?.subscription && data.subscription.active ? data.subscription : null;

  return (
    <div className="fade-in-up">
      {err ? (
        <div className="alert alert-danger d-flex align-items-center" role="alert">
          <span className="me-2 material-symbols-rounded">warning</span>
          <div>{err}</div>
        </div>
      ) : null}

      <div className="row g-4">
        <div className="col-12">
          <div className="row g-4">
            <div className="col-md-6 col-xl-3">
              <div className="card h-100">
                <div className="card-body">
                  <div className="d-flex align-items-center mb-3">
                    <div className="bg-primary bg-opacity-10 text-primary rounded-pill p-2 me-3">
                      <span className="fs-4 px-1 material-symbols-rounded">attach_money</span>
                    </div>
                    <h6 className="card-title mb-0 fw-bold">今日费用</h6>
                  </div>
                  <div className="mb-3">
                    <h3 className="fw-bold mb-1">{todayUsageUSD}</h3>
                    <p className="text-muted small mb-0">预估消耗 (USD)</p>
                  </div>
                </div>
              </div>
            </div>

            <div className="col-md-6 col-xl-3">
              <div className="card h-100">
                <div className="card-body">
                  <div className="d-flex align-items-center mb-3">
                    <div className="bg-info bg-opacity-10 text-info rounded-pill p-2 me-3">
                      <span className="fs-4 px-1 material-symbols-rounded">chat</span>
                    </div>
                    <h6 className="card-title mb-0 fw-bold">今日请求</h6>
                  </div>
                  <div className="mb-3">
                    <h3 className="fw-bold mb-1">{todayRequests}</h3>
                    <div className="text-muted small">
                      <span className="badge bg-light text-secondary border fw-normal">RPM: {todayRPM}</span>
                      <span className="ms-1">次/分钟</span>
                    </div>
                  </div>
                </div>
              </div>
            </div>

            <div className="col-md-6 col-xl-3">
              <div className="card h-100">
                <div className="card-body">
                  <div className="d-flex align-items-center mb-3">
                    <div className="bg-success bg-opacity-10 text-success rounded-pill p-2 me-3">
                      <span className="fs-4 px-1 material-symbols-rounded">memory</span>
                    </div>
                    <h6 className="card-title mb-0 fw-bold">今日 Token</h6>
                  </div>
                  <div className="mb-3">
                    <h3 className="fw-bold mb-1">{todayTokens}</h3>
                    <div className="text-muted small">
                      <span className="badge bg-light text-secondary border fw-normal">TPM: {todayTPM}</span>
                      <span className="ms-1">Tokens/分钟</span>
                    </div>
                  </div>
                </div>
              </div>
            </div>

            {subscription ? (
              <div className="col-md-6 col-xl-3">
                <div className="card h-100">
                  <div className="card-body">
                    <div className="d-flex align-items-center mb-3">
                      <div className="bg-warning bg-opacity-10 text-warning rounded-pill p-2 me-3">
                        <span className="fs-4 px-1 material-symbols-rounded">diamond</span>
                      </div>
                      <h6 className="card-title mb-0 fw-bold">当前订阅</h6>
                    </div>
                    <div className="mb-1">
                      <h5 className="fw-bold mb-1">{subscription.plan_name || '-'}</h5>
                      <p className="text-muted small mb-0">至: {subscription.end_at || '-'}</p>
                    </div>
                    {(subscription.usage_windows || []).map((w, i) => (
                      <div key={`${w.window}-${i}`} className="mt-2">
                        <div className="d-flex justify-content-between mb-0 smaller">
                          <span className="text-muted">{w.window}</span>
                          <span className="fw-medium">
                            {w.used_usd}/{w.limit_usd}
                          </span>
                        </div>
                        <div className="progress" style={{ height: 4 }}>
                          <div
                            className={`progress-bar ${subscriptionProgressBarClass(w.used_percent)}`}
                            role="progressbar"
                            style={{ width: `${Math.min(100, Math.max(0, w.used_percent))}%` }}
                          ></div>
                        </div>
                      </div>
                    ))}
                  </div>
                </div>
              </div>
            ) : (
              <div className="col-md-6 col-xl-3">
                <div className="card h-100 border-dashed">
                  <div className="card-body d-flex flex-column align-items-center justify-content-center text-center py-4">
                    <div className="bg-light text-muted rounded-circle p-2 mb-2">
                      <span className="fs-4 material-symbols-rounded">event_available</span>
                    </div>
                    <h6 className="fw-bold small mb-1">暂无订阅</h6>
                    <a href="/subscriptions" className="btn btn-outline-primary btn-sm px-3 py-1 smaller">
                      浏览套餐
                    </a>
                  </div>
                </div>
              </div>
            )}

            {unreadAnnouncementsCount > 0 ? (
              <div className="col-12">
                <div className="card h-100 bg-warning bg-opacity-10">
                  <div className="card-body d-flex flex-column">
                    <div className="d-flex align-items-center mb-3">
                      <div className="bg-warning bg-opacity-25 text-warning rounded-pill p-2 me-3">
                        <span className="fs-4 px-1 material-symbols-rounded">campaign</span>
                      </div>
                      <h6 className="card-title mb-0 fw-bold text-warning-emphasis">重要公告</h6>
                    </div>
                    <div className="flex-grow-1">
                      <p className="fw-semibold mb-1">你有 {unreadAnnouncementsCount} 条未读公告</p>
                      <p className="text-muted small">请及时查看最新动态和维护通知。</p>
                    </div>
                    <Link to="/announcements" className="btn btn-warning btn-sm w-100 mt-2">
                      查看公告
                    </Link>
                  </div>
                </div>
              </div>
            ) : null}
          </div>
        </div>

        <div className="col-12">
          <div className="card">
            <div className="card-header">
              <div className="d-flex justify-content-between align-items-center">
                <h6 className="mb-0 fw-bold">
                  <span className="text-primary me-2 material-symbols-rounded">pie_chart</span>用量分析 (今日)
                </h6>
                <ul className="nav nav-pills nav-pills-sm smaller" id="chartTabs" role="tablist">
                  <li className="nav-item" role="presentation">
                    <button
                      className="nav-link active py-1 px-2"
                      id="model-tab"
                      data-bs-toggle="tab"
                      data-bs-target="#model-pane"
                      type="button"
                      role="tab"
                    >
                      模型分布
                    </button>
                  </li>
                  <li className="nav-item" role="presentation">
                    <button className="nav-link py-1 px-2" id="token-tab" data-bs-toggle="tab" data-bs-target="#token-pane" type="button" role="tab">
                      Token
                    </button>
                  </li>
                  <li className="nav-item" role="presentation">
                    <button
                      className="nav-link py-1 px-2"
                      id="billing-tab"
                      data-bs-toggle="tab"
                      data-bs-target="#billing-pane"
                      type="button"
                      role="tab"
                    >
                      费用
                    </button>
                  </li>
                </ul>
              </div>
            </div>

            <div className="card-body px-3 pb-3 pt-0">
              <div className="tab-content pt-2">
                <div className="tab-pane fade show active" id="model-pane" role="tabpanel">
                  <div className="row align-items-center g-3">
                    <div className="col-lg-4 text-center">
                      <div style={{ position: 'relative', height: 140, width: '100%', margin: '0 auto' }}>
                        <canvas ref={modelPieRef} id="modelPieChart"></canvas>
                      </div>
                    </div>
                    <div className="col-lg-8">
                      <div className="table-responsive" style={{ maxHeight: 150 }}>
                        <table className="table table-sm table-hover align-middle border-0 mb-0 smaller">
                          <thead className="sticky-top bg-white" style={{ zIndex: 1 }}>
                            <tr className="text-muted text-uppercase">
                              <th className="border-0 ps-0 pb-1 fw-medium">模型</th>
                              <th className="border-0 text-end pb-1 fw-medium">请求</th>
                              <th className="border-0 text-end pb-1 fw-medium">Token</th>
                              <th className="border-0 text-end pb-1 pe-1 fw-medium">USD</th>
                            </tr>
                          </thead>
                          <tbody className="border-top-0">
                            {data?.charts.model_stats?.length ? (
                              data.charts.model_stats.map((s) => (
                                <tr key={s.model}>
                                  <td className="border-0 ps-0 py-1">
                                    <div className="d-flex align-items-center">
                                      <span
                                        className="rounded-circle me-2"
                                        style={{ width: 6, height: 6, backgroundColor: s.color }}
                                      ></span>
                                      <img
                                        src={s.icon_url || fallbackModelIcon}
                                        className="rlm-model-icon me-2"
                                        style={{ width: 14, height: 14 }}
                                        alt={s.model}
                                        loading="lazy"
                                        onError={(e) => {
                                          e.currentTarget.src = fallbackModelIcon;
                                        }}
                                      />
                                      <span className="text-dark text-truncate" style={{ maxWidth: 120 }}>
                                        {s.model}
                                      </span>
                                    </div>
                                  </td>
                                  <td className="border-0 text-end text-secondary py-1">{s.requests}</td>
                                  <td className="border-0 text-end text-secondary py-1">{s.tokens}</td>
                                  <td className="border-0 text-end fw-bold text-dark py-1 pe-1">${s.committed_usd}</td>
                                </tr>
                              ))
                            ) : (
                              <tr>
                                <td colSpan={4} className="text-center py-4 text-muted">
                                  {loading ? '加载中…' : '无记录'}
                                </td>
                              </tr>
                            )}
                          </tbody>
                        </table>
                      </div>
                    </div>
                  </div>
                </div>

                <div className="tab-pane fade" id="token-pane" role="tabpanel">
                  <div style={{ height: 150, width: '100%' }}>
                    <canvas ref={tokenBarRef} id="tokenBarChart"></canvas>
                  </div>
                </div>

                <div className="tab-pane fade" id="billing-pane" role="tabpanel">
                  <div style={{ height: 150, width: '100%' }}>
                    <canvas ref={billingBarRef} id="billingBarChart"></canvas>
                  </div>
                </div>
              </div>
            </div>
          </div>
        </div>

        <div className="col-12">
          <div className="card">
            <div className="card-body">
              <div className="d-flex align-items-center mb-4">
                <span className="text-primary me-2 material-symbols-rounded">bolt</span>
                <h5 className="mb-0 fw-bold">快速开始</h5>
              </div>

              <div className="row g-4">
                <div className="col-12">
                  <div className="bg-dark rounded-3 p-3 position-relative overflow-hidden mb-3">
                    <div className="d-flex justify-content-between align-items-center mb-2">
                      <small className="text-secondary text-uppercase fw-bold smaller">终端配置</small>
                      <div className="d-flex gap-1">
                        <div className="rounded-circle bg-danger" style={{ width: 8, height: 8 }}></div>
                        <div className="rounded-circle bg-warning" style={{ width: 8, height: 8 }}></div>
                        <div className="rounded-circle bg-success" style={{ width: 8, height: 8 }}></div>
                      </div>
                    </div>
                    <pre className="mb-0 text-light overflow-auto smaller font-monospace" style={{ whiteSpace: 'pre-wrap' }}>
                      <code>{`# Linux/macOS
export OPENAI_BASE_URL="${apiBaseURL}"
export OPENAI_API_KEY="rlm_..."

# ~/.codex/config.toml
model_provider = "realms"

[model_providers.realms]
name = "Realms"
base_url = "${apiBaseURL}"
wire_api = "responses"
requires_openai_auth = true`}</code>
                    </pre>
                  </div>
                  <div className="alert alert-light border-0 bg-light small d-flex align-items-start mb-0">
                    <span className="me-2 mt-1 text-primary material-symbols-rounded">info</span>
                    <div>
                      API 基础地址：<strong className="user-select-all ms-1">{apiBaseURL}</strong>
                    </div>
                  </div>
                </div>
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}
