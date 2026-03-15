import { useId, useState } from 'react';

import { getUserRedemptionModePrompt, redeemUserCode, type RedemptionApplyMode, type RedemptionKind } from '../api/redemptionCodes';
import { toErrorMessage } from '../api/redemptionSupport';
import { BootstrapModal } from './BootstrapModal';
import { closeModalById, showModalById } from './modal';

type RedemptionCodeCardProps = {
  kind: RedemptionKind;
  buttonLabel?: string;
  buttonClassName?: string;
  submitLabel?: string;
  onRedeemed?: (message: string) => Promise<void> | void;
};

const MODE_LABELS: Record<RedemptionApplyMode, { title: string; body: string; icon: string }> = {
  parallel: {
    title: '并行生效',
    body: '立即与当前同套餐并行生效。',
    icon: 'layers',
  },
  sequential: {
    title: '顺延生效',
    body: '等当前同套餐结束后再生效。',
    icon: 'schedule',
  },
};

export function RedemptionCodeCard({ kind, buttonLabel, buttonClassName, submitLabel, onRedeemed }: RedemptionCodeCardProps) {
  const modalId = useId().replace(/:/g, '');

  const [code, setCode] = useState('');
  const [pending, setPending] = useState(false);
  const [err, setErr] = useState('');
  const [modePrompt, setModePrompt] = useState<{
    title: string;
    description: string;
    options: RedemptionApplyMode[];
    selected: RedemptionApplyMode;
    reason?: string;
  } | null>(null);

  const normalizedCode = code.trim();
  const isDisabled = pending || !normalizedCode;
  const modalTitle = kind === 'subscription' ? '使用订阅兑换码' : '使用充值兑换码';
  const placeholder = kind === 'subscription' ? '输入订阅兑换码' : '输入充值兑换码';
  const actionLabel = submitLabel || '确认兑换';

  function openRedeemModal() {
    setErr('');
    setModePrompt(null);
    showModalById(modalId);
  }

  async function runRedeem(mode?: RedemptionApplyMode) {
    setPending(true);
    setErr('');
    try {
      const resp = await redeemUserCode({ kind, code: normalizedCode, applyMode: mode });
      const prompt = getUserRedemptionModePrompt(resp);
      if (!resp.success && prompt) {
        setModePrompt({
          title: prompt.title,
          description: prompt.description,
          options: prompt.options,
          selected: prompt.defaultMode,
          reason: prompt.reason,
        });
        return;
      }
      if (!resp.success) throw new Error(resp.message || '兑换失败');

      const message = resp.message || '兑换成功';
      setCode('');
      setModePrompt(null);
      closeModalById(modalId);
      await onRedeemed?.(message);
    } catch (error) {
      setErr(toErrorMessage(error, '兑换失败'));
    } finally {
      setPending(false);
    }
  }

  return (
    <>
      <button type="button" className={buttonClassName || 'btn btn-outline-primary btn-sm'} onClick={openRedeemModal}>
        {buttonLabel || '使用兑换码'}
      </button>

      <BootstrapModal
        id={modalId}
        title={modePrompt?.title || modalTitle}
        dialogClassName="modal-dialog-centered"
        onHidden={() => {
          setErr('');
          setModePrompt(null);
        }}
      >
        {modePrompt ? (
          <div className="d-flex flex-column gap-3">
            <div className="text-muted small">{modePrompt.description}</div>
            {err ? (
              <div className="alert alert-danger d-flex align-items-start mb-0" role="alert">
                <span className="me-2 material-symbols-rounded">warning</span>
                <div>{err}</div>
              </div>
            ) : null}
            {modePrompt.reason ? (
              <div className="alert alert-warning d-flex align-items-start mb-0" role="alert">
                <span className="me-2 material-symbols-rounded">info</span>
                <div>{modePrompt.reason}</div>
              </div>
            ) : null}
            <div className="row g-3">
              {modePrompt.options.map((option) => {
                const meta = MODE_LABELS[option];
                const selected = modePrompt.selected === option;
                return (
                  <div key={option} className="col-md-6">
                    <button
                      type="button"
                      className={`btn w-100 text-start border h-100 ${selected ? 'btn-primary' : 'btn-light'}`}
                      onClick={() => setModePrompt((prev) => (prev ? { ...prev, selected: option } : prev))}
                    >
                      <div className="d-flex align-items-start gap-3">
                        <span className="material-symbols-rounded">{meta.icon}</span>
                        <span>
                          <span className="d-block fw-semibold">{meta.title}</span>
                          <span className={`d-block small ${selected ? 'text-white text-opacity-75' : 'text-muted'}`}>{meta.body}</span>
                        </span>
                      </div>
                    </button>
                  </div>
                );
              })}
            </div>
            <div className="modal-footer border-top-0 px-0 pb-0">
              <button type="button" className="btn btn-light" data-bs-dismiss="modal" disabled={pending}>
                取消
              </button>
              <button
                type="button"
                className="btn btn-primary px-4"
                disabled={pending || !modePrompt}
                onClick={() => {
                  if (!modePrompt) return;
                  void runRedeem(modePrompt.selected);
                }}
              >
                {pending ? '处理中…' : '确认并兑换'}
              </button>
            </div>
          </div>
        ) : (
          <form
            className="d-flex flex-column gap-3"
            onSubmit={(event) => {
              event.preventDefault();
              if (isDisabled) return;
              void runRedeem();
            }}
          >
            {err ? (
              <div className="alert alert-danger d-flex align-items-start mb-0" role="alert">
                <span className="me-2 material-symbols-rounded">warning</span>
                <div>{err}</div>
              </div>
            ) : null}
            <div>
              <label className="form-label">兑换码</label>
              <input
                className="form-control font-monospace"
                value={code}
                onChange={(event) => setCode(event.target.value.toUpperCase())}
                placeholder={placeholder}
                autoComplete="off"
                spellCheck={false}
              />
            </div>
            <div className="modal-footer border-top-0 px-0 pb-0">
              <button type="button" className="btn btn-light" data-bs-dismiss="modal" disabled={pending}>
                取消
              </button>
              <button type="submit" className="btn btn-primary px-4" disabled={isDisabled}>
                {pending ? '提交中…' : actionLabel}
              </button>
            </div>
          </form>
        )}
      </BootstrapModal>
    </>
  );
}
