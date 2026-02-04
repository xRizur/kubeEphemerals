import React, { useState, useCallback } from 'react';
import useBaseUrl from '@docusaurus/useBaseUrl';

// Operator UI uses these exact CSS variables (global_dashboard.html)
const OPERATOR_CSS = `
  .demo-operator-root {
    --bg-primary: #0d1117;
    --bg-secondary: #161b22;
    --bg-tertiary: #21262d;
    --border-color: #30363d;
    --text-primary: #c9d1d9;
    --text-secondary: #8b949e;
    --accent-blue: #58a6ff;
    --accent-green: #3fb950;
    --accent-yellow: #d29922;
    --accent-red: #f85149;
    --accent-purple: #a371f7;
  }
  .demo-operator-root * { box-sizing: border-box; margin: 0; padding: 0; }
  .demo-operator-root { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Helvetica, Arial, sans-serif; background: var(--bg-primary); color: var(--text-primary); line-height: 1.6; border-radius: 12px; overflow: hidden; border: 1px solid var(--border-color); }
  .demo-operator-root .container { max-width: 1400px; margin: 0 auto; padding: 24px; }
  .demo-operator-root header { background: var(--bg-secondary); border-bottom: 1px solid var(--border-color); padding: 16px 24px; display: flex; justify-content: space-between; align-items: center; }
  .demo-operator-root header h1 { font-size: 20px; font-weight: 600; display: flex; align-items: center; gap: 12px; }
  .demo-operator-root header h1 .logo { height: 28px; vertical-align: middle; margin-right: 8px; }
  .demo-operator-root .demo-nav a { color: var(--text-secondary); text-decoration: none; font-size: 14px; padding: 8px 12px; border-radius: 6px; transition: all 0.2s; }
  .demo-operator-root .demo-nav a:hover { color: var(--text-primary); background: var(--bg-tertiary); }
  .demo-operator-root h2 { font-size: 24px; font-weight: 600; margin-bottom: 16px; }
  .demo-operator-root .card { background: var(--bg-secondary); border: 1px solid var(--border-color); border-radius: 8px; padding: 20px; margin-bottom: 16px; }
  .demo-operator-root table { width: 100%; border-collapse: collapse; }
  .demo-operator-root th, .demo-operator-root td { text-align: left; padding: 12px 16px; border-bottom: 1px solid var(--border-color); }
  .demo-operator-root th { font-weight: 600; color: var(--text-secondary); font-size: 12px; text-transform: uppercase; }
  .demo-operator-root tr:hover { background: var(--bg-tertiary); }
  .demo-operator-root .status-badge { display: inline-flex; align-items: center; padding: 4px 10px; border-radius: 20px; font-size: 12px; font-weight: 500; }
  .demo-operator-root .status-active { background: rgba(63, 185, 80, 0.15); color: var(--accent-green); }
  .demo-operator-root .status-pending { background: rgba(210, 153, 34, 0.15); color: var(--accent-yellow); }
  .demo-operator-root .status-failed { background: rgba(248, 81, 73, 0.15); color: var(--accent-red); }
  .demo-operator-root .status-expired { background: rgba(139, 148, 158, 0.15); color: var(--text-secondary); }
  .demo-operator-root .btn { display: inline-flex; align-items: center; gap: 6px; padding: 8px 16px; border: 1px solid var(--border-color); border-radius: 6px; font-size: 14px; font-weight: 500; cursor: pointer; transition: all 0.2s; text-decoration: none; background: transparent; color: inherit; }
  .demo-operator-root .btn-primary { background: var(--accent-green); border-color: var(--accent-green); color: #fff; }
  .demo-operator-root .btn-primary:hover { background: #2ea043; }
  .demo-operator-root .btn-secondary { background: var(--bg-tertiary); color: var(--text-primary); }
  .demo-operator-root .btn-secondary:hover { background: var(--border-color); }
  .demo-operator-root .link { color: var(--accent-blue); text-decoration: none; }
  .demo-operator-root .link:hover { text-decoration: underline; }
  .demo-operator-root .actions { display: flex; gap: 8px; }
  .demo-operator-root .icon-btn { padding: 6px 10px; background: transparent; border: 1px solid var(--border-color); border-radius: 6px; color: var(--text-secondary); cursor: pointer; transition: all 0.2s; font-size: 14px; }
  .demo-operator-root .icon-btn:hover { color: var(--text-primary); background: var(--bg-tertiary); }
  .demo-operator-root .icon-btn.danger:hover { color: var(--accent-red); border-color: var(--accent-red); }
  .demo-operator-root .empty-state { text-align: center; padding: 48px 24px; color: var(--text-secondary); }
  .demo-operator-root .empty-state h3 { font-size: 18px; margin-bottom: 8px; color: var(--text-primary); }
  .demo-operator-root .modal { display: none; position: fixed; top: 0; left: 0; width: 100%; height: 100%; background: rgba(0, 0, 0, 0.7); z-index: 1000; align-items: center; justify-content: center; }
  .demo-operator-root .modal.active { display: flex; }
  .demo-operator-root .modal-content { background: var(--bg-secondary); border: 1px solid var(--border-color); border-radius: 12px; padding: 24px; max-width: 500px; width: 90%; max-height: 90vh; overflow-y: auto; }
  .demo-operator-root .modal-form-wide { max-width: 560px; }
  .demo-operator-root .modal-header { display: flex; justify-content: space-between; align-items: center; margin-bottom: 20px; }
  .demo-operator-root .modal-title { font-size: 18px; font-weight: 600; }
  .demo-operator-root .form-group { margin-bottom: 16px; }
  .demo-operator-root .form-group label { display: block; font-size: 14px; font-weight: 500; margin-bottom: 8px; }
  .demo-operator-root .form-group input, .demo-operator-root .form-group select, .demo-operator-root .form-group textarea { width: 100%; padding: 10px 12px; background: var(--bg-primary); border: 1px solid var(--border-color); border-radius: 6px; color: var(--text-primary); font-size: 14px; }
  .demo-operator-root .form-group input:focus, .demo-operator-root .form-group select:focus { outline: none; border-color: var(--accent-blue); }
  .demo-operator-root .form-actions { display: flex; justify-content: flex-end; gap: 12px; margin-top: 16px; padding-top: 16px; border-top: 1px solid var(--border-color); }
  .demo-operator-root .form-row { display: grid; grid-template-columns: 1fr 1fr; gap: 16px; }
  .demo-operator-root .tabs { display: flex; gap: 8px; margin-bottom: 24px; border-bottom: 1px solid var(--border-color); }
  .demo-operator-root .tab-btn { padding: 12px 20px; background: transparent; border: none; border-bottom: 2px solid transparent; color: var(--text-secondary); cursor: pointer; font-size: 14px; font-weight: 500; transition: all 0.2s; }
  .demo-operator-root .tab-btn:hover { color: var(--text-primary); }
  .demo-operator-root .tab-btn.active { color: var(--accent-blue); border-bottom-color: var(--accent-blue); }
  .demo-operator-root .tab-content { display: none; }
  .demo-operator-root .tab-content.active { display: block; }
  .demo-operator-root .templates-grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(350px, 1fr)); gap: 20px; }
  .demo-operator-root .template-card { background: var(--bg-secondary); border: 1px solid var(--border-color); border-radius: 12px; padding: 20px; transition: all 0.2s; }
  .demo-operator-root .template-card:hover { border-color: var(--accent-blue); transform: translateY(-2px); }
  .demo-operator-root .template-header { display: flex; align-items: center; gap: 12px; margin-bottom: 12px; }
  .demo-operator-root .template-icon { font-size: 32px; }
  .demo-operator-root .template-header h3 { font-size: 18px; font-weight: 600; margin: 0; }
  .demo-operator-root .template-desc { color: var(--text-secondary); font-size: 14px; margin-bottom: 12px; line-height: 1.5; }
  .demo-operator-root .template-meta { display: flex; gap: 16px; color: var(--text-secondary); font-size: 12px; margin-bottom: 12px; }
  .demo-operator-root .template-tags { display: flex; flex-wrap: wrap; gap: 6px; margin-bottom: 16px; }
  .demo-operator-root .tag { background: var(--bg-tertiary); color: var(--text-secondary); padding: 4px 10px; border-radius: 12px; font-size: 11px; }
  .demo-operator-root .template-components { background: var(--bg-primary); padding: 12px; border-radius: 8px; margin-bottom: 16px; font-size: 13px; }
  .demo-operator-root .template-components ul { margin: 8px 0 0 16px; padding: 0; }
  .demo-operator-root .template-components li { margin: 4px 0; color: var(--text-secondary); }
  .demo-operator-root .template-actions { display: flex; justify-content: flex-end; }
  .demo-operator-root .details-summary { padding: 12px; background: var(--bg-tertiary); border-radius: 6px; margin-bottom: 12px; cursor: pointer; }
  .demo-operator-root .progress-mini { font-size: 11px; color: var(--text-secondary); margin-top: 4px; }
  .demo-operator-root .env-header { display: flex; justify-content: space-between; align-items: flex-start; margin-bottom: 24px; flex-wrap: wrap; gap: 16px; }
  .demo-operator-root .env-info h2 { margin-bottom: 8px; }
  .demo-operator-root .env-meta { display: flex; gap: 24px; color: var(--text-secondary); font-size: 14px; flex-wrap: wrap; }
  .demo-operator-root .env-meta-item { display: flex; align-items: center; gap: 6px; }
  .demo-operator-root .env-controls { display: flex; gap: 12px; flex-wrap: wrap; }
  .demo-operator-root .env-tab { padding: 12px 20px; border: none; border-bottom: 2px solid transparent; margin-bottom: -1px; background: transparent; color: var(--text-secondary); cursor: pointer; font-size: 14px; transition: all 0.2s; }
  .demo-operator-root .env-tab:hover { color: var(--text-primary); }
  .demo-operator-root .env-tab.active { color: var(--text-primary); border-bottom-color: var(--accent-blue); }
  .demo-operator-root .log-container { background: #0d1117; border: 1px solid var(--border-color); border-radius: 8px; padding: 16px; font-family: monospace; font-size: 12px; line-height: 1.5; height: 320px; overflow-y: auto; }
  .demo-operator-root .log-line { white-space: pre-wrap; word-break: break-all; }
  .demo-operator-root .log-line:hover { background: var(--bg-tertiary); }
  .demo-operator-root .pod-selector { display: flex; gap: 12px; margin-bottom: 16px; align-items: center; flex-wrap: wrap; }
  .demo-operator-root .config-grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(260px, 1fr)); gap: 16px; }
  .demo-operator-root .config-item { background: var(--bg-tertiary); border-radius: 8px; padding: 16px; }
  .demo-operator-root .config-item label { display: block; font-size: 12px; color: var(--text-secondary); margin-bottom: 4px; }
  .demo-operator-root .config-item pre { font-family: monospace; font-size: 13px; white-space: pre-wrap; word-break: break-all; margin: 0; }
  .demo-operator-root .condition-list { display: flex; flex-direction: column; gap: 12px; }
  .demo-operator-root .condition { display: flex; align-items: center; gap: 12px; padding: 12px 16px; background: var(--bg-tertiary); border-radius: 8px; }
  .demo-operator-root .condition-icon { font-size: 20px; }
  .demo-operator-root .condition-info { flex: 1; }
  .demo-operator-root .condition-type { font-weight: 600; margin-bottom: 2px; }
  .demo-operator-root .condition-reason { font-size: 13px; color: var(--text-secondary); }
  .demo-operator-root .condition-time { font-size: 12px; color: var(--text-secondary); }
  .demo-operator-root .card-header { display: flex; justify-content: space-between; align-items: center; margin-bottom: 16px; padding-bottom: 16px; border-bottom: 1px solid var(--border-color); }
  .demo-operator-root .card-title { font-size: 16px; font-weight: 600; }
`;

