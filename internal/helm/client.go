/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package helm

import (
	"context"
	"fmt"
	"os"
	"time"

	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/cli"
	"helm.sh/helm/v3/pkg/release"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// Client defines the interface for Helm operations
// This interface allows mocking in tests
type Client interface {
	// Install installs a Helm chart and returns the release name
	Install(ctx context.Context, opts InstallOptions) (*ReleaseInfo, error)

	// Uninstall removes a Helm release
	Uninstall(ctx context.Context, releaseName, namespace string) error

	// Status returns the status of a release
	Status(ctx context.Context, releaseName, namespace string) (*ReleaseInfo, error)

	// IsInstalled checks if a release exists
	IsInstalled(ctx context.Context, releaseName, namespace string) (bool, error)
}

// InstallOptions contains options for installing a Helm chart
type InstallOptions struct {
	ReleaseName string
	Namespace   string
	RepoURL     string
	ChartName   string
	Version     string
	Values      map[string]any
	Wait        bool
	Timeout     time.Duration
}

// ReleaseInfo contains information about a Helm release
type ReleaseInfo struct {
	Name      string
	Namespace string
	Version   int
	Status    string
	Chart     string
}

// SDKClient implements Client using the Helm SDK
type SDKClient struct {
	settings *cli.EnvSettings
}

// NewSDKClient creates a new Helm SDK client
func NewSDKClient() *SDKClient {
	return &SDKClient{
		settings: cli.New(),
	}
}

// getActionConfig creates an action configuration for a namespace
func (c *SDKClient) getActionConfig(namespace string) (*action.Configuration, error) {
	actionConfig := new(action.Configuration)

	// Use the default Kubernetes configuration
	if err := actionConfig.Init(c.settings.RESTClientGetter(), namespace, "secret", func(format string, v ...any) {
		// Log debug messages
		log.Log.V(1).Info(fmt.Sprintf(format, v...))
	}); err != nil {
		return nil, fmt.Errorf("failed to initialize helm action config: %w", err)
	}

	return actionConfig, nil
}

// Install installs a Helm chart
func (c *SDKClient) Install(ctx context.Context, opts InstallOptions) (*ReleaseInfo, error) {
	logger := log.FromContext(ctx)

	actionConfig, err := c.getActionConfig(opts.Namespace)
	if err != nil {
		return nil, err
	}

	// First, check if release already exists
	histClient := action.NewHistory(actionConfig)
	histClient.Max = 1
	if _, err := histClient.Run(opts.ReleaseName); err == nil {
		// Release exists, do an upgrade instead
		return c.upgrade(ctx, actionConfig, opts)
	}

	// Load chart from repository
	chartPath, err := c.downloadChart(opts.RepoURL, opts.ChartName, opts.Version)
	if err != nil {
		return nil, fmt.Errorf("failed to download chart: %w", err)
	}

	loadedChart, err := loader.Load(chartPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load chart: %w", err)
	}

	// Configure install
	install := action.NewInstall(actionConfig)
	install.Namespace = opts.Namespace
	install.ReleaseName = opts.ReleaseName
	install.CreateNamespace = false // Namespace should already exist
	install.Wait = opts.Wait
	if opts.Timeout > 0 {
		install.Timeout = opts.Timeout
	} else {
		install.Timeout = 5 * time.Minute
	}

	// Use values directly (already map[string]interface{})
	vals := opts.Values
	if vals == nil {
		vals = make(map[string]any)
	}

	logger.Info("Installing Helm chart",
		"release", opts.ReleaseName,
		"chart", opts.ChartName,
		"version", opts.Version,
		"namespace", opts.Namespace)

	rel, err := install.RunWithContext(ctx, loadedChart, vals)
	if err != nil {
		return nil, fmt.Errorf("failed to install chart: %w", err)
	}

	return releaseToInfo(rel), nil
}

// upgrade upgrades an existing release
func (c *SDKClient) upgrade(ctx context.Context, actionConfig *action.Configuration, opts InstallOptions) (*ReleaseInfo, error) {
	logger := log.FromContext(ctx)

	chartPath, err := c.downloadChart(opts.RepoURL, opts.ChartName, opts.Version)
	if err != nil {
		return nil, fmt.Errorf("failed to download chart: %w", err)
	}

	loadedChart, err := loader.Load(chartPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load chart: %w", err)
	}

	upgrade := action.NewUpgrade(actionConfig)
	upgrade.Namespace = opts.Namespace
	upgrade.Wait = opts.Wait
	if opts.Timeout > 0 {
		upgrade.Timeout = opts.Timeout
	} else {
		upgrade.Timeout = 5 * time.Minute
	}

	// Use values directly (already map[string]interface{})
	vals := opts.Values
	if vals == nil {
		vals = make(map[string]any)
	}

	logger.Info("Upgrading Helm release",
		"release", opts.ReleaseName,
		"chart", opts.ChartName,
		"version", opts.Version)

	rel, err := upgrade.RunWithContext(ctx, opts.ReleaseName, loadedChart, vals)
	if err != nil {
		return nil, fmt.Errorf("failed to upgrade release: %w", err)
	}

	return releaseToInfo(rel), nil
}

// downloadChart downloads a chart from a repository
func (c *SDKClient) downloadChart(repoURL, chartName, version string) (string, error) {
	// Create a temporary directory for chart downloads
	tmpDir, err := os.MkdirTemp("", "helm-chart-")
	if err != nil {
		return "", fmt.Errorf("failed to create temp dir: %w", err)
	}

	// Use Helm's pull action which handles all the complexity
	pull := action.NewPullWithOpts(action.WithConfig(&action.Configuration{}))
	pull.RepoURL = repoURL
	pull.Version = version
	pull.DestDir = tmpDir
	pull.Settings = c.settings
	pull.Untar = false // Keep as tarball

	_, err = pull.Run(chartName)
	if err != nil {
		return "", fmt.Errorf("failed to pull chart %s from %s: %w", chartName, repoURL, err)
	}

	// Find the downloaded chart file
	files, err := os.ReadDir(tmpDir)
	if err != nil {
		return "", fmt.Errorf("failed to read temp dir: %w", err)
	}

	for _, f := range files {
		if !f.IsDir() && (f.Name() == chartName+"-"+version+".tgz" ||
			(len(f.Name()) > 4 && f.Name()[len(f.Name())-4:] == ".tgz")) {
			return tmpDir + "/" + f.Name(), nil
		}
	}

	return "", fmt.Errorf("chart file not found in %s after download", tmpDir)
}

// Uninstall removes a Helm release
func (c *SDKClient) Uninstall(ctx context.Context, releaseName, namespace string) error {
	logger := log.FromContext(ctx)

	actionConfig, err := c.getActionConfig(namespace)
	if err != nil {
		return err
	}

	uninstall := action.NewUninstall(actionConfig)
	uninstall.KeepHistory = false

	logger.Info("Uninstalling Helm release", "release", releaseName, "namespace", namespace)

	_, err = uninstall.Run(releaseName)
	if err != nil {
		return fmt.Errorf("failed to uninstall release: %w", err)
	}

	return nil
}

// Status returns the status of a release
func (c *SDKClient) Status(ctx context.Context, releaseName, namespace string) (*ReleaseInfo, error) {
	actionConfig, err := c.getActionConfig(namespace)
	if err != nil {
		return nil, err
	}

	status := action.NewStatus(actionConfig)
	rel, err := status.Run(releaseName)
	if err != nil {
		return nil, fmt.Errorf("failed to get release status: %w", err)
	}

	return releaseToInfo(rel), nil
}

// IsInstalled checks if a release exists
func (c *SDKClient) IsInstalled(ctx context.Context, releaseName, namespace string) (bool, error) {
	actionConfig, err := c.getActionConfig(namespace)
	if err != nil {
		return false, err
	}

	histClient := action.NewHistory(actionConfig)
	histClient.Max = 1
	_, err = histClient.Run(releaseName)
	if err != nil {
		return false, nil // Release doesn't exist
	}

	return true, nil
}

// releaseToInfo converts a Helm release to ReleaseInfo
func releaseToInfo(rel *release.Release) *ReleaseInfo {
	chartName := ""
	if rel.Chart != nil && rel.Chart.Metadata != nil {
		chartName = rel.Chart.Metadata.Name
	}

	return &ReleaseInfo{
		Name:      rel.Name,
		Namespace: rel.Namespace,
		Version:   rel.Version,
		Status:    string(rel.Info.Status),
		Chart:     chartName,
	}
}

// Ensure SDKClient implements Client interface
var _ Client = (*SDKClient)(nil)

// MockClient is a mock implementation for testing
type MockClient struct {
	InstallFunc     func(ctx context.Context, opts InstallOptions) (*ReleaseInfo, error)
	UninstallFunc   func(ctx context.Context, releaseName, namespace string) error
	StatusFunc      func(ctx context.Context, releaseName, namespace string) (*ReleaseInfo, error)
	IsInstalledFunc func(ctx context.Context, releaseName, namespace string) (bool, error)

	// Track calls for assertions
	InstallCalls   []InstallOptions
	UninstallCalls []struct{ ReleaseName, Namespace string }
}

// NewMockClient creates a new mock client with default implementations
func NewMockClient() *MockClient {
	return &MockClient{
		InstallCalls:   make([]InstallOptions, 0),
		UninstallCalls: make([]struct{ ReleaseName, Namespace string }, 0),
	}
}

func (m *MockClient) Install(ctx context.Context, opts InstallOptions) (*ReleaseInfo, error) {
	m.InstallCalls = append(m.InstallCalls, opts)
	if m.InstallFunc != nil {
		return m.InstallFunc(ctx, opts)
	}
	return &ReleaseInfo{
		Name:      opts.ReleaseName,
		Namespace: opts.Namespace,
		Version:   1,
		Status:    "deployed",
		Chart:     opts.ChartName,
	}, nil
}

func (m *MockClient) Uninstall(ctx context.Context, releaseName, namespace string) error {
	m.UninstallCalls = append(m.UninstallCalls, struct{ ReleaseName, Namespace string }{releaseName, namespace})
	if m.UninstallFunc != nil {
		return m.UninstallFunc(ctx, releaseName, namespace)
	}
	return nil
}

func (m *MockClient) Status(ctx context.Context, releaseName, namespace string) (*ReleaseInfo, error) {
	if m.StatusFunc != nil {
		return m.StatusFunc(ctx, releaseName, namespace)
	}
	return &ReleaseInfo{
		Name:      releaseName,
		Namespace: namespace,
		Version:   1,
		Status:    "deployed",
	}, nil
}

func (m *MockClient) IsInstalled(ctx context.Context, releaseName, namespace string) (bool, error) {
	if m.IsInstalledFunc != nil {
		return m.IsInstalledFunc(ctx, releaseName, namespace)
	}
	return len(m.InstallCalls) > 0, nil
}

// Ensure MockClient implements Client interface
var _ Client = (*MockClient)(nil)
