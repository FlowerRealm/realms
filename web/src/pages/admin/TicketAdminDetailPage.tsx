import { useEffect, useMemo, useState } from 'react';
import { useParams } from 'react-router-dom';

import { closeAdminTicket, getAdminTicketDetail, reopenAdminTicket, replyAdminTicket, type AdminTicketDetailResponse, type AdminTicketMessage } from '../../api/admin/tickets';

function isUserMessage(m: AdminTicketMessage): boolean {
  const actor = (m.actor || '').trim();
  return actor === '用户';
}

export function TicketAdminDetailPage() {
  const params = useParams();
  const ticketId = Number.parseInt((params.id || '').toString(), 10);

  const [data, setData] = useState<AdminTicketDetailResponse | null>(null);
  const [loading, setLoading] = useState(true);
  const [err, setErr] = useState('');

  const [replyBody, setReplyBody] = useState('');
  const [replyFiles, setReplyFiles] = useState<File[]>([]);
  const [replying, setReplying] = useState(false);

  async function refresh() {
    setErr('');
    setLoading(true);
    try {
      if (!Number.isFinite(ticketId) || ticketId <= 0) throw new Error('参数错误');
      const res = await getAdminTicketDetail(ticketId);
      if (!res.success) throw new Error(res.message || '加载失败');
      setData(res.data || null);
    } catch (e) {
      setErr(e instanceof Error ? e.message : '加载失败');
      setData(null);
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    void refresh();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [ticketId]);

  const ticket = data?.ticket;
  const messages = data?.messages || [];

  const badgeCls = useMemo(() => `badge rounded-pill ${ticket?.status_badge || 'bg-secondary bg-opacity-10 text-secondary'}`, [ticket?.status_badge]);

  return (
    <div className="fade-in-up">
      <div className="d-flex justify-content-between align-items-center mb-4">
        <div>
          <div className="d-flex align-items-center gap-2 mb-1">
            <h3 className="h4 fw-bold mb-0">工单 #{ticketId || '-'}</h3>
            {ticket ? <span className={badgeCls}>{ticket.status_text}</span> : null}
          </div>
          {ticket ? (
            <div className="text-muted small">
              <span className="text-dark fw-medium">{ticket.user_email}</span>
              <span className="mx-1">|</span>
              <span>{ticket.created_at}</span>
            </div>
          ) : null}
        </div>
        <div className="d-flex gap-2">
          {ticket ? (
            ticket.closed ? (
              <button
                className="btn btn-sm btn-primary shadow-sm"
                type="button"
                onClick={async () => {
                  if (!window.confirm('确认恢复该工单？')) return;
                  setErr('');
                  try {
                    const res = await reopenAdminTicket(ticketId);
                    if (!res.success) throw new Error(res.message || '恢复失败');
                    await refresh();
                  } catch (e) {
                    setErr(e instanceof Error ? e.message : '恢复失败');
                  }
                }}
              >
                <i className="ri-refresh-line me-1"></i>恢复工单
              </button>
            ) : (
              <button
                className="btn btn-sm btn-outline-danger shadow-sm border bg-white"
                type="button"
                onClick={async () => {
                  if (!window.confirm('确认关闭该工单？')) return;
                  setErr('');
                  try {
                    const res = await closeAdminTicket(ticketId);
                    if (!res.success) throw new Error(res.message || '关闭失败');
                    await refresh();
                  } catch (e) {
                    setErr(e instanceof Error ? e.message : '关闭失败');
                  }
                }}
              >
                <i className="ri-lock-line me-1"></i>关闭工单
              </button>
            )
          ) : null}
        </div>
      </div>

      {err ? (
        <div className="alert alert-danger shadow-sm mb-4">
          <i className="ri-alert-line me-2"></i>
          {err}
        </div>
      ) : null}

      {loading ? (
        <div className="text-muted">加载中…</div>
      ) : (
        <>
          <div className="d-flex flex-column gap-3 mb-4">
            {messages.map((m) =>
              isUserMessage(m) ? (
                <div key={m.id} className="d-flex justify-content-start">
                  <div className="d-flex flex-column align-items-start" style={{ maxWidth: '85%' }}>
                    <div className="d-flex align-items-center gap-2 mb-1 small text-muted">
                      <span className="fw-medium">{m.actor}</span>
                      <span>{m.actor_meta}</span>
                      <span>{m.created_at}</span>
                    </div>
                    <div className="card border shadow-sm bg-white">
                      <div className="card-body py-2 px-3">
                        <div style={{ whiteSpace: 'pre-wrap' }}>{m.body}</div>
                        {m.attachments?.length ? (
                          <div className="mt-2 pt-2 border-top">
                            <div className="small text-muted mb-1">附件</div>
                            <div className="d-flex flex-wrap gap-2">
                              {m.attachments.map((a) => (
                                <a key={a.id} href={a.url} className="badge bg-light text-dark border text-decoration-none d-flex align-items-center p-2">
                                  <i className="ri-attachment-2 me-1"></i>
                                  <span className="text-truncate" style={{ maxWidth: 150 }}>
                                    {a.name}
                                  </span>
                                  <span className="ms-1 opacity-50">({a.size})</span>
                                </a>
                              ))}
                            </div>
                          </div>
                        ) : null}
                      </div>
                    </div>
                  </div>
                </div>
              ) : (
                <div key={m.id} className="d-flex justify-content-end">
                  <div className="d-flex flex-column align-items-end" style={{ maxWidth: '85%' }}>
                    <div className="d-flex align-items-center gap-2 mb-1 small text-muted">
                      <span>{m.created_at}</span>
                      <span className="fw-medium">{m.actor}</span>
                      <span>{m.actor_meta}</span>
                    </div>
                    <div className="card border-0 shadow-sm bg-primary bg-opacity-10 text-dark">
                      <div className="card-body py-2 px-3">
                        <div style={{ whiteSpace: 'pre-wrap' }}>{m.body}</div>
                        {m.attachments?.length ? (
                          <div className="mt-2 pt-2 border-top border-primary border-opacity-10">
                            <div className="small text-muted mb-1">附件</div>
                            <div className="d-flex flex-wrap gap-2">
                              {m.attachments.map((a) => (
                                <a key={a.id} href={a.url} className="badge bg-white text-primary border text-decoration-none d-flex align-items-center p-2 shadow-sm">
                                  <i className="ri-attachment-2 me-1"></i>
                                  <span className="text-truncate" style={{ maxWidth: 150 }}>
                                    {a.name}
                                  </span>
                                  <span className="ms-1 opacity-50">({a.size})</span>
                                </a>
                              ))}
                            </div>
                          </div>
                        ) : null}
                      </div>
                    </div>
                  </div>
                </div>
              ),
            )}
          </div>

          <div className="card shadow-sm border-0 bg-light">
            <div className="card-body">
              {ticket?.can_reply ? (
                <>
                  <h5 className="mb-3 fw-bold h6">回复用户</h5>
                  <form
                    onSubmit={async (e) => {
                      e.preventDefault();
                      setErr('');
                      setReplying(true);
                      try {
                        const res = await replyAdminTicket(ticketId, replyBody.trim(), replyFiles);
                        if (!res.success) throw new Error(res.message || '回复失败');
                        setReplyBody('');
                        setReplyFiles([]);
                        await refresh();
                      } catch (e) {
                        setErr(e instanceof Error ? e.message : '回复失败');
                      } finally {
                        setReplying(false);
                      }
                    }}
                  >
                    <div className="mb-3">
                      <textarea
                        className="form-control shadow-sm"
                        rows={4}
                        placeholder="请输入回复内容..."
                        required
                        value={replyBody}
                        onChange={(e) => setReplyBody(e.target.value)}
                      ></textarea>
                    </div>
                    <div className="d-flex justify-content-between align-items-center flex-wrap gap-2">
                      <div className="flex-grow-1" style={{ maxWidth: 400 }}>
                        <input className="form-control form-control-sm" type="file" multiple onChange={(e) => setReplyFiles(Array.from(e.target.files || []))} />
                        <div className="form-text small mt-0">支持多文件 (Max 100MB)</div>
                      </div>
                      <button className="btn btn-primary shadow-sm px-4" type="submit" disabled={replying}>
                        <i className="ri-send-plane-fill me-1"></i>
                        {replying ? '发送中…' : '发送回复'}
                      </button>
                    </div>
                  </form>
                </>
              ) : (
                <div className="d-flex align-items-center justify-content-center text-muted py-2">
                  <i className="ri-lock-fill me-2"></i> 工单已关闭。
                </div>
              )}
            </div>
          </div>
        </>
      )}
    </div>
  );
}