const MOCK_TEMPLATES = [
  { id: 'frontend', namespace: 'ephemeral-system', displayName: 'Frontend', icon: '🌐', description: 'Nginx frontend for quick previews', defaultTTL: '1h', components: [{ name: 'nginx', chart: 'nginx', version: '15.0.0', primary: true }], tags: ['web', 'simple'] },
  { id: 'backend', namespace: 'ephemeral-system', displayName: 'Backend', icon: '📦', description: 'API + database stack', defaultTTL: '2h', components: [{ name: 'api', chart: 'podinfo', version: '6.9.4', primary: true }, { name: 'db', chart: 'postgresql', version: '12', primary: false }], tags: ['api', 'database'] },
  { id: 'fullstack-webapp', namespace: 'ephemeral-system', displayName: 'Full Stack Web App', icon: '🚀', description: 'Frontend, API, and PostgreSQL', defaultTTL: '4h', components: [{ name: 'frontend', chart: 'nginx', version: '15.0.0', primary: true }, { name: 'database', chart: 'postgresql', version: '12.0.0', primary: false }], tags: ['fullstack', 'web'] },
];

const ENVIRONMENTS_INITIAL = [
  { name: 'staging-env', status: 'Active', activeNamespace: 'env-staging-env', ttlRemaining: '45m', helmRelease: 'staging-env-podinfo', accessURL: 'https://staging-env.preview.example.com', progress: 100 },
];

