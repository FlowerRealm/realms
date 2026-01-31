import { useEffect, useMemo, useState } from 'react';

type BuildInfo = {
  version?: string;
  env?: string;
  date?: string;
};

function formatBuildInfo(info: BuildInfo | null): { label: string; title: string } | null {
  if (!info) return null;
  const version = (info.version || '').toString().trim();
  if (!version) return null;

  const env = (info.env || '').toString().trim();
  const date = (info.date || '').toString().trim();

  const titleParts: string[] = [];
  if (env) titleParts.push(`env: ${env}`);
  if (date && date !== 'unknown') titleParts.push(`build: ${date}`);

  return { label: version, title: titleParts.join(' · ') };
}

export function ProjectFooter({ variant }: { variant: 'app' | 'admin' | 'public' }) {
  const [buildInfo, setBuildInfo] = useState<{ label: string; title: string } | null>(null);

  useEffect(() => {
    let mounted = true;
    (async () => {
      try {
        const resp = await fetch('/api/version', {
          method: 'GET',
          headers: { Accept: 'application/json' },
          cache: 'no-store',
        });
        if (!resp.ok) return;
        const data = (await resp.json()) as BuildInfo;
        const formatted = formatBuildInfo(data);
        if (mounted) {
          setBuildInfo(formatted);
        }
      } catch {
        // ignore
      }
    })();
    return () => {
      mounted = false;
    };
  }, []);

  const year = useMemo(() => new Date().getFullYear(), []);
  const baseClass =
    variant === 'app'
      ? 'mt-5 text-center text-muted small pb-4 opacity-50'
      : variant === 'admin'
        ? 'mt-5 text-center text-muted small pb-4 opacity-50'
        : 'py-3 my-4 text-center text-muted small opacity-50';

  return (
    <footer id="rlmProjectFooter" data-rlm-github="FlowerRealm/realms" className={baseClass}>
      {variant === 'app' ? 'Realms 中转服务' : variant === 'admin' ? '管理面板' : 'Realms 服务'} &copy; {year}
      {buildInfo ? (
        <span title={buildInfo.title ? buildInfo.title : undefined}>
          {' '}
          · <span id="rlmBuildInfo">{buildInfo.label}</span>
        </span>
      ) : null}{' '}
      ·{' '}
      <a href="https://github.com/FlowerRealm/realms" target="_blank" rel="noopener noreferrer" className="link-secondary text-decoration-none">
        GitHub: FlowerRealm/realms
      </a>
    </footer>
  );
}
