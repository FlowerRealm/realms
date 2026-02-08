import { useEffect, useRef, useState, type MutableRefObject } from 'react';
import { Link } from 'react-router-dom';

import { useAuth } from '../../auth/AuthContext';
import { getAdminHome, type AdminHome } from '../../api/admin/home';
import { getAdminUsageTimeSeries, type AdminUsageTimeSeriesPoint } from '../../api/admin/usage';

type ChartInstance = {
  destroy?: () => void;
};

type ChartConstructor = new (ctx: CanvasRenderingContext2D, config: unknown) => ChartInstance;

export function AdminHomePage() {
  const { user } = useAuth();
  const [data, setData] = useState<AdminHome | null>(null);
  const [err, setErr] = useState('');
  const detailTimeLineRef = useRef<HTMLCanvasElement | null>(null);
  const detailTimeLineChartRef = useRef<ChartInstance | null>(null);
  const [detailSeries, setDetailSeries] = useState<AdminUsageTimeSeriesPoint[]>([]);
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
        const res = await getAdminHome();
        if (!res.success) throw new Error(res.message || '加载失败');
        if (mounted) setData(res.data || null);
      } catch (e) {
        if (!mounted) return;
        setErr(e instanceof Error ? e.message : '加载失败');
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
        const res = await getAdminUsageTimeSeries({ granularity: detailGranularity });
        if (!res.success) throw new Error(res.message || '加载时间序列失败');
        if (!active) return;
        setDetailSeries(res.data?.points || []);
      } catch (e) {
        if (!active) return;
        setDetailSeries([]);
        setDetailSeriesErr(e instanceof Error ? e.message : '加载时间序列失败');
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

    const fieldMeta: Record<string, { label: string; color: string; read: (point: AdminUsageTimeSeriesPoint) => number }> = {
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
          title: { display: true, text: '今日概述 · 时间序列' },
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

  if (err) {
    return (
      <div className="alert alert-danger mb-4" role="alert">
        <i className="ri-alert-line me-2"></i>
        {err}
      </div>
    );
  }

  if (!data) {
    return (
      <div className="card border-0 shadow-sm">
        <div className="card-body text-muted small d-flex align-items-center">
          <span className="spinner-border spinner-border-sm me-2" role="status" aria-hidden="true"></span>
          正在加载…
        </div>
      </div>
    );
  }

  const tz = data.admin_time_zone || 'UTC';
  const stats = data.stats;

  return (
    <div className="fade-in-up">
      <div className="d-flex align-items-center justify-content-between mb-4">
        <h2 className="h4 fw-bold mb-0 text-dark">仪表盘</h2>
        <span className="badge bg-white text-secondary border shadow-sm">{tz} 时间</span>
      </div>

      <div className="row g-4 mb-4">
        <div className="col-md-4">
          <div className="card h-100 border-0 shadow-sm metric-card" style={{ borderTop: '3px solid var(--bs-primary)' }}>
            <div className="card-body d-flex align-items-center">
              <div className="bg-primary bg-opacity-10 p-3 rounded-circle me-3">
                <i className="ri-group-line fs-4 text-primary"></i>
              </div>
              <div>
                <h6 className="text-muted text-uppercase mb-1 small fw-semibold">总用户数</h6>
                <h3 className="mb-0 fw-bold text-dark">{stats.users_count}</h3>
              </div>
            </div>
          </div>
        </div>

        <div className="col-md-4">
          <div className="card h-100 border-0 shadow-sm metric-card" style={{ borderTop: '3px solid #10b981' }}>
            <div className="card-body d-flex align-items-center">
              <div className="bg-success bg-opacity-10 p-3 rounded-circle me-3">
                <i className="ri-git-merge-line fs-4 text-success"></i>
              </div>
              <div>
                <h6 className="text-muted text-uppercase mb-1 small fw-semibold">上游渠道</h6>
                <h3 className="mb-0 fw-bold text-dark">{stats.channels_count}</h3>
              </div>
            </div>
          </div>
        </div>

        <div className="col-md-4">
          <div className="card h-100 border-0 shadow-sm metric-card" style={{ borderTop: '3px solid #0ea5e9' }}>
            <div className="card-body d-flex align-items-center">
              <div className="bg-info bg-opacity-10 p-3 rounded-circle me-3">
                <i className="ri-server-line fs-4 text-info"></i>
              </div>
              <div>
                <h6 className="text-muted text-uppercase mb-1 small fw-semibold">上游节点</h6>
                <h3 className="mb-0 fw-bold text-dark">{stats.endpoints_count}</h3>
              </div>
            </div>
          </div>
        </div>
      </div>

      <div className="card border-0 shadow-sm mb-4 overflow-hidden">
        <div className="card-header bg-white border-bottom py-3 px-4 d-flex justify-content-between align-items-center">
          <div className="d-flex align-items-center">
            <div className="rounded-circle bg-primary p-1 me-2"></div>
            <span className="fw-bold text-dark text-uppercase small">今日概览</span>
            <span className="text-muted smaller ms-2">{tz} 时间</span>
          </div>
          <div className="text-muted smaller">
            <i className="ri-pulse-line me-1 text-primary"></i> 实时监控
          </div>
        </div>
        <div className="card-body p-4">
          <div className="row text-center">
            <div className="col-md-4 border-end">
              <h6 className="text-muted mb-2 small fw-semibold text-uppercase">总请求数</h6>
              <h2 className="fw-bold text-dark">{stats.requests_today}</h2>
            </div>
            <div className="col-md-4 border-end">
              <h6 className="text-muted mb-2 small fw-semibold text-uppercase">Token 消耗</h6>
              <h2 className="fw-bold text-dark">{stats.tokens_today}</h2>
              <div className="small text-muted font-monospace mt-1">
                <span className="me-2">
                  <i className="ri-arrow-up-line text-success"></i> {stats.input_tokens_today}
                </span>
                <span>
                  <i className="ri-arrow-down-line text-primary"></i> {stats.output_tokens_today}
                </span>
              </div>
            </div>
            <div className="col-md-4">
              <h6 className="text-muted mb-2 small fw-semibold text-uppercase">预估消费</h6>
              <h2 className="fw-bold text-primary">{stats.cost_today}</h2>
            </div>
          </div>
          <div className="border-top mt-4 pt-3">
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
                <div style={{ height: 260 }}>
                  <canvas ref={detailTimeLineRef}></canvas>
                </div>
              </>
            )}
          </div>
        </div>
      </div>

      <div className="row g-4">
        <div className="col-md-6">
          <div className="card h-100 border-0 shadow-sm">
            <div className="card-body">
              <h5 className="card-title fw-bold mb-3 text-dark h6">快捷操作</h5>
              <div className="d-grid gap-2">
                <Link to="/admin/channels" className="btn btn-outline-primary text-start border-light shadow-sm text-dark hover-white">
                  <i className="ri-git-merge-line me-2 text-primary"></i> 管理上游渠道
                </Link>
                <Link to="/admin/users" className="btn btn-outline-primary text-start border-light shadow-sm text-dark hover-white">
                  <i className="ri-user-settings-line me-2 text-primary"></i> 管理用户与权限
                </Link>
              </div>
            </div>
          </div>
        </div>

        <div className="col-md-6">
          <div className="card h-100 border-0 shadow-sm">
            <div className="card-body">
              <h5 className="card-title fw-bold mb-3 text-dark h6">系统信息</h5>
              <ul className="list-unstyled mb-0">
                <li className="mb-3 d-flex align-items-center">
                  <span className="text-muted small me-2">当前用户:</span>
                  <strong className="text-dark">{user?.email || '-'}</strong>
                </li>
                <li className="mb-3 d-flex align-items-center">
                  <span className="text-muted small me-2">角色权限:</span>
                  <span className="badge bg-primary bg-opacity-10 text-primary px-3 py-2 rounded-pill">{user?.role || '-'}</span>
                </li>
                <li className="d-flex align-items-center">
                  <span className="text-muted small me-2">服务状态:</span>
                  <span className="badge bg-success bg-opacity-10 text-success px-3 py-2 rounded-pill">
                    <i className="ri-checkbox-circle-fill me-1"></i> 运行中
                  </span>
                </li>
              </ul>
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}
