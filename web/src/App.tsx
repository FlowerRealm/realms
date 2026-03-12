import { Suspense, lazy } from 'react';
import { Navigate, Route, Routes } from 'react-router-dom';

import { RequireAuth } from './auth/RequireAuth';
import { useAuth } from './auth/AuthContext';
import { AdminLayout } from './layout/AdminLayout';
import { AppLayout } from './layout/AppLayout';
import { PublicLayout } from './layout/PublicLayout';

const AdminPage = lazy(() => import('./pages/AdminPage').then((m) => ({ default: m.AdminPage })));
const AccountPage = lazy(() => import('./pages/AccountPage').then((m) => ({ default: m.AccountPage })));
const AnnouncementDetailPage = lazy(() => import('./pages/AnnouncementDetailPage').then((m) => ({ default: m.AnnouncementDetailPage })));
const AnnouncementsPage = lazy(() => import('./pages/AnnouncementsPage').then((m) => ({ default: m.AnnouncementsPage })));
const DashboardPage = lazy(() => import('./pages/DashboardPage').then((m) => ({ default: m.DashboardPage })));
const LoginPage = lazy(() => import('./pages/LoginPage').then((m) => ({ default: m.LoginPage })));
const ModelsPage = lazy(() => import('./pages/ModelsPage').then((m) => ({ default: m.ModelsPage })));
const NotFoundPage = lazy(() => import('./pages/NotFoundPage').then((m) => ({ default: m.NotFoundPage })));
const OAuthAuthorizePage = lazy(() => import('./pages/OAuthAuthorizePage').then((m) => ({ default: m.OAuthAuthorizePage })));
const PayPage = lazy(() => import('./pages/PayPage').then((m) => ({ default: m.PayPage })));
const RankingPage = lazy(() => import('./pages/RankingPage').then((m) => ({ default: m.RankingPage })));
const RegisterPage = lazy(() => import('./pages/RegisterPage').then((m) => ({ default: m.RegisterPage })));
const SubscriptionPage = lazy(() => import('./pages/SubscriptionPage').then((m) => ({ default: m.SubscriptionPage })));
const TicketDetailPage = lazy(() => import('./pages/TicketDetailPage').then((m) => ({ default: m.TicketDetailPage })));
const TicketNewPage = lazy(() => import('./pages/TicketNewPage').then((m) => ({ default: m.TicketNewPage })));
const TicketsPage = lazy(() => import('./pages/TicketsPage').then((m) => ({ default: m.TicketsPage })));
const TokenCreatedPage = lazy(() => import('./pages/TokenCreatedPage').then((m) => ({ default: m.TokenCreatedPage })));
const TokensPage = lazy(() => import('./pages/TokensPage').then((m) => ({ default: m.TokensPage })));
const TopupPage = lazy(() => import('./pages/TopupPage').then((m) => ({ default: m.TopupPage })));
const UsagePage = lazy(() => import('./pages/UsagePage').then((m) => ({ default: m.UsagePage })));
const UserGuidePage = lazy(() => import('./pages/UserGuidePage').then((m) => ({ default: m.UserGuidePage })));

function RouteFallback() {
  return <div className="container py-5 text-muted">加载中…</div>;
}

function HomeRedirect() {
  const { user, booting } = useAuth();
  if (booting) return null;
  if (user) return <Navigate to="/dashboard" replace />;
  return <Navigate to="/login" replace state={{ from: '/dashboard' }} />;
}

export function App() {
  const { booting } = useAuth();
  if (booting) return null;

  return (
    <Suspense fallback={<RouteFallback />}>
      <Routes>
        <Route path="/" element={<HomeRedirect />} />
        <Route element={<PublicLayout />}>
          <Route path="/login" element={<LoginPage />} />
          <Route path="/register" element={<RegisterPage />} />
        </Route>

        <Route
          path="/oauth/authorize"
          element={
            <RequireAuth>
              <OAuthAuthorizePage />
            </RequireAuth>
          }
        />

        <Route
          element={
            <RequireAuth>
              <AppLayout />
            </RequireAuth>
          }
        >
          <Route path="/dashboard" element={<DashboardPage />} />
          <Route path="/guide" element={<UserGuidePage />} />
          <Route path="/announcements" element={<AnnouncementsPage />} />
          <Route path="/announcements/:id" element={<AnnouncementDetailPage />} />
          <Route path="/tokens" element={<TokensPage />} />
          <Route path="/tokens/created" element={<TokenCreatedPage />} />
          <Route path="/models" element={<ModelsPage />} />
          <Route path="/ranking" element={<RankingPage />} />
          <Route path="/usage" element={<UsagePage />} />
          <Route path="/account" element={<AccountPage />} />
          <Route path="/subscription" element={<SubscriptionPage />} />
          <Route path="/topup" element={<TopupPage />} />
          <Route path="/pay/:kind/:orderId" element={<PayPage />} />
          <Route path="/pay/:kind/:orderId/success" element={<PayPage />} />
          <Route path="/pay/:kind/:orderId/cancel" element={<PayPage />} />
          <Route path="/tickets" element={<TicketsPage mode="all" />} />
          <Route path="/tickets/open" element={<TicketsPage mode="open" />} />
          <Route path="/tickets/closed" element={<TicketsPage mode="closed" />} />
          <Route path="/tickets/new" element={<TicketNewPage />} />
          <Route path="/tickets/:id" element={<TicketDetailPage />} />
        </Route>

        <Route
          element={
            <RequireAuth>
              <AdminLayout />
            </RequireAuth>
          }
        >
          <Route path="/admin/*" element={<AdminPage />} />
        </Route>

        <Route path="*" element={<NotFoundPage />} />
      </Routes>
    </Suspense>
  );
}
