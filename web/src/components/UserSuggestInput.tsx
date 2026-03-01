import { useEffect, useMemo, useRef, useState } from 'react';

import { getAdminUsageUserSuggest, type AdminUsageUserSuggest } from '../api/admin/usage';
import { useAnchoredPopover } from '../hooks/useAnchoredPopover';
import { usePresence } from '../hooks/usePresence';
import { Portal } from './Portal';

export function UserSuggestInput(props: {
  id: string;
  value: string;
  disabled?: boolean;
  placeholder?: string;
  onChange: (value: string) => void;
  onSelect: (u: AdminUsageUserSuggest) => void;
}) {
  const { id, value, disabled, placeholder, onChange, onSelect } = props;

  const inputRef = useRef<HTMLInputElement | null>(null);
  const panelRef = useRef<HTMLDivElement | null>(null);
  const [focused, setFocused] = useState(false);
  const [loading, setLoading] = useState(false);
  const [items, setItems] = useState<AdminUsageUserSuggest[]>([]);
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
          const res = await getAdminUsageUserSuggest(q, 20);
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
  }, [focused, q]);

  const select = (u: AdminUsageUserSuggest) => {
    onSelect(u);
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
          // Delay so click selection can run.
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
            style={{ ...panelStyle, minWidth: 360, maxWidth: 520, zIndex: 1080 }}
            onPointerDown={(e) => e.stopPropagation()}
          >
            {err ? <div className="text-danger small px-2 py-1">{err}</div> : null}
            {loading ? <div className="text-muted small px-2 py-1">加载中…</div> : null}
            {!loading && !err && items.length === 0 ? <div className="text-muted small px-2 py-1">无匹配用户</div> : null}
            <div className="list-group list-group-flush">
              {items.map((u) => (
                <button
                  key={u.id}
                  type="button"
                  className="list-group-item list-group-item-action py-2 px-2"
                  onMouseDown={(e) => {
                    e.preventDefault();
                    select(u);
                  }}
                >
                  <div className="d-flex justify-content-between align-items-center">
                    <div className="small fw-semibold text-truncate">{u.email}</div>
                    <div className="text-muted smaller ms-2">ID: {u.id}</div>
                  </div>
                  <div className="text-muted smaller text-truncate">@{u.username}</div>
                </button>
              ))}
            </div>
          </div>
        </Portal>
      ) : null}
    </>
  );
}
