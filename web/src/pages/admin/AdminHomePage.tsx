import { useEffect, useState } from 'react';
import { Link } from 'react-router-dom';

import { useAuth } from '../../auth/AuthContext';
import { getAdminHome, type AdminHome } from '../../api/admin/home';

export function AdminHomePage() {
  const { user } = useAuth();
  const [data, setData] = useState<AdminHome | null>(null);
  const [err, setErr] = useState('');

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

