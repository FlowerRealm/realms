import { useEffect, useMemo, useState } from 'react';
import { Link } from 'react-router-dom';

import { listAdminTickets, type AdminTicketListItem } from '../../api/admin/tickets';

export function TicketsAdminPage({ mode }: { mode: 'all' | 'open' | 'closed' }) {
  const [items, setItems] = useState<AdminTicketListItem[]>([]);
  const [loading, setLoading] = useState(true);
  const [err, setErr] = useState('');

  useEffect(() => {
    let mounted = true;
    (async () => {
      setErr('');
      setLoading(true);
      try {
        const res = await listAdminTickets(mode);
        if (!res.success) throw new Error(res.message || '加载失败');
        if (mounted) setItems(res.data || []);
      } catch (e) {
        if (mounted) {
          setErr(e instanceof Error ? e.message : '加载失败');
          setItems([]);
        }
      } finally {
        if (mounted) setLoading(false);
      }
    })();
    return () => {
      mounted = false;
    };
  }, [mode]);

  const tabAllCls = useMemo(() => (mode === 'all' ? 'btn btn-sm btn-primary text-white border-primary' : 'btn btn-sm btn-white text-dark'), [mode]);
  const tabOpenCls = useMemo(() => (mode === 'open' ? 'btn btn-sm btn-primary text-white border-primary' : 'btn btn-sm btn-white text-dark'), [mode]);
  const tabClosedCls = useMemo(() => (mode === 'closed' ? 'btn btn-sm btn-primary text-white border-primary' : 'btn btn-sm btn-white text-dark'), [mode]);

  return (
    <div className="fade-in-up">
      <div className="d-flex justify-content-between align-items-center mb-4">
        <div>
          <h3 className="h4 fw-bold mb-0">工单管理</h3>
          <p className="text-muted small mb-0">处理用户的技术支持请求。</p>
        </div>
        <div className="card shadow-sm border-0 py-1 px-1">
          <div className="d-flex gap-1">
            <Link className={tabAllCls} to="/admin/tickets">
              全部
            </Link>
            <Link className={tabOpenCls} to="/admin/tickets/open">
              待处理
            </Link>
            <Link className={tabClosedCls} to="/admin/tickets/closed">
              已关闭
            </Link>
          </div>
        </div>
      </div>

      {err ? (
        <div className="alert alert-danger shadow-sm mb-4">
          <i className="ri-alert-line me-2"></i>
          {err}
        </div>
      ) : null}

      <div className="card shadow-sm border-0 overflow-hidden">
        {loading ? (
          <div className="text-center py-5 text-muted">加载中…</div>
        ) : items.length === 0 ? (
          <div className="text-center py-5 text-muted">
            <i className="ri-customer-service-2-line fs-1 d-block mb-3 opacity-50"></i>
            <p>暂无相关工单</p>
          </div>
        ) : (
          <div className="table-responsive">
            <table className="table table-hover align-middle mb-0">
              <thead className="table-light">
                <tr>
                  <th className="ps-4" style={{ width: 80 }}>
                    ID
                  </th>
                  <th style={{ width: 260 }}>用户</th>
                  <th>标题</th>
                  <th style={{ width: 120 }}>状态</th>
                  <th style={{ width: 160 }}>最后更新</th>
                  <th style={{ width: 100 }}></th>
                </tr>
              </thead>
              <tbody>
                {items.map((t) => (
                  <tr key={t.id}>
                    <td className="ps-4 text-muted font-monospace">#{t.id}</td>
                    <td>
                      <div className="d-flex align-items-center">
                        <div
                          className="bg-light rounded-circle p-1 me-2 text-primary d-flex align-items-center justify-content-center"
                          style={{ width: 24, height: 24 }}
                        >
                          <i className="ri-user-line rlm-icon-14"></i>
                        </div>
                        <code className="text-dark bg-transparent p-0">{t.user_email}</code>
                      </div>
                    </td>
                    <td className="fw-medium text-dark">{t.subject}</td>
                    <td>
                      <span className={`badge rounded-pill ${t.status_badge}`}>{t.status_text}</span>
                    </td>
                    <td className="text-muted small">{t.last_message_at}</td>
                    <td className="text-end pe-4">
                      <Link className="btn btn-sm btn-light border" to={`/admin/tickets/${t.id}`}>
                        <i className="ri-eye-line me-1"></i>查看
                      </Link>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}

      </div>
    </div>
  );
}
