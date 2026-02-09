import { useEffect, useRef, useState, type MutableRefObject } from 'react';
import { Link } from 'react-router-dom';

import { getDashboard, type DashboardData } from '../api/dashboard';
import { getUsageTimeSeries, type UsageTimeSeriesPoint } from '../api/usage';

type ChartInstance = {
  destroy?: () => void;
};

type ChartConstructor = new (ctx: CanvasRenderingContext2D, config: unknown) => ChartInstance;

function subscriptionProgressBarClass(percent: number): string {
  if (percent > 90) return 'bg-danger';
  if (percent > 70) return 'bg-warning';
  return 'bg-success';
}

export function DashboardPage() {
  const [data, setData] = useState<DashboardData | null>(null);
  const [err, setErr] = useState('');

  const detailTimeLineRef = useRef<HTMLCanvasElement | null>(null);
  const detailTimeLineChartRef = useRef<ChartInstance | null>(null);
  const [detailSeries, setDetailSeries] = useState<UsageTimeSeriesPoint[]>([]);
  const [detailSeriesLoading, setDetailSeriesLoading] = useState(false);
  const [detailSeriesErr, setDetailSeriesErr] = useState('');
  const [detailField, setDetailField] = useState<'requests' | 'tokens' | 'committed_usd' | 'cache_ratio' | 'avg_first_token_latency' | 'tokens_per_second'>(
    'requests',
  );
  const [detailGranularity, setDetailGranularity] = useState<'hour' | 'day'>('hour');
  const fieldOptions: Array<{
    value: 'requests' | 'tokens' | 'committed_usd' | 'cache_ratio' | 'avg_first_token_latency' | 'tokens_per_second';
    label: string;
  }> = [
    { value: 'requests', label: '请求数' },
    { value: 'tokens', label: 'Token' },
    { value: 'committed_usd', label: '消耗 (USD)' },
    { value: 'cache_ratio', label: '缓存率 (%)' },
    { value: 'avg_first_token_latency', label: '首字延迟 (ms)' },
    { value: 'tokens_per_second', label: 'Tokens/s' },
  ];
  const granularityOptions: Array<{ value: 'hour' | 'day'; label: string }> = [
    { value: 'hour', label: '按小时' },
    { value: 'day', label: '按天' },
  ];

  useEffect(() => {
    let mounted = true;
    (async () => {
      setErr('');
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
      }
    })();
    return () => {
      mounted = false;
    };
  }, []);

  useEffect(() => {
    let active = true;
    void (async () => {
      setDetailSeriesErr('');
      setDetailSeriesLoading(true);
      try {
        const res = await getUsageTimeSeries(undefined, undefined, detailGranularity);
        if (!res.success) throw new Error(res.message || '时间序列加载失败');
        if (!active) return;
        setDetailSeries(res.data?.points || []);
      } catch (e) {
        if (!active) return;
        setDetailSeries([]);
        setDetailSeriesErr(e instanceof Error ? e.message : '时间序列加载失败');
      } finally {
        if (active) setDetailSeriesLoading(false);
      }
    })();
    return () => {
      active = false;
    };
  }, [detailGranularity]);

  useEffect(() => {
    const ChartCtor = (globalThis.window as unknown as { Chart?: ChartConstructor })?.Chart;

    const destroy = (ref: MutableRefObject<ChartInstance | null>) => {
      try {
        ref.current?.destroy?.();
      } catch {
        // ignore
      }
      ref.current = null;
    };

    destroy(detailTimeLineChartRef);
    if (!ChartCtor) return;
    const ctx = detailTimeLineRef.current?.getContext('2d');
    if (!ctx) return;

    const fieldMeta: Record<string, { label: string; color: string; read: (point: UsageTimeSeriesPoint) => number }> = {
      requests: {
        label: '请求数',
        color: 'rgba(59, 130, 246, 0.95)',
        read: (point) => point.requests,
      },
      tokens: {
        label: 'Token',
        color: 'rgba(16, 185, 129, 0.95)',
        read: (point) => point.tokens,
      },
      committed_usd: {
        label: '消耗 (USD)',
        color: 'rgba(99, 102, 241, 0.95)',
        read: (point) => point.committed_usd,
      },
      cache_ratio: {
        label: '缓存率 (%)',
        color: 'rgba(245, 158, 11, 0.95)',
        read: (point) => point.cache_ratio,
      },
      avg_first_token_latency: {
        label: '首字延迟 (ms)',
        color: 'rgba(239, 68, 68, 0.95)',
        read: (point) => point.avg_first_token_latency,
      },
      tokens_per_second: {
        label: 'Tokens/s',
        color: 'rgba(14, 165, 233, 0.95)',
        read: (point) => point.tokens_per_second,
      },
    };
    const meta = fieldMeta[detailField];

    detailTimeLineChartRef.current = new ChartCtor(ctx, {
      type: 'line',
      data: {
        labels: detailSeries.map((point) => point.bucket),
        datasets: [
          {
            label: meta.label,
            data: detailSeries.map((point) => meta.read(point)),
            borderColor: meta.color,
            backgroundColor: meta.color.replace('0.95', '0.18'),
            pointRadius: 2,
            tension: 0.2,
          },
        ],
      },
      options: {
        responsive: true,
        maintainAspectRatio: false,
        interaction: { mode: 'index', intersect: false },
        plugins: {
          legend: { position: 'bottom' },
          title: { display: true, text: '用量时间序列' },
        },
        scales: {
          x: {
            grid: { display: false },
            ticks: {
              autoSkip: true,
              maxTicksLimit: detailGranularity === 'hour' ? 10 : 14,
              maxRotation: 0,
              minRotation: 0,
            },
          },
          y: {
            beginAtZero: true,
            suggestedMax: detailField === 'cache_ratio' ? 100 : undefined,
            grid: { color: 'rgba(148, 163, 184, 0.18)' },
          },
        },
      },
    });

    return () => {
      destroy(detailTimeLineChartRef);
    };
  }, [detailSeries, detailField, detailGranularity]);

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
                    <Link to="/subscription" className="btn btn-outline-primary btn-sm px-3 py-1 smaller">
                      浏览套餐
                    </Link>
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
          <div className="card border-0 overflow-hidden">
            <div className="card-header py-3 px-4">
              <h5 className="mb-0 fw-bold">
                <i className="ri-line-chart-line me-2"></i>用量时间序列
              </h5>
            </div>
            <div className="card-body p-4 border-top">
              <div className="d-flex flex-wrap align-items-center gap-3 mb-2">
                <div className="d-flex align-items-center gap-2 flex-grow-1">
                  <div className="d-flex flex-wrap gap-1">
                    {fieldOptions.map((option) => (
                      <button
                        key={option.value}
                        type="button"
                        className={`btn btn-sm ${detailField === option.value ? 'btn-primary' : 'btn-outline-secondary'}`}
                        onClick={() => setDetailField(option.value)}
                      >
                        {option.label}
                      </button>
                    ))}
                  </div>
                </div>
                <div className="d-flex align-items-center gap-2 ms-auto">
                  <div className="d-flex gap-1">
                    {granularityOptions.map((option) => (
                      <button
                        key={option.value}
                        type="button"
                        className={`btn btn-sm ${detailGranularity === option.value ? 'btn-primary' : 'btn-outline-secondary'}`}
                        onClick={() => setDetailGranularity(option.value)}
                      >
                        {option.label}
                      </button>
                    ))}
                  </div>
                </div>
              </div>
              {detailSeriesErr ? <div className="alert alert-danger py-2 mb-2">{detailSeriesErr}</div> : null}
              {detailSeriesLoading ? (
                <div className="text-muted small py-4">时间序列加载中…</div>
              ) : (
                <>
                  <div style={{ height: 280 }}>
                    <canvas ref={detailTimeLineRef}></canvas>
                  </div>
                </>
              )}
            </div>
          </div>
        </div>

      </div>
    </div>
  );
}