const MOCK_CONDITIONS = [
  { type: 'NamespaceReady', status: 'True', reason: 'NamespaceCreated', message: 'Namespace env-staging-env created', lastTransition: '2m ago' },
  { type: 'NetworkPolicyApplied', status: 'True', reason: 'NetworkPolicyCreated', message: 'Deny-all NetworkPolicy applied', lastTransition: '2m ago' },
  { type: 'HelmDeployed', status: 'True', reason: 'HelmInstallSuccess', message: 'Release staging-env-podinfo deployed', lastTransition: '1m ago' },
  { type: 'HTTPRouteReady', status: 'True', reason: 'RouteAccepted', message: 'HTTPRoute accepted by gateway', lastTransition: '1m ago' },
];

const MOCK_PODS = [
  { name: 'staging-env-podinfo-7b8c9d-xyz12', status: 'Running', ready: true, restarts: 0, age: '5m' },
  { name: 'staging-env-podinfo-7b8c9d-abc34', status: 'Running', ready: true, restarts: 0, age: '5m' },
];

const MOCK_LOG_LINES = [
  '2026-02-02T10:00:01Z INFO  Starting podinfo',
  '2026-02-02T10:00:02Z INFO  Listening on :9898',
  '2026-02-02T10:00:03Z INFO  Readiness probe passed',
  '2026-02-02T10:00:04Z INFO  Liveness probe passed',
  '2026-02-02T10:01:00Z INFO  Request GET / ready=200',
  '2026-02-02T10:02:00Z INFO  Request GET /health ready=200',
];

function Toast({ message, onClose }) {
  return (
    <div style={{ position: 'fixed', bottom: 24, right: 24, padding: '12px 20px', borderRadius: 8, background: 'var(--accent-green)', color: '#fff', zIndex: 9999, display: 'flex', alignItems: 'center', gap: 12, boxShadow: '0 4px 12px rgba(0,0,0,0.3)' }}>
      <span style={{ flex: 1 }}>{message}</span>
      <button type="button" onClick={onClose} className="icon-btn" style={{ padding: '4px 8px' }}>✕</button>
    </div>
  );
}

