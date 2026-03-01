import { useEffect, useMemo, useRef, useState } from 'react';

import { getAdminUsageModelSuggest, type AdminUsageModelSuggest } from '../api/admin/usage';
import { useAnchoredPopover } from '../hooks/useAnchoredPopover';
import { usePresence } from '../hooks/usePresence';
import { Portal } from './Portal';

export function ModelSuggestInput(props: {
  id: string;
  value: string;
  disabled?: boolean;
  placeholder?: string;
  start?: string;
  end?: string;
  allTime?: boolean;
  onChange: (value: string) => void;
  onSelect: (m: string) => void;
}) {
  const { id, value, disabled, placeholder, start, end, allTime, onChange, onSelect } = props;

  const inputRef = useRef<HTMLInputElement | null>(null);
  const panelRef = useRef<HTMLDivElement | null>(null);
  const [focused, setFocused] = useState(false);
  const [loading, setLoading] = useState(false);
  const [items, setItems] = useState<AdminUsageModelSuggest[]>([]);
  const [err, setErr] = useState('');
  const reqSeqRef = useRef(0);

  const q = useMemo(() => (value || '').trim(), [value]);
  const open = focused && q.length > 0 && (loading || err !== '' || items.length > 0);
  const { present, phase } = usePresence(open, 160);
  const panelStyle = useAnchoredPopover({
    open: present,
    onClose: () => setFocused(false),
    triggerRef: inputRef,
    panelRef,
    offset: 4,
  });

  useEffect(() => {
    if (!focused) return;
    if (q === '') {
      setItems([]);
      setErr('');
      setLoading(false);
      return;
    }

    const seq = ++reqSeqRef.current;
    const timer = window.setTimeout(() => {
      void (async () => {
        setLoading(true);
        setErr('');
        try {
          const res = await getAdminUsageModelSuggest({
            q,
            limit: 20,
            start: start || undefined,
            end: end || undefined,
            all_time: !!allTime || undefined,
          });
          if (reqSeqRef.current !== seq) return;
          if (!res.success) throw new Error(res.message || '加载失败');
          setItems(res.data || []);
        } catch (e) {
          if (reqSeqRef.current !== seq) return;
          setItems([]);
          setErr(e instanceof Error ? e.message : '加载失败');
        } finally {
          if (reqSeqRef.current === seq) setLoading(false);
        }
      })();
    }, 200);

    return () => {
      window.clearTimeout(timer);
    };
  }, [allTime, end, focused, q, start]);

  const select = (m: string) => {
    onSelect(m);
    setFocused(false);
  };

  return (
    <>
      <input
        ref={inputRef}
        id={id}
        type="text"
        className="form-control"
        placeholder={placeholder}
        value={value}
        onChange={(e) => onChange(e.target.value || '')}
        onFocus={() => setFocused(true)}
        onBlur={() => {
          window.setTimeout(() => setFocused(false), 120);
        }}
        disabled={!!disabled}
        autoComplete="off"
      />

      {present ? (
        <Portal>
          <div
            ref={panelRef}
            className={`rlm-suggest-dropdown rlm-popover ${phase === 'enter' ? 'rlm-popover-enter' : 'rlm-popover-leave'}`}
            style={{ ...panelStyle, minWidth: 380, maxWidth: 560, zIndex: 1080 }}
            onPointerDown={(e) => e.stopPropagation()}
          >
            {err ? <div className="text-danger small px-2 py-1">{err}</div> : null}
            {loading ? <div className="text-muted small px-2 py-1">加载中…</div> : null}
            {!loading && !err && items.length === 0 ? <div className="text-muted small px-2 py-1">无匹配模型</div> : null}
            <div className="list-group list-group-flush">
              {items.map((it) => (
                <button
                  key={it.model}
                  type="button"
                  className="list-group-item list-group-item-action py-2 px-2"
                  onMouseDown={(e) => {
                    e.preventDefault();
                    select(it.model);
                  }}
                >
                  <div className="small fw-semibold text-truncate">{it.model}</div>
                </button>
              ))}
            </div>
          </div>
        </Portal>
      ) : null}
    </>
  );
}
