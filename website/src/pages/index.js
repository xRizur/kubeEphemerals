import React, { useRef, useState } from 'react';
import Layout from '@theme/Layout';
import Link from '@docusaurus/Link';
import useBaseUrl from '@docusaurus/useBaseUrl';
import DemoDashboard from '@site/src/components/DemoDashboard';

const K8S_BLUE = '#326ce5';

const valueProps = [
  { title: 'Safety', description: 'Automatic Resource Quotas & Limits', icon: 'icon-safety.svg' },
  { title: 'Savings', description: 'Sleep Mode & Auto-TTL', icon: 'icon-savings.svg' },
  { title: 'Observability', description: 'Logs per environment', icon: 'icon-observability.svg' },
  { title: 'Kubeconfig', description: 'Per-env kubeconfig download in the UI; short-lived token & RBAC for kubectl', icon: 'icon-kubeconfig.svg', docLink: '/docs/kubeconfig' },
];

const YAML_EPHEMERAL_ENV = `apiVersion: ephemeral.ephemeralenv.io/v1alpha1
kind: EphemeralEnv
metadata:
  name: pr-123-demo
  labels:
    pr-number: "123"
spec:
  ttl: "2h"
  isolation: true
  helm:
    repository: "https://stefanprodan.github.io/podinfo"
    chart: "podinfo"
    version: "6.9.4"
    values:
      replicaCount: 2
      extraEnvs:
        - name: PODINFO_UI_MESSAGE
          value: "PR #123 - Preview"
  gateway:
    name: "main-gateway"
    namespace: "default"
    domainPrefix: "pr-123"
    serviceName: "env-pr-123-demo-podinfo"
    targetPort: 9898
`;

const YAML_TEMPLATE = `apiVersion: ephemeral.ephemeralenv.io/v1alpha1
kind: EnvironmentTemplate
metadata:
  name: fullstack-webapp
  namespace: ephemeral-system
spec:
  displayName: Full Stack Web Application
  description: Frontend + API + PostgreSQL
  defaultTTL: "4h"
  components:
    - name: frontend
      repository: https://charts.bitnami.com/bitnami
      chart: nginx
      version: "15.0.0"
      serviceName: frontend-nginx
      servicePort: 80
      primary: true
    - name: database
      repository: https://charts.bitnami.com/bitnami
      chart: postgresql
      version: "12.0.0"
      serviceName: postgresql
      servicePort: 5432
      primary: false
`;

function HeroSection({ demoRef }) {
  const baseUrl = useBaseUrl('img/');
  const scrollToDemo = () => {
    demoRef.current?.scrollIntoView({ behavior: 'smooth' });
  };

  return (
    <header
      className="landing-hero"
      style={{
        padding: '80px 24px 60px',
        textAlign: 'center',
        background: 'linear-gradient(180deg, rgba(50, 108, 229, 0.06) 0%, transparent 50%)',
      }}
    >
      <img
        src={baseUrl + 'logo.png'}
        alt="Ephemeral Operator"
        className="landing-fade-in"
        style={{ height: 80, marginBottom: 24, objectFit: 'contain' }}
      />
      <h1
        className="landing-fade-in landing-delay-1"
        style={{
          fontSize: 'clamp(2rem, 5vw, 3rem)',
          marginBottom: 16,
          color: 'var(--ifm-heading-color)',
          fontWeight: 700,
        }}
      >
        Ephemeral Operator
      </h1>
      <p
        className="landing-fade-in landing-delay-2"
        style={{
          fontSize: 'clamp(1.1rem, 2.5vw, 1.35rem)',
          color: 'var(--ifm-font-color-secondary)',
          maxWidth: 560,
          margin: '0 auto 32px',
          lineHeight: 1.5,
        }}
      >
        Instant, Isolated Kubernetes Environments for every PR.
      </p>
      <div
        className="landing-fade-in landing-delay-3"
        style={{ display: 'flex', flexWrap: 'wrap', gap: 16, justifyContent: 'center' }}
      >
        <Link
          to="/docs/intro"
          className="button button--primary button--lg"
          style={{
            backgroundColor: K8S_BLUE,
            borderColor: K8S_BLUE,
          }}
        >
          Get Started
        </Link>
        <button
          type="button"
          onClick={scrollToDemo}
          className="button button--secondary button--lg"
          style={{
            borderColor: K8S_BLUE,
            color: K8S_BLUE,
          }}
        >
          View Demo
        </button>
      </div>
    </header>
  );
}