function ProgressBar({ label, pct }) {
  return (
    <div className="progress-mini">
      {label}: <span style={{ color: 'var(--accent-blue)' }}>{pct}%</span>
      <div style={{ height: 4, background: 'var(--bg-tertiary)', borderRadius: 2, marginTop: 4, overflow: 'hidden' }}>
        <div style={{ height: '100%', width: `${pct}%`, background: 'var(--accent-blue)', borderRadius: 2, transition: 'width 0.3s' }} />
      </div>
    </div>
  );
}

function EnvDetailView({ env, onBack, onExtendTTL, onKubeconfig }) {
  const [detailTab, setDetailTab] = useState('overview');
  const [logPod, setLogPod] = useState(MOCK_PODS[0]?.name || '');
  const statusClass = (s) => (s === 'Active' ? 'status-active' : s === 'Pending' ? 'status-pending' : 'status-expired');
  return (
    <>
      <div className="env-header">
        <div className="env-info">
          <h2>
            <button type="button" className="link" onClick={onBack} style={{ textDecoration: 'none', background: 'none', border: 'none', cursor: 'pointer', fontSize: 'inherit', marginRight: 8 }}>←</button>
            {env.name}
          </h2>
          <div className="env-meta">
            <div className="env-meta-item"><span className={`status-badge ${statusClass(env.status)}`}>{env.status}</span></div>
            <div className="env-meta-item">📁 {env.activeNamespace}</div>
            <div className="env-meta-item">⏰ TTL: {env.ttlRemaining}</div>
            {env.helmRelease && env.helmRelease !== '—' && <div className="env-meta-item">📦 {env.helmRelease}</div>}
          </div>
        </div>
        <div className="env-controls">
          <button type="button" className="btn btn-secondary" onClick={() => onExtendTTL(env.name)}>⏰ Extend TTL</button>
          <button type="button" className="btn btn-secondary" onClick={() => onKubeconfig(env.name)}>📥 Download kubeconfig</button>
          {env.accessURL && (
            <a href={env.accessURL} target="_blank" rel="noopener noreferrer" className="btn btn-primary">🔗 Open App</a>
          )}
        </div>
      </div>
      <div className="tabs" style={{ marginBottom: 20, borderBottom: '1px solid var(--border-color)' }}>
        {['overview', 'pods', 'logs', 'config'].map((t) => (
          <button key={t} type="button" className={`env-tab ${detailTab === t ? 'active' : ''}`} onClick={() => setDetailTab(t)}>
            {t === 'overview' && '📊 Overview'}
            {t === 'pods' && '🐳 Pods'}
            {t === 'logs' && '📜 Logs'}
            {t === 'config' && '⚙️ Config'}
          </button>
        ))}
      </div>
      {detailTab === 'overview' && (
        <>
          <div className="card">
            <div className="card-header"><h3 className="card-title">Conditions</h3></div>
            <div className="condition-list">
              {MOCK_CONDITIONS.map((c) => (
                <div key={c.type} className="condition">
                  <div className="condition-icon">{c.status === 'True' ? '✅' : '⏳'}</div>
                  <div className="condition-info">
                    <div className="condition-type">{c.type}</div>
                    <div className="condition-reason">{c.reason}: {c.message}</div>
                  </div>
                  <div className="condition-time">{c.lastTransition}</div>
                </div>
              ))}
            </div>
          </div>
          <div className="card">
            <div className="card-header"><h3 className="card-title">Gateway &amp; Routing</h3></div>
            <div className="config-grid">
              <div className="config-item"><label>Access URL</label><pre>{env.accessURL || '—'}</pre></div>
              <div className="config-item"><label>Gateway</label><pre>default/main-gateway</pre></div>
              <div className="config-item"><label>Target Service</label><pre>{env.helmRelease}:80</pre></div>
            </div>
          </div>
          <div className="card">
            <div className="card-header"><h3 className="card-title">Deployed Components</h3></div>
            <table>
              <thead><tr><th>Name</th><th>Chart</th><th>Version</th><th>Primary</th></tr></thead>
              <tbody>
                <tr>
                  <td>podinfo</td>
                  <td>podinfo</td>
                  <td>6.9.4</td>
                  <td>✅</td>
                </tr>
              </tbody>
            </table>
          </div>
        </>
      )}
      {detailTab === 'pods' && (
        <div className="card">
          <table>
            <thead><tr><th>Name</th><th>Status</th><th>Ready</th><th>Restarts</th><th>Age</th><th>Actions</th></tr></thead>
            <tbody>
              {MOCK_PODS.map((p) => (
                <tr key={p.name}>
                  <td>{p.name}</td>
                  <td><span className={`status-badge ${p.status === 'Running' ? 'status-active' : 'status-pending'}`}>{p.status}</span></td>
                  <td>{p.ready ? '✅' : '❌'}</td>
                  <td>{p.restarts}</td>
                  <td>{p.age}</td>
                  <td><button type="button" className="icon-btn" title="View Logs" onClick={() => { setLogPod(p.name); setDetailTab('logs'); }}>📜</button></td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
      {detailTab === 'logs' && (
        <>
          <div className="pod-selector">
            <label>Pod:</label>
            <select value={logPod} onChange={(e) => setLogPod(e.target.value)} style={{ padding: '8px 12px', background: 'var(--bg-secondary)', border: '1px solid var(--border-color)', borderRadius: 6, color: 'var(--text-primary)', fontSize: 14, minWidth: 220 }}>
              <option value="">Select a pod...</option>
              {MOCK_PODS.map((p) => <option key={p.name} value={p.name}>{p.name}</option>)}
            </select>
          </div>
          <p style={{ fontSize: 13, color: 'var(--text-secondary)', marginBottom: 12 }}>
            Pod logs from the selected container.
          </p>
          <div className="log-container">
            {logPod ? MOCK_LOG_LINES.map((line, i) => <div key={i} className="log-line">{line}</div>) : <div style={{ color: 'var(--text-secondary)', padding: 20, textAlign: 'center' }}>Select a pod to view logs</div>}
          </div>
        </>
      )}
      {detailTab === 'config' && (
        <>
          <div className="card">
            <div className="card-header"><h3 className="card-title">Gateway</h3></div>
            <div className="config-grid">
              <div className="config-item"><label>Name</label><pre>main-gateway</pre></div>
              <div className="config-item"><label>Namespace</label><pre>default</pre></div>
              <div className="config-item"><label>Domain Prefix</label><pre>{env.name}</pre></div>
            </div>
          </div>
          <div className="card">
            <div className="card-header"><h3 className="card-title">Helm</h3></div>
            <div className="config-grid">
              <div className="config-item"><label>Release</label><pre>{env.helmRelease || '—'}</pre></div>
              <div className="config-item"><label>Chart</label><pre>podinfo</pre></div>
              <div className="config-item"><label>Version</label><pre>6.9.4</pre></div>
            </div>
          </div>
        </>
      )}
    </>
  );
}

export default function DemoDashboard() {
  const logoUrl = useBaseUrl('img/logo.png');
  const [activeTab, setActiveTab] = useState('environments');
  const [selectedEnv, setSelectedEnv] = useState(null);
  const [environments, setEnvironments] = useState(ENVIRONMENTS_INITIAL);
  const [createModalOpen, setCreateModalOpen] = useState(false);
  const [extendModalOpen, setExtendModalOpen] = useState(false);
  const [extendEnvName, setExtendEnvName] = useState('');
  const [createTemplateModalOpen, setCreateTemplateModalOpen] = useState(false);
  const [toast, setToast] = useState(null);
  const [createForm, setCreateForm] = useState({
    templateId: '',
    templateNamespace: '',
    envName: 'pr-123',
    ttl: '1h',
    helmRepo: '',
    helmChart: '',
    helmVersion: '',
    gatewayName: 'main-gateway',
    gatewayNamespace: 'default',
    discoveryPort: '',
    serviceName: '',
    servicePort: 80,
  });

  const handleCreateEnv = useCallback((e) => {
    e?.preventDefault();
    const name = createForm.envName.trim() || 'pr-demo';
    const id = `demo-${Date.now()}`;
    const tpl = MOCK_TEMPLATES.find((t) => t.id === createForm.templateId);
    setEnvironments((prev) => [
      ...prev,
      {
        name,
        id,
        status: 'Pending',
        activeNamespace: `env-${name}`,
        ttlRemaining: createForm.ttl,
        helmRelease: '—',
        accessURL: null,
        progress: 0,
      },
    ]);
    setCreateModalOpen(false);
    setCreateForm((f) => ({ ...f, envName: 'pr-123', ttl: '1h', templateId: '', templateNamespace: '' }));

    let progress = 0;
    const iv = setInterval(() => {
      progress += 10;
      if (progress >= 100) {
        clearInterval(iv);
        setEnvironments((prev) =>
          prev.map((e) =>
            e.id === id
              ? {
                  ...e,
                  status: 'Active',
                  helmRelease: `${name}-podinfo`,
                  accessURL: `https://${name}.preview.example.com`,
                  progress: 100,
                }
              : e
          )
        );
        setToast({ message: `Environment ${name} is ready!` });
        return;
      }
      setEnvironments((prev) => prev.map((e) => (e.id === id ? { ...e, progress } : e)));
    }, 250);
  }, [createForm.envName, createForm.ttl, createForm.templateId]);

  const handleDeleteEnv = useCallback((name) => {
    if (!confirm(`Delete environment "${name}"?`)) return;
    setEnvironments((prev) => prev.filter((e) => e.name !== name));
    setSelectedEnv(null);
    setToast({ message: `Environment ${name} deleted.` });
  }, []);

  const handleKubeconfig = useCallback((name) => {
    setToast({ message: `Download kubeconfig-${name}.yaml (demo). In the real UI: GET /api/envs/${name}/kubeconfig` });
  }, []);

  const openExtend = (name) => {
    setExtendEnvName(name);
    setExtendModalOpen(true);
  };
  const closeExtend = () => {
    setExtendModalOpen(false);
    setExtendEnvName('');
  };
  const handleExtend = (e) => {
    e?.preventDefault();
    setEnvironments((prev) =>
      prev.map((e) => (e.name === extendEnvName ? { ...e, ttlRemaining: '2h' } : e))
    );
    closeExtend();
    setToast({ message: `TTL extended for ${extendEnvName}.` });
  };

  const createFromTemplate = (templateId, namespace) => {
    const t = MOCK_TEMPLATES.find((x) => x.id === templateId);
    setCreateForm((f) => ({
      ...f,
      templateId,
      templateNamespace: namespace || 'ephemeral-system',
      envName: `pr-${templateId}-demo`,
      ttl: t?.defaultTTL || '1h',
      serviceName: t?.components?.[0]?.name || '',
      servicePort: 80,
    }));
    setCreateModalOpen(true);
  };

  const statusClass = (status) => {
    if (status === 'Active') return 'status-active';
    if (status === 'Pending') return 'status-pending';
    if (status === 'Failed') return 'status-failed';
    if (status === 'Expired') return 'status-expired';
    return 'status-pending';
  };

  return (
    <div className="demo-operator-root" style={{ margin: '0 auto', maxWidth: 1400 }}>
      <style>{OPERATOR_CSS}</style>

      <header>
        <h1><img src={logoUrl} alt="" className="logo" />Ephemeral Operator</h1>
        <nav className="demo-nav">
          <a href="#env">🏠 Dashboard</a>
          <a href="#templates" onClick={(e) => { e.preventDefault(); setActiveTab('templates'); }}>📦 Templates</a>
          <a href="#api">📋 API</a>
        </nav>
      </header>

      <div className="container">
        {selectedEnv ? (
          <EnvDetailView
            env={selectedEnv}
            onBack={() => setSelectedEnv(null)}
            onExtendTTL={openExtend}
            onKubeconfig={handleKubeconfig}
          />
        ) : (
          <>
        <div className="tabs" style={{ marginBottom: 24, borderBottom: '1px solid var(--border-color)' }}>
          <button type="button" className={`tab-btn ${activeTab === 'environments' ? 'active' : ''}`} onClick={() => setActiveTab('environments')}>🏠 Environments</button>
          <button type="button" className={`tab-btn ${activeTab === 'templates' ? 'active' : ''}`} onClick={() => setActiveTab('templates')}>📦 Templates</button>
        </div>

        {/* Environments Tab */}
        <div id="environments-tab" className={`tab-content ${activeTab === 'environments' ? 'active' : ''}`}>
          <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 24 }}>
            <h2>Ephemeral Environments</h2>
            <button type="button" className="btn btn-primary" onClick={() => setCreateModalOpen(true)}>➕ Create Environment</button>
          </div>
          <div className="card">
            <table>
              <thead>
                <tr>
                  <th>Name</th>
                  <th>Status</th>
                  <th>Namespace</th>
                  <th>TTL Remaining</th>
                  <th>Helm Release</th>
                  <th>Access URL</th>
                  <th>Actions</th>
                </tr>
              </thead>
              <tbody>
                {environments.length === 0 ? (
                  <tr>
                    <td colSpan={7}>
                      <div className="empty-state">
                        <h3>No environments yet</h3>
                        <p>Create your first ephemeral environment to get started.</p>
                      </div>
                    </td>
                  </tr>
                ) : (
                  environments.map((env) => (
                    <tr key={env.id || env.name}>
                      <td>
                        <button type="button" className="link" onClick={() => setSelectedEnv(env)} style={{ background: 'none', border: 'none', cursor: 'pointer', padding: 0, font: 'inherit', textAlign: 'left' }}>{env.name}</button>
                      </td>
                      <td><span className={`status-badge ${statusClass(env.status)}`}>{env.status}</span></td>
                      <td>{env.activeNamespace}</td>
                      <td>{env.ttlRemaining}</td>
                      <td>{env.helmRelease}</td>
                      <td>
                        {env.status === 'Pending' && env.progress !== undefined && (
                          <ProgressBar label="Provisioning" pct={env.progress} />
                        )}
                        {env.status === 'Active' && env.accessURL && (
                          <a href={env.accessURL} target="_blank" rel="noopener noreferrer" className="link">{env.accessURL}</a>
                        )}
                        {env.status === 'Active' && !env.accessURL && <span style={{ color: 'var(--text-secondary)' }}>—</span>}
                      </td>
                      <td>
                        <div className="actions">
                          <button type="button" className="icon-btn" title="View Details" onClick={() => setSelectedEnv(env)}>👁️</button>
                          <button type="button" className="icon-btn" title="Extend TTL" onClick={() => openExtend(env.name)}>⏰</button>
                          <button type="button" className="icon-btn danger" title="Delete" onClick={() => handleDeleteEnv(env.name)}>🗑️</button>
                        </div>
                      </td>
                    </tr>
                  ))
                )}
              </tbody>
            </table>
          </div>
        </div>

        {/* Templates Tab */}
        <div id="templates-tab" className={`tab-content ${activeTab === 'templates' ? 'active' : ''}`} style={activeTab === 'templates' ? {} : { display: 'none' }}>
          <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 24 }}>
            <h2>Environment Templates</h2>
            <button type="button" className="btn btn-primary" onClick={() => setCreateTemplateModalOpen(true)}>➕ Create Template</button>
          </div>
          <div className="templates-grid">
            {MOCK_TEMPLATES.map((t) => (
              <div key={t.id} className="template-card">
                <div className="template-header">
                  <span className="template-icon">{t.icon || '📦'}</span>
                  <h3>{t.displayName || t.id}</h3>
                  <button type="button" className="icon-btn danger" style={{ marginLeft: 'auto' }} title="Delete Template">🗑️</button>
                </div>
                <p className="template-desc">{t.description || 'No description'}</p>
                <div className="template-meta">
                  <span>⏱️ TTL: {t.defaultTTL || '1h'}</span>
                  <span>📦 {t.components?.length || 0} component(s)</span>
                </div>
                <div className="template-tags">
                  {(t.tags || []).map((tag) => <span key={tag} className="tag">{tag}</span>)}
                </div>
                <div className="template-components">
                  <strong>Components:</strong>
                  <ul>
                    {(t.components || []).map((c) => (
                      <li key={c.name}>{c.primary ? '⭐' : '📦'} {c.name} ({c.chart}:{c.version || 'latest'})</li>
                    ))}
                  </ul>
                </div>
                <div className="template-actions">
                  <button type="button" className="btn btn-primary" onClick={() => createFromTemplate(t.id, t.namespace)}>🚀 Create Environment</button>
                </div>
              </div>
            ))}
          </div>
        </div>
          </>
        )}
      </div>

      {/* Create Environment Modal - 1:1 with operator form */}
      <div className={`modal ${createModalOpen ? 'active' : ''}`} style={createModalOpen ? { display: 'flex' } : {}} onClick={() => setCreateModalOpen(false)}>
        <div className="modal-content modal-form-wide" onClick={(e) => e.stopPropagation()} style={{ maxHeight: '90vh', overflowY: 'auto' }}>
          <div className="modal-header">
            <h3 className="modal-title">Create Environment</h3>
            <button type="button" className="icon-btn" onClick={() => setCreateModalOpen(false)}>✕</button>
          </div>
          <form onSubmit={handleCreateEnv}>
            <div className="form-group">
              <label htmlFor="templateSelect">Use Template (Optional)</label>
              <select
                id="templateSelect"
                value={createForm.templateId}
                onChange={(e) => {
                  const t = MOCK_TEMPLATES.find((x) => x.id === e.target.value);
                  setCreateForm((f) => ({
                    ...f,
                    templateId: e.target.value,
                    templateNamespace: t?.namespace || 'ephemeral-system',
                    ttl: t?.defaultTTL || f.ttl,
                  }));
                }}
              >
                <option value="">-- Manual Configuration --</option>
                {MOCK_TEMPLATES.map((t) => (
                  <option key={t.id} value={t.id}>{t.icon || '📦'} {t.displayName} ({t.components?.length || 0} components)</option>
                ))}
              </select>
              <small style={{ color: 'var(--text-secondary)', fontSize: 12, marginTop: 4, display: 'block' }}>Select a template to auto-fill configuration or configure manually below</small>
            </div>
            <div className="form-row">
              <div className="form-group">
                <label htmlFor="envName">Environment Name *</label>
                <input id="envName" type="text" value={createForm.envName} onChange={(e) => setCreateForm((f) => ({ ...f, envName: e.target.value }))} placeholder="pr-123" pattern="[a-z0-9-]+" required />
              </div>
              <div className="form-group">
                <label htmlFor="ttl">TTL</label>
                <input id="ttl" type="text" value={createForm.ttl} onChange={(e) => setCreateForm((f) => ({ ...f, ttl: e.target.value }))} placeholder="1h, 30m, 24h" />
              </div>
            </div>
            <details style={{ marginBottom: 12 }}>
              <summary className="details-summary">🎯 Helm Configuration</summary>
              <div className="form-row">
                <div className="form-group">
                  <label htmlFor="helmRepo">Repository</label>
                  <input id="helmRepo" type="text" value={createForm.helmRepo} onChange={(e) => setCreateForm((f) => ({ ...f, helmRepo: e.target.value }))} placeholder="https://charts.example.com" />
                </div>
                <div className="form-group">
                  <label htmlFor="helmChart">Chart</label>
                  <input id="helmChart" type="text" value={createForm.helmChart} onChange={(e) => setCreateForm((f) => ({ ...f, helmChart: e.target.value }))} placeholder="my-app" />
                </div>
              </div>
              <div className="form-group">
                <label htmlFor="helmVersion">Chart Version</label>
                <input id="helmVersion" type="text" value={createForm.helmVersion} onChange={(e) => setCreateForm((f) => ({ ...f, helmVersion: e.target.value }))} placeholder="1.0.0" />
              </div>
            </details>
            <details style={{ marginBottom: 12 }}>
              <summary className="details-summary">🌐 Gateway &amp; networking</summary>
              <small style={{ color: 'var(--text-secondary)', fontSize: 12, marginBottom: 12, display: 'block' }}>Defaults: main-gateway, default ns. Expand only if you need different gateway or discovery.</small>
              <div className="form-row">
                <div className="form-group">
                  <label htmlFor="gatewayName">Gateway Name</label>
                  <input id="gatewayName" type="text" value={createForm.gatewayName} onChange={(e) => setCreateForm((f) => ({ ...f, gatewayName: e.target.value }))} placeholder="main-gateway" />
                </div>
                <div className="form-group">
                  <label htmlFor="gatewayNamespace">Gateway Namespace</label>
                  <input id="gatewayNamespace" type="text" value={createForm.gatewayNamespace} onChange={(e) => setCreateForm((f) => ({ ...f, gatewayNamespace: e.target.value }))} placeholder="default" />
                </div>
              </div>
              <div className="form-row">
                <div className="form-group">
                  <label htmlFor="discoveryPort">Discover by port</label>
                  <input id="discoveryPort" type="number" value={createForm.discoveryPort} onChange={(e) => setCreateForm((f) => ({ ...f, discoveryPort: e.target.value }))} placeholder="optional" min={1} max={65535} />
                </div>
                <div className="form-group">
                  <label htmlFor="serviceName">Service Name</label>
                  <input id="serviceName" type="text" value={createForm.serviceName} onChange={(e) => setCreateForm((f) => ({ ...f, serviceName: e.target.value }))} placeholder="my-app" />
                </div>
              </div>
              <div className="form-group">
                <label htmlFor="servicePort">Target Port</label>
                <input id="servicePort" type="number" value={createForm.servicePort} onChange={(e) => setCreateForm((f) => ({ ...f, servicePort: Number(e.target.value) }))} min={1} max={65535} />
              </div>
            </details>
            <div className="form-actions">
              <button type="button" className="btn btn-secondary" onClick={() => setCreateModalOpen(false)}>Cancel</button>
              <button type="submit" className="btn btn-primary">Create</button>
            </div>
          </form>
        </div>
      </div>

      {/* Extend TTL Modal */}
      <div className={`modal ${extendModalOpen ? 'active' : ''}`} style={extendModalOpen ? { display: 'flex' } : {}} onClick={closeExtend}>
        <div className="modal-content" onClick={(e) => e.stopPropagation()}>
          <div className="modal-header">
            <h3 className="modal-title">Extend TTL</h3>
            <button type="button" className="icon-btn" onClick={closeExtend}>✕</button>
          </div>
          <form onSubmit={handleExtend}>
            <div className="form-group">
              <input type="hidden" name="envName" value={extendEnvName} />
              <label htmlFor="extendDuration">Extend by</label>
              <select id="extendDuration" name="duration">
                <option value="30m">30 minutes</option>
                <option value="1h">1 hour</option>
                <option value="2h" selected>2 hours</option>
                <option value="4h">4 hours</option>
                <option value="12h">12 hours</option>
                <option value="24h">24 hours</option>
              </select>
            </div>
            <div className="form-actions">
              <button type="button" className="btn btn-secondary" onClick={closeExtend}>Cancel</button>
              <button type="submit" className="btn btn-primary">Extend</button>
            </div>
          </form>
        </div>
      </div>

      {/* Create Template Modal - simplified for demo */}
      <div className={`modal ${createTemplateModalOpen ? 'active' : ''}`} style={createTemplateModalOpen ? { display: 'flex' } : {}} onClick={() => setCreateTemplateModalOpen(false)}>
        <div className="modal-content modal-form-wide" style={{ maxWidth: 700, maxHeight: '90vh', overflowY: 'auto' }} onClick={(e) => e.stopPropagation()}>
          <div className="modal-header">
            <h3 className="modal-title">Create Template</h3>
            <button type="button" className="icon-btn" onClick={() => setCreateTemplateModalOpen(false)}>✕</button>
          </div>
          <form onSubmit={(e) => { e.preventDefault(); setCreateTemplateModalOpen(false); setToast({ message: 'Template created (demo).' }); }}>
            <div className="form-row">
              <div className="form-group">
                <label>Template Name *</label>
                <input type="text" placeholder="my-template" required />
              </div>
              <div className="form-group">
                <label>Display Name</label>
                <input type="text" placeholder="My Template" />
              </div>
            </div>
            <div className="form-group">
              <label>Description</label>
              <textarea rows={2} placeholder="Describe what this template deploys..." style={{ width: '100%', resize: 'vertical' }} />
            </div>
            <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr 1fr', gap: 16 }}>
              <div className="form-group">
                <label>Icon (Emoji)</label>
                <input type="text" placeholder="🚀" maxLength={4} />
              </div>
              <div className="form-group">
                <label>Default TTL</label>
                <input type="text" defaultValue="1h" placeholder="1h, 2h, 24h" />
              </div>
              <div className="form-group">
                <label>Namespace</label>
                <input type="text" defaultValue="ephemeral-system" placeholder="ephemeral-system" />
              </div>
            </div>
            <div className="form-group">
              <label>Tags (comma separated)</label>
              <input type="text" placeholder="web, database, fullstack" />
            </div>
            <h4 style={{ margin: '20px 0 12px', color: 'var(--accent-blue)' }}>📦 Components</h4>
            <p style={{ color: 'var(--text-secondary)', fontSize: 13 }}>Add components in the real dashboard. This is a demo.</p>
            <div className="form-actions">
              <button type="button" className="btn btn-secondary" onClick={() => setCreateTemplateModalOpen(false)}>Cancel</button>
              <button type="submit" className="btn btn-primary">Create Template</button>
            </div>
          </form>
        </div>
      </div>

      <footer style={{ textAlign: 'center', padding: 24, color: 'var(--text-secondary)', fontSize: 12, borderTop: '1px solid var(--border-color)' }}>
        Ephemeral Operator © 2026 | Kubernetes Native Preview Environments
      </footer>

      {toast && <div className="demo-operator-root"><Toast message={toast.message} onClose={() => setToast(null)} /></div>}
    </div>
  );
}
