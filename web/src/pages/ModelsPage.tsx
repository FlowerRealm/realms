import { useEffect, useState } from 'react';

import { listUserModelsDetail, type UserManagedModel } from '../api/models';
import { formatUSDPlain } from '../format/money';

export function ModelsPage() {
  const [models, setModels] = useState<UserManagedModel[]>([]);
  const [loading, setLoading] = useState(true);
  const [err, setErr] = useState('');

  useEffect(() => {
    (async () => {
      setErr('');
      setLoading(true);
      try {
        const res = await listUserModelsDetail();
        if (!res.success) {
          throw new Error(res.message || '加载失败');
        }
        setModels(res.data || []);
      } catch (e) {
        setErr(e instanceof Error ? e.message : '加载失败');
      } finally {
        setLoading(false);
      }
    })();
  }, []);

  return (
    <div className="fade-in-up">
      <div className="card overflow-hidden">
        <div className="card-header">
          <div>
            <h5 className="mb-0">
              <span className="me-2 material-symbols-rounded">smart_toy</span>可用模型列表
            </h5>
            <small className="text-muted">由管理员维护（名单外模型将被拒绝）</small>
          </div>
        </div>

        <div className="card-body p-0">
          {err ? (
            <div className="alert alert-danger m-3" role="alert">
              <span className="me-2 material-symbols-rounded">report</span> {err}
            </div>
          ) : null}

          {loading ? (
            <div className="text-center py-5 text-muted">加载中…</div>
          ) : models.length === 0 ? (
            <div className="text-center py-5 text-muted">
              <span className="fs-1 d-block mb-3 material-symbols-rounded">inbox</span>
              暂无可用模型，请联系管理员配置模型目录。
            </div>
          ) : (
            <div className="table-responsive">
              <table className="table table-hover align-middle mb-0">
                <thead className="table-light">
                  <tr>
                    <th className="ps-4">模型 ID</th>
                    <th>归属方</th>
                    <th>
                      计费 <span className="text-muted small">（每 1M Token）</span>
                    </th>
                    <th className="text-end pe-4">状态</th>
                  </tr>
                </thead>
                <tbody>
                  {models.map((m) => (
                    <tr key={m.public_id}>
                      <td className="ps-4">
                        <div className="d-flex align-items-center gap-2">
                          {m.icon_url ? (
                            <img
                              className="rlm-model-icon"
                              src={m.icon_url}
                              alt={m.owned_by || 'realms'}
                              title={m.owned_by || 'realms'}
                              loading="lazy"
                              onError={(e) => {
                                (e.currentTarget as HTMLImageElement).style.display = 'none';
                              }}
                            />
                          ) : null}
                          <span className="font-monospace fw-medium text-primary">{m.public_id}</span>
                        </div>
                      </td>
                      <td>
                        {m.owned_by ? (
                          <span className="badge bg-light text-dark border">{m.owned_by}</span>
                        ) : (
                          <span className="text-muted small">-</span>
                        )}
                      </td>
                      <td className="text-muted small">
                        <div className="d-flex flex-column gap-1">
                          <div>
                            <span className="text-muted">输入</span> <span className="me-1 material-symbols-rounded">attach_money</span>
                            {formatUSDPlain(m.input_usd_per_1m)}
                          </div>
                          <div>
                            <span className="text-muted">输出</span> <span className="me-1 material-symbols-rounded">attach_money</span>
                            {formatUSDPlain(m.output_usd_per_1m)}
                          </div>
                          <div>
                            <span className="text-muted">缓存输入</span> <span className="me-1 material-symbols-rounded">attach_money</span>
                            {formatUSDPlain(m.cache_input_usd_per_1m)}
                          </div>
                          <div>
                            <span className="text-muted">缓存输出</span> <span className="me-1 material-symbols-rounded">attach_money</span>
                            {formatUSDPlain(m.cache_output_usd_per_1m)}
                          </div>
                        </div>
                      </td>
                      <td className="text-end pe-4">
                        <span className="badge bg-success-subtle text-success border border-success-subtle">可用</span>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </div>
      </div>
    </div>
  );
}