function ValuePropsSection() {
  const baseUrl = useBaseUrl('img/');

  return (
    <section className="landing-section landing-fade-in-up" style={{ padding: '60px 24px' }}>
      <h2
        style={{
          textAlign: 'center',
          marginBottom: 40,
          fontSize: 'clamp(1.5rem, 3vw, 2rem)',
          color: 'var(--ifm-heading-color)',
        }}
      >
        Why Ephemeral Operator?
      </h2>
      <div
        style={{
          display: 'grid',
          gridTemplateColumns: 'repeat(auto-fit, minmax(220px, 1fr))',
          gap: 24,
          maxWidth: 960,
          margin: '0 auto',
        }}
      >
        {valueProps.map(({ title, description, icon, docLink }) => {
          const cardContent = (
            <>
              <img
                src={baseUrl + icon}
                alt=""
                width={48}
                height={48}
                style={{ marginBottom: 12 }}
                aria-hidden
              />
              <h3 style={{ margin: '0 0 8px', fontSize: 18, color: 'var(--ifm-heading-color)' }}>
                {title}
              </h3>
              <p style={{ margin: 0, fontSize: 14, color: 'var(--ifm-font-color-secondary)', lineHeight: 1.5 }}>
                {description}
              </p>
              {docLink && (
                <span style={{ display: 'inline-block', marginTop: 8, fontSize: 13, color: 'var(--ifm-color-primary)' }}>
                  Docs →
                </span>
              )}
            </>
          );
          const cardStyle = {
            padding: 24,
            borderRadius: 12,
            border: '1px solid rgba(50, 108, 229, 0.2)',
            boxShadow: '0 4px 16px rgba(50, 108, 229, 0.06)',
            background: 'var(--ifm-background-surface-color)',
            textAlign: 'center',
            transition: 'transform 0.25s ease, box-shadow 0.25s ease',
          };
          return docLink ? (
            <Link
              key={title}
              to={docLink}
              className="landing-card"
              style={{ ...cardStyle, textDecoration: 'none', color: 'inherit' }}
            >
              {cardContent}
            </Link>
          ) : (
            <div key={title} className="landing-card" style={cardStyle}>
              {cardContent}
            </div>
          );
        })}
      </div>
    </section>
  );
}

function DemoSection({ demoRef }) {
  return (
    <section
      ref={demoRef}
      className="landing-section landing-fade-in-up"
      style={{
        padding: '60px 24px',
        background: 'var(--ifm-background-color)',
      }}
    >
      <h2
        style={{
          textAlign: 'center',
          marginBottom: 32,
          fontSize: 'clamp(1.5rem, 3vw, 2rem)',
          color: 'var(--ifm-heading-color)',
        }}
      >
        Try the Demo
      </h2>
      <p
        style={{
          textAlign: 'center',
          marginBottom: 40,
          color: 'var(--ifm-font-color-secondary)',
          maxWidth: 560,
          marginLeft: 'auto',
          marginRight: 'auto',
        }}
      >
        Simulated dashboard: create a new environment, click an env for details, logs, and config.
      </p>
      <DemoDashboard />
    </section>
  );
}

