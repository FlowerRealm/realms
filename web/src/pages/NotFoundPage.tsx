import { Link } from 'react-router-dom';

export function NotFoundPage() {
  return (
    <div className="container-fluid d-flex flex-column min-vh-100 p-0">
      <main className="flex-fill d-flex flex-column justify-content-center align-items-center">
        <div className="card border-0" style={{ width: '100%', maxWidth: 520 }}>
          <div className="card-body p-4 text-center">
            <div className="bg-primary bg-opacity-10 text-primary rounded-circle d-inline-flex align-items-center justify-content-center mb-3" style={{ width: 56, height: 56 }}>
              <span className="fs-3 material-symbols-rounded">warning</span>
            </div>
            <h2 className="h4 mb-2">404</h2>
            <p className="text-muted mb-4">页面不存在。</p>
            <div className="d-flex gap-2 justify-content-center flex-wrap">
              <Link to="/dashboard" className="btn btn-primary btn-sm">
                前往控制台
              </Link>
              <Link to="/login" className="btn btn-outline-secondary btn-sm">
                前往登录
              </Link>
            </div>
          </div>
        </div>
      </main>
    </div>
  );
}
