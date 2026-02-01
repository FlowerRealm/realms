import { useMemo } from 'react';

export function ProjectFooter({ variant }: { variant: 'app' | 'admin' | 'public' }) {
  const year = useMemo(() => new Date().getFullYear(), []);
  const baseClass =
    variant === 'app'
      ? 'mt-5 text-center text-muted small pb-4 opacity-50'
      : variant === 'admin'
        ? 'mt-5 text-center text-muted small pb-4 opacity-50'
        : 'py-3 my-4 text-center text-muted small opacity-50';

  return (
    <footer id="rlmProjectFooter" data-rlm-github="FlowerRealm/realms" className={baseClass}>
      {variant === 'app' ? 'Realms 中转服务' : variant === 'admin' ? '管理面板' : 'Realms 服务'} &copy; {year} ·{' '}
      <a
        href="https://github.com/FlowerRealm/realms"
        target="_blank"
        rel="noopener noreferrer"
        className="link-secondary text-decoration-none"
      >
        GitHub: FlowerRealm/realms
      </a>
    </footer>
  );
}
