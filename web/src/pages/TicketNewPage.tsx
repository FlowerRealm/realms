import { useMemo, useState } from 'react';
import { Link, useNavigate } from 'react-router-dom';

import { createTicket } from '../api/tickets';

export function TicketNewPage() {
  const navigate = useNavigate();

  const [subject, setSubject] = useState('');
  const [body, setBody] = useState('');
  const [files, setFiles] = useState<File[]>([]);
  const [submitting, setSubmitting] = useState(false);
  const [err, setErr] = useState('');

  const totalSizeMB = useMemo(() => {
    const total = files.reduce((acc, f) => acc + (f.size || 0), 0);
    return total / (1024 * 1024);
  }, [files]);

  return (
    <div className="fade-in-up">
      <div className="mb-4">
        <h3 className="mb-1 fw-bold">创建工单</h3>
        <div className="text-muted small">提交问题或反馈，我们会尽快处理。请勿上传敏感信息（如完整 Key/Token）。</div>
      </div>

      {err ? (
        <div className="alert alert-danger">
          <span className="me-2 material-symbols-rounded">warning</span>
          {err}
        </div>
      ) : null}

      <div className="card border-0">
        <div className="card-body p-4">
          <form
            onSubmit={async (e) => {
              e.preventDefault();
              setErr('');
              setSubmitting(true);
              try {
                const res = await createTicket(subject.trim(), body.trim(), files);
                if (!res.success) throw new Error(res.message || '提交失败');
                const id = res.data?.ticket_id;
                if (!id) throw new Error('提交失败：缺少 ticket_id');
                navigate(`/tickets/${id}`);
              } catch (e) {
                setErr(e instanceof Error ? e.message : '提交失败');
              } finally {
                setSubmitting(false);
              }
            }}
          >
            <div className="mb-3">
              <label className="form-label fw-medium">标题</label>
              <input
                className="form-control"
                maxLength={200}
                placeholder="简要描述问题 (例如: API 调用 500 错误)"
                required
                value={subject}
                onChange={(e) => setSubject(e.target.value)}
              />
            </div>

            <div className="mb-3">
              <label className="form-label fw-medium">问题描述</label>
              <textarea
                className="form-control"
                rows={8}
                placeholder={`详细描述问题发生的过程、期望结果与实际结果。\n如有报错信息，请完整粘贴。`}
                required
                value={body}
                onChange={(e) => setBody(e.target.value)}
              ></textarea>
            </div>

            <div className="mb-4">
              <label className="form-label fw-medium">附件 (可选)</label>
              <input
                className="form-control"
                type="file"
                multiple
                onChange={(e) => {
                  const list = Array.from(e.target.files || []);
                  setFiles(list);
                }}
              />
              <div className="form-text">
                支持图片或日志文件，单次总大小不超过 100MB。{files.length ? `（当前 ${files.length} 个文件，约 ${totalSizeMB.toFixed(1)} MB）` : ''}
              </div>
            </div>

            <div className="d-flex gap-2 align-items-center border-top pt-3">
              <button className="btn btn-primary px-4" type="submit" disabled={submitting}>
                <span className="me-1 material-symbols-rounded">send</span> {submitting ? '提交中…' : '提交工单'}
              </button>
              <Link className="btn btn-link text-decoration-none text-muted" to="/tickets">
                取消
              </Link>
            </div>
          </form>
        </div>
      </div>
    </div>
  );
}

