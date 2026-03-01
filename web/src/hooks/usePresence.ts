import { useEffect, useRef, useState } from 'react';

export function usePresence(open: boolean, durationMs: number) {
  const [present, setPresent] = useState(open);
  const [phase, setPhase] = useState<'enter' | 'leave'>(open ? 'enter' : 'leave');
  const timerRef = useRef<number | null>(null);

  useEffect(() => {
    if (timerRef.current !== null) {
      window.clearTimeout(timerRef.current);
      timerRef.current = null;
    }

    if (open) {
      setPresent(true);
      setPhase('enter');
      return;
    }

    setPhase('leave');
    timerRef.current = window.setTimeout(() => {
      setPresent(false);
      timerRef.current = null;
    }, Math.max(0, durationMs));

    return () => {
      if (timerRef.current !== null) {
        window.clearTimeout(timerRef.current);
        timerRef.current = null;
      }
    };
  }, [durationMs, open]);

  return { present, phase };
}

