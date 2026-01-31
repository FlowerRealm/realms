import { useEffect, useState } from 'react';
import { Link } from 'react-router-dom';

import { listAnnouncements, type AnnouncementListItem } from '../api/announcements';

export function AnnouncementsPage() {
  const [items, setItems] = useState<AnnouncementListItem[]>([]);
  const [unreadCount, setUnreadCount] = useState(0);
  const [loading, setLoading] = useState(true);
  const [err, setErr] = useState('');

  useEffect(() => {
    (async () => {
      setErr('');
      setLoading(true);
      try {
        const res = await listAnnouncements(200);
        if (!res.success || !res.data) {
          throw new Error(res.message || '加载失败');
        }
        setItems(res.data.items || []);
        setUnreadCount(res.data.unread_count || 0);
      } catch (e) {
        setErr(e instanceof Error ? e.message : '加载失败');
        setItems([]);
        setUnreadCount(0);
      } finally {
        setLoading(false);
      }
    })();
  }, []);

  return (
    <div className="fade-in-up">
      <div className="d-flex justify-content-between align-items-center mb-4">
        <div>
          <h2 className="h4 fw-bold mb-0">公告</h2>
          <p className="text-muted small mb-0">
            {unreadCount > 0 ? (
              <>
                你有 <span className="badge bg-warning text-dark">{unreadCount}</span> 条未读公告
              </>
            ) : (
              <>暂无未读公告</>
            )}
          </p>
        </div>
      </div>

      {err ? (
        <div className="alert alert-danger d-flex align-items-center" role="alert">
          <span className="me-2 material-symbols-rounded">warning</span>
          <div>{err}</div>
        </div>
      ) : null}

      <div className="card overflow-hidden">
        <div className="table-responsive">
          <table className="table table-hover align-middle mb-0">
            <thead className="bg-light">
              <tr>
                <th className="ps-4">标题</th>
                <th>时间</th>
                <th style={{ width: 120 }}>状态</th>
              </tr>
            </thead>
            <tbody>
              {loading ? (
                <tr>
                  <td colSpan={3} className="text-muted small">
                    加载中…
                  </td>
                </tr>
              ) : items.length > 0 ? (
                items.map((a) => (
                  <tr key={a.id}>
                    <td className="ps-4">
                      <Link className="text-decoration-none fw-semibold" to={`/announcements/${a.id}`}>
                        {a.title}
                      </Link>
                    </td>
                    <td className="small text-muted">{a.created_at}</td>
                    <td>
                      {a.read ? (
                        <span className="badge rounded-pill bg-light text-secondary border px-2">已读</span>
                      ) : (
                        <span className="badge rounded-pill bg-warning text-dark px-2">未读</span>
                      )}
                    </td>
                  </tr>
                ))
              ) : (
                <tr>
                  <td colSpan={3} className="text-muted small">
                    暂无公告
                  </td>
                </tr>
              )}
            </tbody>
          </table>
        </div>
      </div>
    </div>
  );
}

