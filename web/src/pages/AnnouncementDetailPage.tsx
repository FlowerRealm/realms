import { useEffect, useMemo, useState } from 'react';
import { useParams } from 'react-router-dom';

import { getAnnouncement, type AnnouncementDetail } from '../api/announcements';

export function AnnouncementDetailPage() {
  const params = useParams();

  const announcementID = useMemo(() => {
    const raw = (params.id || '').toString().trim();
    const n = Number.parseInt(raw, 10);
    if (!Number.isFinite(n) || n <= 0) return null;
    return n;
  }, [params.id]);

  const [data, setData] = useState<AnnouncementDetail | null>(null);
  const [loading, setLoading] = useState(true);
  const [err, setErr] = useState('');

  useEffect(() => {
    let mounted = true;
    (async () => {
      setErr('');
      setLoading(true);
      try {
        if (!announcementID) throw new Error('公告 ID 不合法');
        const res = await getAnnouncement(announcementID);
        if (!res.success || !res.data) {
          throw new Error(res.message || '加载失败');
        }
        if (mounted) setData(res.data);
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
  }, [announcementID]);

  return (
    <div className="fade-in-up">
      <div className="d-flex justify-content-between align-items-center mb-4">
        <div>
          <h2 className="h4 fw-bold mb-0">{data?.title || (loading ? '加载中…' : '公告')}</h2>
          <p className="text-muted small mb-0">{data?.created_at || ''}</p>
        </div>
      </div>

      {err ? (
        <div className="alert alert-danger d-flex align-items-center" role="alert">
          <span className="me-2 material-symbols-rounded">warning</span>
          <div>{err}</div>
        </div>
      ) : null}

      <div className="card overflow-hidden">
        <div className="card-body">
          {loading ? <div className="text-muted small">加载中…</div> : <div style={{ whiteSpace: 'pre-wrap' }}>{data?.body || ''}</div>}
        </div>
      </div>
    </div>
  );
}
