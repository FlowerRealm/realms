import { SegmentedFrame } from '../components/SegmentedFrame';

export function UsagePage() {
  return (
    <div className="fade-in-up">
      <SegmentedFrame>
        <div>
          <div className="d-flex justify-content-between align-items-center mb-3">
            <div>
              <h3 className="mb-1 fw-bold">用量统计</h3>
              <div className="text-muted small">单个 API 令牌的用量查询已移至“API 令牌”页面。</div>
            </div>
          </div>

          <div className="alert alert-info mb-0 d-flex align-items-start" role="alert">
            <span className="me-2 mt-1 material-symbols-rounded">info</span>
            <div className="small">
              请前往 <a href="/tokens">API 令牌</a>，在对应令牌行点击“用量”进行查询。
            </div>
          </div>
        </div>
      </SegmentedFrame>
    </div>
  );
}
