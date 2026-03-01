import { useCallback, useEffect, useMemo, useRef, useState } from 'react';

import { stableHash } from '../utils/stableHash';

export type AutoSaveStatus = 'idle' | 'dirty' | 'saving' | 'saved' | 'blocked' | 'error';

export type UseAutoSaveOptions<T> = {
  enabled: boolean;
  value: T;
  trackValue?: unknown;
  save: (value: T) => Promise<void>;

  debounceMs?: number;
  saveOnBlur?: boolean;

  validate?: (value: T) => string | '';
  resetKey?: string | number;
  afterSave?: () => void | Promise<void>;
};

export type UseAutoSaveResult = {
  status: AutoSaveStatus;
  blockedReason: string;
  error: string;
  dirty: boolean;

  retry: () => void;
  flush: () => void;
};

type SaveReason = 'debounce' | 'blur' | 'flush';

function isEnabled(opts: Pick<UseAutoSaveOptions<unknown>, 'enabled'>): boolean {
  return !!opts.enabled;
}

export function useAutoSave<T>(opts: UseAutoSaveOptions<T>): UseAutoSaveResult {
  const debounceMs = typeof opts.debounceMs === 'number' && opts.debounceMs >= 0 ? Math.floor(opts.debounceMs) : 800;
  const saveOnBlur = opts.saveOnBlur !== false;

  const trackingSig = useMemo(() => stableHash(opts.trackValue ?? opts.value), [opts.trackValue, opts.value]);

  const [status, setStatus] = useState<AutoSaveStatus>('idle');
  const [blockedReason, setBlockedReason] = useState('');
  const [error, setError] = useState('');

  const baselineSigRef = useRef<string>(trackingSig);
  const lastAttemptSigRef = useRef<string>('');
  const timerRef = useRef<number | null>(null);
  const inFlightRef = useRef(false);
  const queuedRef = useRef(false);
  const lastSavedAtRef = useRef<number>(0);

  const optsRef = useRef(opts);
  useEffect(() => {
    optsRef.current = opts;
  }, [opts]);

  const enabled = isEnabled(opts);
  const dirty = enabled && trackingSig !== baselineSigRef.current;

  const clearTimer = useCallback(() => {
    if (timerRef.current !== null) {
      window.clearTimeout(timerRef.current);
      timerRef.current = null;
    }
  }, []);

  const runSave = useCallback(
    async (reason: SaveReason) => {
      const curOpts = optsRef.current;
      if (!isEnabled(curOpts)) return;

      const currentTrackingSig = stableHash(curOpts.trackValue ?? curOpts.value);
      if (currentTrackingSig === baselineSigRef.current) {
        setStatus('idle');
        setBlockedReason('');
        setError('');
        return;
      }

      const validate = curOpts.validate;
      if (validate) {
        const msg = (validate(curOpts.value) || '').trim();
        if (msg) {
          setStatus('blocked');
          setBlockedReason(msg);
          setError('');
          return;
        }
      }

      if (inFlightRef.current) {
        queuedRef.current = true;
        setStatus('saving');
        return;
      }

      inFlightRef.current = true;
      queuedRef.current = false;
      setStatus('saving');
      setBlockedReason('');
      setError('');

      const snapshotValue = curOpts.value;
      const snapshotTrackingSig = currentTrackingSig;
      lastAttemptSigRef.current = snapshotTrackingSig;

      let nextTrackingSig = '';
      let followUp = false;
      try {
        await curOpts.save(snapshotValue);
        lastSavedAtRef.current = Date.now();
        baselineSigRef.current = snapshotTrackingSig;
        setStatus('saved');
        setError('');
        if (curOpts.afterSave) await curOpts.afterSave();
      } catch (e) {
        const msg = e instanceof Error ? e.message : '保存失败';
        setStatus('error');
        setError(msg);
      } finally {
        inFlightRef.current = false;
        const nextOpts = optsRef.current;
        nextTrackingSig = stableHash(nextOpts.trackValue ?? nextOpts.value);
        if (queuedRef.current || nextTrackingSig !== baselineSigRef.current) {
          queuedRef.current = false;
          followUp = nextTrackingSig !== lastAttemptSigRef.current;
        }
      }

      if (followUp) {
        void runSave(reason);
        return;
      }
      if (nextTrackingSig && nextTrackingSig === baselineSigRef.current) {
        setStatus(lastSavedAtRef.current ? 'saved' : 'idle');
      }
    },
    [],
  );

  const schedule = useCallback(() => {
    clearTimer();
    if (!enabled) return;
    timerRef.current = window.setTimeout(() => void runSave('debounce'), debounceMs);
  }, [clearTimer, debounceMs, enabled, runSave]);

  const flush = useCallback(() => {
    clearTimer();
    void runSave('flush');
  }, [clearTimer, runSave]);

  const retry = useCallback(() => {
    clearTimer();
    void runSave('flush');
  }, [clearTimer, runSave]);

  useEffect(() => {
    if (!enabled) {
      clearTimer();
      setStatus('idle');
      setBlockedReason('');
      setError('');
      return;
    }
    schedule();
  }, [enabled, trackingSig, schedule, clearTimer]);

  useEffect(() => {
    if (!enabled) return;
    if (opts.resetKey === undefined) return;
    baselineSigRef.current = trackingSig;
    setStatus('idle');
    setBlockedReason('');
    setError('');
    clearTimer();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [enabled, opts.resetKey]);

  useEffect(() => {
    if (!enabled) return;
    if (!saveOnBlur) return;

    const onFocusOut = () => {
      if (!optsRef.current.enabled) return;
      const sig = stableHash(optsRef.current.trackValue ?? optsRef.current.value);
      if (sig === baselineSigRef.current) return;
      clearTimer();
      void runSave('blur');
    };

    const onVisibility = () => {
      if (document.visibilityState !== 'hidden') return;
      onFocusOut();
    };

    window.addEventListener('focusout', onFocusOut);
    document.addEventListener('visibilitychange', onVisibility);
    return () => {
      window.removeEventListener('focusout', onFocusOut);
      document.removeEventListener('visibilitychange', onVisibility);
    };
  }, [enabled, saveOnBlur, clearTimer, runSave]);

  useEffect(() => {
    if (!enabled) return;
    if (status === 'saving') return;
    if (!dirty) {
      if (status === 'dirty') setStatus('idle');
      return;
    }
    if (status === 'idle') setStatus('dirty');
  }, [dirty, enabled, status]);

  return { status, blockedReason, error, dirty, retry, flush };
}
