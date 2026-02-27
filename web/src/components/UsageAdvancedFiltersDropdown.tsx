import { forwardRef, useImperativeHandle, useRef, useState } from 'react';

import { useAnchoredPopover } from '../hooks/useAnchoredPopover';

export type UsageAdvancedFiltersDropdownHandle = {
  close: () => void;
};

export type UsageAdvancedFilterField = {
  inputId: string;
  label: string;
  title?: string;
  placeholder?: string;
  value: string;
  onChange: (value: string) => void;
};

export const UsageAdvancedFiltersDropdown = forwardRef<UsageAdvancedFiltersDropdownHandle, {
  disabled?: boolean;
  toggleTestId: string;
  fields: UsageAdvancedFilterField[];
}>(function UsageAdvancedFiltersDropdown({ disabled, toggleTestId, fields }, ref) {
  const [open, setOpen] = useState(false);
  const btnRef = useRef<HTMLButtonElement | null>(null);
  const panelRef = useRef<HTMLDivElement | null>(null);
  const panelStyle = useAnchoredPopover({
    open,
    onClose: () => setOpen(false),
    triggerRef: btnRef,
    panelRef,
  });

  useImperativeHandle(
    ref,
    () => ({
      close: () => setOpen(false),
    }),
    [],
  );

  return (
    <div className="position-relative">
      <button
        ref={btnRef}
        type="button"
        className={`btn btn-sm ${open ? 'btn-primary' : 'btn-outline-secondary'}`}
        onClick={() => setOpen((v) => !v)}
        disabled={!!disabled}
        data-testid={toggleTestId}
      >
        <span className="material-symbols-rounded me-1">tune</span>
        高级筛选
      </button>

      {open ? (
        <div ref={panelRef} className="rlm-usage-filter-dropdown card shadow-sm" style={panelStyle}>
          <div className="card-body p-2 rlm-usage-filter-panel">
            <div className="rlm-usage-filter-row">
              {fields.map((f) => (
                <div key={f.inputId} className="rlm-usage-filter-item">
                  <div className="input-group input-group-sm">
                    <span className="input-group-text rlm-usage-filter-prefix">
                      <span className="form-label mb-0 smaller text-muted text-truncate" title={f.title || f.label}>
                        {f.label}
                      </span>
                    </span>
                    <input
                      id={f.inputId}
                      type="text"
                      className="form-control"
                      placeholder={f.placeholder}
                      value={f.value}
                      onChange={(e) => f.onChange(e.target.value || '')}
                      disabled={!!disabled}
                    />
                  </div>
                </div>
              ))}
            </div>

            <div className="d-flex justify-content-between align-items-center mt-2">
              <div className="text-muted smaller">多个条件同时启用时，按交集过滤（AND）。</div>
              <button type="button" className="btn btn-link btn-sm p-0" onClick={() => setOpen(false)}>
                收起
              </button>
            </div>
          </div>
        </div>
      ) : null}
    </div>
  );
});

