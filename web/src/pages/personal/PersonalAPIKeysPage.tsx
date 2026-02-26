import { PersonalAPIKeysPanel } from '../admin/PersonalAPIKeysPanel'

export function PersonalAPIKeysPage() {
  return (
    <div className="fade-in-up">
      <div className="d-flex justify-content-between align-items-center mb-4">
        <div>
          <h3 className="mb-1 fw-bold">API Keys</h3>
          <p className="text-muted small mb-0">管理 API Key（可创建、可撤销）。</p>
        </div>
      </div>

      <PersonalAPIKeysPanel />
    </div>
  )
}
