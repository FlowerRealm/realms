import { useEffect, useState } from 'react';

import { getUsageLeaderboard, type UsageLeaderboardUser } from '../api/usage';
import { SegmentedFrame } from '../components/SegmentedFrame';
import { formatUSDPlain } from '../format/money';

type RankingWindow = '1d' | '7d' | '1mo';

const windowOptions: Array<{ value: RankingWindow; label: string; hint: string }> = [
  { value: '1d', label: '今日', hint: '北京时间今日 00:00 至当前' },
  { value: '7d', label: '近7日', hint: '北京时间近 7 日自然日累计至当前' },
  { value: '1mo', label: '近30日', hint: '北京时间近 30 日自然日累计至当前' },
];

type RankingData = {
  window: RankingWindow;
  since: string;
  until: string;
  users: UsageLeaderboardUser[];
};

function rankTone(rank: number) {
  if (rank === 1) {
    return {
      rankColor: '#8b6b2f',
      rankBg: '#f4ecd8',
      avatarBg: '#efe3c0',
      avatarColor: '#7a5922',
    };
  }
  if (rank === 2) {
    return {
      rankColor: '#566273',
      rankBg: '#e9edf2',
      avatarBg: '#dde3ea',
      avatarColor: '#465362',
    };
  }
  if (rank === 3) {
    return {
      rankColor: '#3f6d69',
      rankBg: '#e2efed',
      avatarBg: '#d5e8e4',
      avatarColor: '#315651',
    };
  }
  return {
    rankColor: '#5f6b7a',
    rankBg: '#edf1f5',
    avatarBg: '#e6ebf1',
    avatarColor: '#4c5967',
  };
}

function userInitial(name: string) {
  const trimmed = name.trim();
  if (!trimmed) return '?';
  return trimmed.slice(0, 1).toUpperCase();
}

function formatRankingAmount(value: string) {
  const n = Number(value);
  if (!Number.isFinite(n)) return formatUSDPlain(value);
  return n.toFixed(1);
}

export function RankingPage() {
  const [activeWindow, setActiveWindow] = useState<RankingWindow>('7d');
  const [data, setData] = useState<RankingData | null>(null);
  const [loading, setLoading] = useState(true);
  const [err, setErr] = useState('');

  useEffect(() => {
    let active = true;
    void (async () => {
      setLoading(true);
      setErr('');
      try {
        const res = await getUsageLeaderboard(activeWindow, 100);
        if (!res.success || !res.data) {
          throw new Error(res.message || '排行榜加载失败');
        }
        if (!active) return;
        setData(res.data);
      } catch (e) {
        if (!active) return;
        setErr(e instanceof Error ? e.message : '排行榜加载失败');
        setData(null);
      } finally {
        if (active) setLoading(false);
      }
    })();
    return () => {
      active = false;
    };
  }, [activeWindow]);

  return (
    <div className="fade-in-up">
      <SegmentedFrame>
        <div>
          <div className="d-flex flex-wrap align-items-end justify-content-between gap-3 mb-3">
            <div>
              <h3 className="mb-0 fw-bold">排行榜</h3>
            </div>
            <div className="d-flex flex-wrap gap-2">
              {windowOptions.map((option) => (
                <button
                  key={option.value}
                  type="button"
                  className={`btn btn-sm ${activeWindow === option.value ? 'btn-primary' : 'btn-outline-secondary'}`}
                  onClick={() => setActiveWindow(option.value)}
                  disabled={loading}
                >
                  {option.label}
                </button>
              ))}
            </div>
          </div>

          {err ? (
            <div className="alert alert-danger mb-0">
              <span className="me-2 material-symbols-rounded">warning</span>
              {err}
            </div>
          ) : null}
        </div>

        <div className="card border-0 overflow-hidden mb-0">
          <div className="card-body p-4">
            {loading ? <div className="text-muted py-4">加载中…</div> : null}

            {!loading && data ? (
              <div className="d-flex flex-column gap-3">
                {data.users.map((user) => {
                  const tone = rankTone(user.rank);
                  return (
                    <div key={`${user.rank}-${user.display_name}`} className="card border border-light-subtle shadow-sm mb-0">
                      <div className="card-body d-flex flex-wrap align-items-center justify-content-between gap-3">
                        <div className="d-flex align-items-center gap-4">
                          <div
                            className="d-flex align-items-center justify-content-center fw-bold rounded-4"
                            style={{
                              width: 60,
                              minWidth: 60,
                              height: 60,
                              backgroundColor: tone.rankBg,
                              color: tone.rankColor,
                              fontSize: '2rem',
                              letterSpacing: '-0.04em',
                            }}
                          >
                            {user.rank}
                          </div>
                          <div
                            className="rounded-circle d-flex align-items-center justify-content-center fw-semibold"
                            style={{
                              width: 48,
                              minWidth: 48,
                              height: 48,
                              backgroundColor: tone.avatarBg,
                              color: tone.avatarColor,
                              fontSize: '1.05rem',
                            }}
                          >
                            {userInitial(user.display_name)}
                          </div>
                          <div>
                            <div className="fw-semibold fs-4 lh-sm">{user.display_name}</div>
                          </div>
                        </div>
                        <div className="d-flex flex-wrap align-items-center gap-4 ms-auto text-md-end">
                          <div>
                            <div className="text-muted smaller mb-1">已结算消耗</div>
                            <div className="font-monospace fw-bold fs-3 lh-1">{formatRankingAmount(user.committed_usd)}</div>
                          </div>
                        </div>
                      </div>
                    </div>
                  );
                })}
                {data.users.length === 0 ? <div className="text-center py-5 text-muted small">当前时间窗还没有可展示的消耗数据</div> : null}
              </div>
            ) : null}
          </div>
        </div>
      </SegmentedFrame>
    </div>
  );
}