function YamlExamplesSection() {
  const [activeTab, setActiveTab] = useState('ephemeral');

  return (
    <section
      className="landing-section landing-fade-in-up"
      style={{
        padding: '60px 24px',
        background: 'var(--ifm-background-surface-color)',
      }}
    >
      <h2
        style={{
          textAlign: 'center',
          marginBottom: 16,
          fontSize: 'clamp(1.5rem, 3vw, 2rem)',
          color: 'var(--ifm-heading-color)',
        }}
      >
        How the backend works
      </h2>
      <p
        style={{
          textAlign: 'center',
          marginBottom: 32,
          color: 'var(--ifm-font-color-secondary)',
          maxWidth: 560,
          marginLeft: 'auto',
          marginRight: 'auto',
        }}
      >
        Define EphemeralEnvs and EnvironmentTemplates with Kubernetes YAML. The operator reconciles them into namespaces, Helm releases, and routes.
      </p>
      <div style={{ maxWidth: 800, margin: '0 auto' }}>
        <div
          style={{
            display: 'flex',
            gap: 0,
            borderBottom: '2px solid var(--ifm-toc-border-color)',
            marginBottom: 0,
          }}
        >
          <button
            type="button"
            onClick={() => setActiveTab('ephemeral')}
            style={{
              padding: '12px 24px',
              border: 'none',
              borderBottom: activeTab === 'ephemeral' ? `3px solid ${K8S_BLUE}` : '3px solid transparent',
              background: 'transparent',
              color: activeTab === 'ephemeral' ? K8S_BLUE : 'var(--ifm-font-color-secondary)',
              fontWeight: 600,
              cursor: 'pointer',
              fontSize: 14,
              marginBottom: '-2px',
            }}
          >
            EphemeralEnv
          </button>
          <button
            type="button"
            onClick={() => setActiveTab('template')}
            style={{
              padding: '12px 24px',
              border: 'none',
              borderBottom: activeTab === 'template' ? `3px solid ${K8S_BLUE}` : '3px solid transparent',
              background: 'transparent',
              color: activeTab === 'template' ? K8S_BLUE : 'var(--ifm-font-color-secondary)',
              fontWeight: 600,
              cursor: 'pointer',
              fontSize: 14,
              marginBottom: '-2px',
            }}
          >
            EnvironmentTemplate
          </button>
        </div>
        <div
          style={{
            padding: 20,
            borderRadius: '0 0 12px 12px',
            background: 'var(--ifm-code-background)',
            border: '1px solid var(--ifm-toc-border-color)',
            borderTop: 'none',
            overflow: 'auto',
          }}
        >
          {activeTab === 'ephemeral' && (
            <pre
              style={{
                margin: 0,
                fontSize: 13,
                lineHeight: 1.5,
                color: 'var(--ifm-font-color-base)',
                fontFamily: 'var(--ifm-font-family-monospace)',
                whiteSpace: 'pre',
              }}
            >
              <code>{YAML_EPHEMERAL_ENV}</code>
            </pre>
          )}
          {activeTab === 'template' && (
            <pre
              style={{
                margin: 0,
                fontSize: 13,
                lineHeight: 1.5,
                color: 'var(--ifm-font-color-base)',
                fontFamily: 'var(--ifm-font-family-monospace)',
                whiteSpace: 'pre',
              }}
            >
              <code>{YAML_TEMPLATE}</code>
            </pre>
          )}
        </div>
      </div>
    </section>
  );
}

function InstallSection() {
  return (
    <section
      className="landing-section landing-fade-in-up"
      style={{ padding: '60px 24px', background: 'var(--ifm-background-color)' }}
    >
      <h2
        style={{
          textAlign: 'center',
          marginBottom: 24,
          fontSize: 'clamp(1.5rem, 3vw, 2rem)',
          color: 'var(--ifm-heading-color)',
        }}
      >
        Installation
      </h2>
      <div
        style={{
          maxWidth: 720,
          margin: '0 auto',
          padding: 20,
          borderRadius: 12,
          background: 'var(--ifm-code-background)',
          border: '1px solid var(--ifm-toc-border-color)',
          overflow: 'auto',
        }}
      >
        <code style={{ fontSize: 14, wordBreak: 'break-all' }}>
          helm install my-op oci://ghcr.io/your-repo/charts/ephemeral-operator
        </code>
      </div>
      <p
        style={{
          textAlign: 'center',
          marginTop: 16,
          fontSize: 14,
          color: 'var(--ifm-font-color-secondary)',
        }}
      >
        Replace <code>your-repo</code> with your GitHub org (e.g. <code>maciekmm/kubeEphemerals</code>).
      </p>
    </section>
  );
}

export default function Home() {
  const demoRef = useRef(null);

  return (
    <Layout title="Ephemeral Operator" description="Instant, Isolated Kubernetes Environments for every PR.">
      <main>
        <HeroSection demoRef={demoRef} />
        <ValuePropsSection />
        <DemoSection demoRef={demoRef} />
        <YamlExamplesSection />
        <InstallSection />
      </main>
    </Layout>
  );
}
