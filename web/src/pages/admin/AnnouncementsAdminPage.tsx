import { useEffect, useMemo, useState } from 'react';

import { BootstrapModal } from '../../components/BootstrapModal';
import { closeModalById } from '../../components/modal';
import {
  createAdminAnnouncement,
  deleteAdminAnnouncement,
  listAdminAnnouncements,
  updateAdminAnnouncementStatus,
  type AdminAnnouncement,
} from '../../api/admin/announcements';

function statusBadge(status: number): { cls: string; label: string } {
  if (status === 1) return { cls: 'badge rounded-pill bg-success bg-opacity-10 text-success px-2', label: '已发布' };
  return { cls: 'badge rounded-pill bg-secondary bg-opacity-10 text-secondary px-2', label: '草稿' };
}

export function AnnouncementsAdminPage() {
  const [items, setItems] = useState<AdminAnnouncement[]>([]);
  const [loading, setLoading] = useState(true);
  const [err, setErr] = useState('');
  const [notice, setNotice] = useState('');

  const [title, setTitle] = useState('');
  const [body, setBody] = useState('');
  const [status, setStatus] = useState(1);

  const publishedCount = useMemo(() => items.filter((x) => x.status === 1).length, [items]);

  async function refresh() {
    setErr('');
    setNotice('');
    setLoading(true);
    try {
      const res = await listAdminAnnouncements();
      if (!res.success) throw new Error(res.message || '加载失败');
      setItems(res.data || []);
    } catch (e) {
      setErr(e instanceof Error ? e.message : '加载失败');
      setItems([]);
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    void refresh();
  }, []);

  return (
    <div className="fade-in-up">
      <div className="row g-4">
        <div className="col-12">
          <div className="card">
            <div className="card-body d-flex flex-column flex-md-row justify-content-between align-items-center">
              <div className="d-flex align-items-center mb-3 mb-md-0">
                <div
                  className="bg-warning bg-opacity-10 text-warning rounded-circle d-flex align-items-center justify-content-center me-3"
                  style={{ width: 48, height: 48 }}
                >
                  <span className="fs-4 material-symbols-rounded">campaign</span>
                </div>
                <div>
                  <h5 className="mb-1 fw-semibold">公告</h5>
                  <p className="mb-0 text-muted small">
                    {publishedCount} 已发布 / {items.length} 总计 · 发布后用户侧会显示未读提示
                  </p>
                </div>
              </div>

              <div className="d-flex gap-2">
                <button type="button" className="btn btn-primary btn-sm" data-bs-toggle="modal" data-bs-target="#createAnnouncementModal">
                  <span className="me-1 material-symbols-rounded">add</span> 新建公告
                </button>
              </div>
            </div>
          </div>
        </div>

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
          {loading ? (
            <div className="text-muted">加载中…</div>
          ) : items.length === 0 ? (
            <div className="text-center py-5 text-muted">
              <span className="fs-1 d-block mb-3 material-symbols-rounded">inbox</span>
              暂无公告。
            </div>
          ) : (
            <div className="card overflow-hidden">
              <div className="table-responsive">
                <table className="table table-hover align-middle mb-0">
                  <thead className="table-light">
                    <tr>
                      <th className="ps-4">标题</th>
                      <th>状态</th>
                      <th>创建时间</th>
                      <th>更新时间</th>
                      <th className="text-end pe-4">操作</th>
                    </tr>
                  </thead>
                  <tbody>
                    {items.map((a) => {
                      const st = statusBadge(a.status);
                      return (
                        <tr key={a.id}>
                          <td className="ps-4" style={{ minWidth: 0 }}>
                            <div className="fw-bold text-dark text-truncate" style={{ maxWidth: 640 }}>
                              {a.title}
                            </div>
                            <div className="text-muted small text-truncate" style={{ maxWidth: 640 }}>
                              {a.body}
                            </div>
                          </td>
                          <td>
                            <span className={st.cls}>{st.label}</span>
                          </td>
                          <td className="text-muted small text-nowrap">{a.created_at}</td>
                          <td className="text-muted small text-nowrap">{a.updated_at}</td>
                          <td className="text-end pe-4 text-nowrap">
                            <div className="d-inline-flex gap-1">
                              <button
                                className="btn btn-sm btn-light border text-primary"
                                type="button"
                                title={a.status === 1 ? '切换为草稿' : '切换为发布'}
                                onClick={async () => {
                                  setErr('');
                                  setNotice('');
                                  try {
                                    const next = a.status === 1 ? 0 : 1;
                                    const res = await updateAdminAnnouncementStatus(a.id, next);
                                    if (!res.success) throw new Error(res.message || '保存失败');
                                    setNotice(res.message || '已保存');
                                    await refresh();
                                  } catch (e) {
                                    setErr(e instanceof Error ? e.message : '保存失败');
                                  }
                                }}
                              >
                                <i className="ri-swap-line"></i>
                              </button>
                              <button
                                className="btn btn-sm btn-light border text-danger"
                                type="button"
                                title="删除"
                                onClick={async () => {
                                  if (!window.confirm('确认删除该公告？')) return;
                                  setErr('');
                                  setNotice('');
                                  try {
                                    const res = await deleteAdminAnnouncement(a.id);
                                    if (!res.success) throw new Error(res.message || '删除失败');
                                    setNotice(res.message || '已删除');
                                    await refresh();
                                  } catch (e) {
                                    setErr(e instanceof Error ? e.message : '删除失败');
                                  }
                                }}
                              >
                                <i className="ri-delete-bin-line"></i>
                              </button>
                            </div>
                          </td>
                        </tr>
                      );
                    })}
                  </tbody>
                </table>
              </div>
            </div>
          )}
        </div>
      </div>

      <BootstrapModal
        id="createAnnouncementModal"
        title="新建公告"
        dialogClassName="modal-dialog-centered modal-lg"
        onHidden={() => {
          setTitle('');
          setBody('');
          setStatus(1);
        }}
      >
        <form
          className="row g-3"
          onSubmit={async (e) => {
            e.preventDefault();
            setErr('');
            setNotice('');
            try {
              const res = await createAdminAnnouncement({ title: title.trim(), body: body.trim(), status });
              if (!res.success) throw new Error(res.message || '创建失败');
              setNotice('已创建');
              closeModalById('createAnnouncementModal');
              await refresh();
            } catch (e) {
              setErr(e instanceof Error ? e.message : '创建失败');
            }
          }}
        >
          <div className="col-md-8">
            <label className="form-label">标题</label>
            <input className="form-control" value={title} onChange={(e) => setTitle(e.target.value)} placeholder="标题（最多 200 字符）" required />
            <div className="form-text small text-muted">最多 200 字。</div>
          </div>
          <div className="col-md-4">
            <label className="form-label">状态</label>
            <select className="form-select" value={status} onChange={(e) => setStatus(Number.parseInt(e.target.value, 10) || 0)}>
              <option value={1}>发布</option>
              <option value={0}>草稿</option>
            </select>
          </div>
          <div className="col-12">
            <label className="form-label">内容</label>
            <textarea className="form-control" rows={8} value={body} onChange={(e) => setBody(e.target.value)} placeholder="正文（最多 8000 字符）" required />
            <div className="form-text small text-muted">支持换行；用户侧会按原样展示（支持 Markdown）。</div>
          </div>
          <div className="modal-footer border-top-0 px-0 pb-0">
            <button type="button" className="btn btn-light" data-bs-dismiss="modal">
              取消
            </button>
            <button className="btn btn-primary px-4" type="submit" disabled={loading || !title.trim() || !body.trim()}>
              提交
            </button>
          </div>
        </form>
      </BootstrapModal>
    </div>
  );
}
