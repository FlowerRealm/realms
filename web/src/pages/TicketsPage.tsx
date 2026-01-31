import { useEffect, useMemo, useState } from 'react';
import { Link } from 'react-router-dom';

import { listTickets, type TicketListItem } from '../api/tickets';

export function TicketsPage({ mode }: { mode: 'all' | 'open' | 'closed' }) {
  const [items, setItems] = useState<TicketListItem[]>([]);
  const [loading, setLoading] = useState(true);
  const [err, setErr] = useState('');

  useEffect(() => {
    let mounted = true;
    (async () => {
      setErr('');
      setLoading(true);
      try {
        const res = await listTickets(mode);
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

  const tabAllCls = useMemo(() => (mode === 'all' ? 'btn btn-sm btn-primary text-white border-primary' : 'btn btn-sm btn-white border text-dark'), [mode]);
  const tabOpenCls = useMemo(() => (mode === 'open' ? 'btn btn-sm btn-primary text-white border-primary' : 'btn btn-sm btn-white border text-dark'), [mode]);
  const tabClosedCls = useMemo(() => (mode === 'closed' ? 'btn btn-sm btn-primary text-white border-primary' : 'btn btn-sm btn-white border text-dark'), [mode]);

  return (
    <div className="fade-in-up">
      <div className="d-flex justify-content-between align-items-center mb-4">
        <div>
          <h3 className="mb-1 fw-bold">工单</h3>
          <div className="text-muted small">查看与管理您的所有工单记录。</div>
        </div>
        <Link className="btn btn-primary" to="/tickets/new">
          <span className="me-1 material-symbols-rounded">add</span> 创建工单
        </Link>
      </div>

      {err ? (
        <div className="alert alert-danger">
          <span className="me-2 material-symbols-rounded">warning</span>
          {err}
        </div>
      ) : null}

      <div className="card border-0 overflow-hidden">
        <div className="card-body p-0">
          <div className="p-3 border-bottom d-flex gap-2 bg-light rounded-top">
            <Link className={tabAllCls} to="/tickets">
              全部
            </Link>
            <Link className={tabOpenCls} to="/tickets/open">
              进行中
            </Link>
            <Link className={tabClosedCls} to="/tickets/closed">
              已关闭
            </Link>
          </div>

          {loading ? (
            <div className="text-center py-5 text-muted">加载中…</div>
          ) : items.length === 0 ? (
            <div className="text-center py-5 text-muted">
              <span className="fs-1 d-block mb-3 opacity-50 material-symbols-rounded">inbox</span>
              <p>暂无工单记录</p>
              <Link to="/tickets/new" className="btn btn-sm btn-outline-primary">
                创建一个新工单
              </Link>
            </div>
          ) : (
            <div className="table-responsive">
              <table className="table table-hover align-middle mb-0">
                <thead className="table-light">
                  <tr>
                    <th className="ps-4" style={{ width: 80 }}>
                      ID
                    </th>
                    <th>标题</th>
                    <th style={{ width: 120 }}>状态</th>
                    <th style={{ width: 180 }}>最后更新</th>
                    <th style={{ width: 180 }}>创建时间</th>
                    <th style={{ width: 100 }}></th>
                  </tr>
                </thead>
                <tbody>
                  {items.map((t) => (
                    <tr key={t.id}>
                      <td className="ps-4 text-muted font-monospace">#{t.id}</td>
                      <td className="fw-medium text-dark">{t.subject}</td>
                      <td>
                        <span className={`badge rounded-pill ${t.status_badge}`}>{t.status_text}</span>
                      </td>
                      <td className="text-muted small">{t.last_message_at}</td>
                      <td className="text-muted small">{t.created_at}</td>
                      <td className="text-end pe-4">
                        <Link className="btn btn-sm btn-light border" to={`/tickets/${t.id}`}>
                          查看
                        </Link>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </div>
        <div className="card-footer bg-white small text-muted border-top-0 py-3">
          <span className="me-1 material-symbols-rounded">info</span>系统会自动清理超过 7 天的附件，请及时下载保存。
        </div>
      </div>
    </div>
  );
}
