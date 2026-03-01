import type { AutoSaveStatus } from '../hooks/useAutoSave';

export function AutoSaveIndicator(props: {
  status: AutoSaveStatus;
  blockedReason?: string;
  error?: string;
  onRetry?: () => void;
  className?: string;
}) {
  const { status, blockedReason, error, onRetry, className } = props;

  if (status === 'idle') return null;

  const cls = (className || '').trim() || 'small';

  if (status === 'saving') {
    return (
      <div className={cls}>
        <span className="material-symbols-rounded align-middle me-1">sync</span>
        <span className="text-muted align-middle">保存中…</span>
      </div>
    );
  }

  if (status === 'saved') {
    return (
      <div className={cls}>
        <span className="material-symbols-rounded align-middle me-1 text-success">check_circle</span>
        <span className="text-muted align-middle">已自动保存</span>
      </div>
    );
  }

  if (status === 'blocked') {
    return (
      <div className={cls}>
        <span className="material-symbols-rounded align-middle me-1 text-warning">report</span>
        <span className="text-muted align-middle">{(blockedReason || '').trim() || '内容不完整，未保存'}</span>
      </div>
    );
  }

  if (status === 'error') {
    return (
      <div className={cls}>
        <span className="material-symbols-rounded align-middle me-1 text-danger">error</span>
        <span className="text-muted align-middle">{(error || '').trim() || '保存失败'}</span>
        {onRetry ? (
          <button type="button" className="btn btn-link btn-sm align-baseline ps-2 pe-0" onClick={onRetry}>
            重试
          </button>
        ) : null}
      </div>
    );
  }

  // dirty
  return (
    <div className={cls}>
      <span className="material-symbols-rounded align-middle me-1 text-secondary">edit</span>
      <span className="text-muted align-middle">待保存…</span>
    </div>
  );
}

