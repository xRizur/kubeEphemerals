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

package ui

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	ephemeralv1alpha1 "github.com/maciekmm/kubeEphemerals/api/v1alpha1"
)

//go:embed templates/*.html
var templateFS embed.FS

//go:embed static/*
var staticFS embed.FS

var log = logf.Log.WithName("ui-server")

// Config holds the UI server configuration
type Config struct {
	// PlatformDomain is the domain for the global dashboard (e.g., "platform.local")
	PlatformDomain string
	// AdminPrefix is the prefix for admin routes (e.g., "admin")
	AdminPrefix string
	// BaseDomain is the base domain for environments (e.g., "preview.example.com")
	BaseDomain string
	// ListenAddr is the address to listen on (e.g., ":8080")
	ListenAddr string
}

// DefaultConfig returns a default configuration
func DefaultConfig() Config {
	return Config{
		PlatformDomain: "platform.local",
		AdminPrefix:    "admin",
		BaseDomain:     "preview.example.com",
		ListenAddr:     ":8080",
	}
}

// Server is the UI server that serves dashboards
type Server struct {
	config           Config
	client           client.Client
	kubeClient       kubernetes.Interface
	router           *mux.Router
	templates        *template.Template
	templateRegistry *TemplateRegistry
	upgrader         websocket.Upgrader
	server           *http.Server
	mu               sync.RWMutex
}

// NewServer creates a new UI server
func NewServer(cfg Config, c client.Client, kubeClient kubernetes.Interface) (*Server, error) {
	s := &Server{
		config:           cfg,
		client:           c,
		kubeClient:       kubeClient,
		templateRegistry: NewTemplateRegistry(),
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				return true // Allow all origins for development
			},
		},
	}

	// Parse templates - parse all templates together so they can reference each other
	tmpl := template.New("").Funcs(s.templateFuncs())

	// Read all template files
	entries, err := templateFS.ReadDir("templates")
	if err != nil {
		return nil, fmt.Errorf("failed to read templates directory: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		content, err := templateFS.ReadFile("templates/" + entry.Name())
		if err != nil {
			return nil, fmt.Errorf("failed to read template %s: %w", entry.Name(), err)
		}
		_, err = tmpl.New(entry.Name()).Parse(string(content))
		if err != nil {
			return nil, fmt.Errorf("failed to parse template %s: %w", entry.Name(), err)
		}
	}
	s.templates = tmpl

	// Setup router
	s.setupRouter()

	return s, nil
}

// templateFuncs returns custom template functions
func (s *Server) templateFuncs() template.FuncMap {
	return template.FuncMap{
		"timeUntil": func(t *metav1.Time) string {
			if t == nil {
				return "N/A"
			}
			d := time.Until(t.Time)
			if d < 0 {
				return "Expired"
			}
			return d.Round(time.Second).String()
		},
		"timeSince": func(t *metav1.Time) string {
			if t == nil {
				return "N/A"
			}
			return time.Since(t.Time).Round(time.Second).String()
		},
		"statusClass": func(phase ephemeralv1alpha1.EphemeralEnvPhase) string {
			switch phase {
			case ephemeralv1alpha1.PhaseActive:
				return "status-active"
			case ephemeralv1alpha1.PhasePending:
				return "status-pending"
			case ephemeralv1alpha1.PhaseFailed:
				return "status-failed"
			case ephemeralv1alpha1.PhaseExpired:
				return "status-expired"
			default:
				return "status-unknown"
			}
		},
		"json": func(v interface{}) string {
			b, _ := json.MarshalIndent(v, "", "  ")
			return string(b)
		},
	}
}

// setupRouter configures the HTTP router with host-based routing
func (s *Server) setupRouter() {
	s.router = mux.NewRouter()

	// Static files
	s.router.PathPrefix("/static/").Handler(http.StripPrefix("/static/", http.FileServer(http.FS(staticFS))))

	// Platform dashboard routes (global view)
	platformRouter := s.router.Host(s.config.PlatformDomain).Subrouter()
	platformRouter.HandleFunc("/", s.handleGlobalDashboard).Methods("GET")
	platformRouter.HandleFunc("/api/envs", s.handleListEnvs).Methods("GET")
	platformRouter.HandleFunc("/api/envs", s.handleCreateEnv).Methods("POST")
	platformRouter.HandleFunc("/api/envs/{name}", s.handleDeleteEnv).Methods("DELETE")
	platformRouter.HandleFunc("/api/envs/{name}", s.handleGetEnv).Methods("GET")

	// Admin dashboard routes (per-environment view)
	// Matches: admin.pr-123.preview.example.com or admin-pr-123.preview.example.com
	adminPattern := fmt.Sprintf("%s.{env:[a-z0-9-]+}.%s", s.config.AdminPrefix, s.config.BaseDomain)
	adminRouter := s.router.Host(adminPattern).Subrouter()
	adminRouter.Use(s.envContextMiddleware)
	adminRouter.HandleFunc("/", s.handleEnvDashboard).Methods("GET")
	adminRouter.HandleFunc("/api/status", s.handleEnvStatus).Methods("GET")
	adminRouter.HandleFunc("/api/pods", s.handleEnvPods).Methods("GET")
	adminRouter.HandleFunc("/api/pods/{pod}/restart", s.handleRestartPod).Methods("POST")
	adminRouter.HandleFunc("/api/logs/{pod}", s.handlePodLogs).Methods("GET")
	adminRouter.HandleFunc("/api/logs/{pod}/stream", s.handlePodLogsStream)
	adminRouter.HandleFunc("/api/extend-ttl", s.handleExtendTTL).Methods("POST")

	// Fallback - also accept requests without proper host header for development
	s.router.HandleFunc("/", s.handleGlobalDashboard).Methods("GET")
	s.router.HandleFunc("/env/{name}", s.handleEnvDashboardByName).Methods("GET")
	s.router.HandleFunc("/api/envs", s.handleListEnvs).Methods("GET")
	s.router.HandleFunc("/api/envs", s.handleCreateEnv).Methods("POST")
	s.router.HandleFunc("/api/envs/{name}", s.handleDeleteEnv).Methods("DELETE")
	s.router.HandleFunc("/api/envs/{name}", s.handleGetEnv).Methods("GET")
	s.router.HandleFunc("/api/envs/{name}/pods", s.handleEnvPodsByName).Methods("GET")
	s.router.HandleFunc("/api/envs/{name}/pods/{pod}/logs", s.handlePodLogsByName).Methods("GET")
	s.router.HandleFunc("/api/envs/{name}/pods/{pod}/logs/stream", s.handlePodLogsStreamByName)
	s.router.HandleFunc("/api/envs/{name}/extend-ttl", s.handleExtendTTLByName).Methods("POST")

	// Templates API - Service Catalog
	s.router.HandleFunc("/api/templates", s.handleListTemplates).Methods("GET")
	s.router.HandleFunc("/api/templates", s.handleCreateTemplate).Methods("POST")
	s.router.HandleFunc("/api/templates/{id}", s.handleGetTemplate).Methods("GET")
	s.router.HandleFunc("/api/templates/{id}", s.handleDeleteTemplate).Methods("DELETE")
}

// Start starts the UI server
func (s *Server) Start(ctx context.Context) error {
	s.server = &http.Server{
		Addr:    s.config.ListenAddr,
		Handler: s.router,
	}

	log.Info("Starting UI server", "addr", s.config.ListenAddr)

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		s.server.Shutdown(shutdownCtx)
	}()

	if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("UI server error: %w", err)
	}

	return nil
}

// envContextMiddleware extracts the environment name from the Host header
func (s *Server) envContextMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		envName := vars["env"]
		if envName == "" {
			// Try to extract from host header
			host := r.Host
			re := regexp.MustCompile(fmt.Sprintf(`%s\.([a-z0-9-]+)\.`, s.config.AdminPrefix))
			matches := re.FindStringSubmatch(host)
			if len(matches) > 1 {
				envName = matches[1]
			}
		}

		if envName != "" {
			ctx := context.WithValue(r.Context(), envContextKey, envName)
			r = r.WithContext(ctx)
		}

		next.ServeHTTP(w, r)
	})
}

type contextKey string

const envContextKey contextKey = "envName"

// getEnvFromContext retrieves the environment name from context
func getEnvFromContext(ctx context.Context) string {
	if v := ctx.Value(envContextKey); v != nil {
		return v.(string)
	}
	return ""
}

// =============================================================================
// Global Dashboard Handlers
// =============================================================================

func (s *Server) handleGlobalDashboard(w http.ResponseWriter, r *http.Request) {
	envList := &ephemeralv1alpha1.EphemeralEnvList{}
	if err := s.client.List(r.Context(), envList); err != nil {
		http.Error(w, "Failed to list environments", http.StatusInternalServerError)
		return
	}

	data := map[string]interface{}{
		"Title":        "Ephemeral Environments",
		"Environments": envList.Items,
		"Config":       s.config,
	}

	s.renderTemplate(w, "global_dashboard.html", data)
}

func (s *Server) handleListEnvs(w http.ResponseWriter, r *http.Request) {
	envList := &ephemeralv1alpha1.EphemeralEnvList{}
	if err := s.client.List(r.Context(), envList); err != nil {
		s.jsonError(w, "Failed to list environments", http.StatusInternalServerError)
		return
	}

	s.jsonResponse(w, envList.Items)
}

func (s *Server) handleCreateEnv(w http.ResponseWriter, r *http.Request) {
	var env ephemeralv1alpha1.EphemeralEnv
	if err := json.NewDecoder(r.Body).Decode(&env); err != nil {
		s.jsonError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if env.Namespace == "" {
		env.Namespace = "default"
	}

	if err := s.client.Create(r.Context(), &env); err != nil {
		s.jsonError(w, fmt.Sprintf("Failed to create environment: %v", err), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
	s.jsonResponse(w, env)
}

func (s *Server) handleDeleteEnv(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	name := vars["name"]

	env := &ephemeralv1alpha1.EphemeralEnv{}
	env.Name = name
	env.Namespace = "default" // TODO: support multiple namespaces

	if err := s.client.Delete(r.Context(), env); err != nil {
		s.jsonError(w, fmt.Sprintf("Failed to delete environment: %v", err), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleGetEnv(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	name := vars["name"]

	env := &ephemeralv1alpha1.EphemeralEnv{}
	if err := s.client.Get(r.Context(), client.ObjectKey{Name: name, Namespace: "default"}, env); err != nil {
		s.jsonError(w, "Environment not found", http.StatusNotFound)
		return
	}

	s.jsonResponse(w, env)
}

// =============================================================================
// Environment Dashboard Handlers
// =============================================================================

func (s *Server) handleEnvDashboard(w http.ResponseWriter, r *http.Request) {
	envName := getEnvFromContext(r.Context())
	s.renderEnvDashboard(w, r, envName)
}

func (s *Server) handleEnvDashboardByName(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	envName := vars["name"]
	s.renderEnvDashboard(w, r, envName)
}

func (s *Server) renderEnvDashboard(w http.ResponseWriter, r *http.Request, envName string) {
	env := &ephemeralv1alpha1.EphemeralEnv{}
	if err := s.client.Get(r.Context(), client.ObjectKey{Name: envName, Namespace: "default"}, env); err != nil {
		http.Error(w, "Environment not found", http.StatusNotFound)
		return
	}

	// Get pods in the environment namespace
	pods := &corev1.PodList{}
	if err := s.client.List(r.Context(), pods, client.InNamespace(env.Status.ActiveNamespace)); err != nil {
		log.Error(err, "Failed to list pods", "namespace", env.Status.ActiveNamespace)
	}

	data := map[string]interface{}{
		"Title":       fmt.Sprintf("Environment: %s", envName),
		"Environment": env,
		"Pods":        pods.Items,
		"Config":      s.config,
	}

	s.renderTemplate(w, "env_dashboard.html", data)
}

func (s *Server) handleEnvStatus(w http.ResponseWriter, r *http.Request) {
	envName := getEnvFromContext(r.Context())
	s.getEnvStatus(w, r, envName)
}

func (s *Server) getEnvStatus(w http.ResponseWriter, r *http.Request, envName string) {
	env := &ephemeralv1alpha1.EphemeralEnv{}
	if err := s.client.Get(r.Context(), client.ObjectKey{Name: envName, Namespace: "default"}, env); err != nil {
		s.jsonError(w, "Environment not found", http.StatusNotFound)
		return
	}

	status := map[string]interface{}{
		"name":           env.Name,
		"phase":          env.Status.Phase,
		"namespace":      env.Status.ActiveNamespace,
		"accessURL":      env.Status.AccessURL,
		"helmRelease":    env.Status.HelmRelease,
		"expirationTime": env.Status.ExpirationTime,
		"conditions":     env.Status.Conditions,
	}

	if env.Status.ExpirationTime != nil {
		status["ttlRemaining"] = time.Until(env.Status.ExpirationTime.Time).Round(time.Second).String()
	}

	s.jsonResponse(w, status)
}

func (s *Server) handleEnvPods(w http.ResponseWriter, r *http.Request) {
	envName := getEnvFromContext(r.Context())
	s.getEnvPods(w, r, envName)
}

func (s *Server) handleEnvPodsByName(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	envName := vars["name"]
	s.getEnvPods(w, r, envName)
}

func (s *Server) getEnvPods(w http.ResponseWriter, r *http.Request, envName string) {
	env := &ephemeralv1alpha1.EphemeralEnv{}
	if err := s.client.Get(r.Context(), client.ObjectKey{Name: envName, Namespace: "default"}, env); err != nil {
		s.jsonError(w, "Environment not found", http.StatusNotFound)
		return
	}

	pods := &corev1.PodList{}
	if err := s.client.List(r.Context(), pods, client.InNamespace(env.Status.ActiveNamespace)); err != nil {
		s.jsonError(w, "Failed to list pods", http.StatusInternalServerError)
		return
	}

	// Simplify pod data for JSON response
	podInfos := make([]map[string]interface{}, 0, len(pods.Items))
	for _, pod := range pods.Items {
		podInfo := map[string]interface{}{
			"name":      pod.Name,
			"namespace": pod.Namespace,
			"status":    string(pod.Status.Phase),
			"ready":     isPodReady(&pod),
			"restarts":  getPodRestarts(&pod),
			"age":       time.Since(pod.CreationTimestamp.Time).Round(time.Second).String(),
		}
		podInfos = append(podInfos, podInfo)
	}

	s.jsonResponse(w, podInfos)
}

func (s *Server) handleRestartPod(w http.ResponseWriter, r *http.Request) {
	envName := getEnvFromContext(r.Context())
	vars := mux.Vars(r)
	podName := vars["pod"]

	env := &ephemeralv1alpha1.EphemeralEnv{}
	if err := s.client.Get(r.Context(), client.ObjectKey{Name: envName, Namespace: "default"}, env); err != nil {
		s.jsonError(w, "Environment not found", http.StatusNotFound)
		return
	}

	// Delete the pod to trigger restart
	pod := &corev1.Pod{}
	pod.Name = podName
	pod.Namespace = env.Status.ActiveNamespace

	if err := s.client.Delete(r.Context(), pod); err != nil {
		s.jsonError(w, fmt.Sprintf("Failed to restart pod: %v", err), http.StatusInternalServerError)
		return
	}

	s.jsonResponse(w, map[string]string{"status": "restarting", "pod": podName})
}

func (s *Server) handlePodLogs(w http.ResponseWriter, r *http.Request) {
	envName := getEnvFromContext(r.Context())
	vars := mux.Vars(r)
	podName := vars["pod"]
	s.getPodLogs(w, r, envName, podName)
}

func (s *Server) handlePodLogsByName(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	envName := vars["name"]
	podName := vars["pod"]
	s.getPodLogs(w, r, envName, podName)
}

func (s *Server) getPodLogs(w http.ResponseWriter, r *http.Request, envName, podName string) {
	env := &ephemeralv1alpha1.EphemeralEnv{}
	if err := s.client.Get(r.Context(), client.ObjectKey{Name: envName, Namespace: "default"}, env); err != nil {
		s.jsonError(w, "Environment not found", http.StatusNotFound)
		return
	}

	if s.kubeClient == nil {
		s.jsonError(w, "Kubernetes client not available", http.StatusInternalServerError)
		return
	}

	tailLines := int64(100)
	opts := &corev1.PodLogOptions{
		TailLines: &tailLines,
	}

	req := s.kubeClient.CoreV1().Pods(env.Status.ActiveNamespace).GetLogs(podName, opts)
	stream, err := req.Stream(r.Context())
	if err != nil {
		s.jsonError(w, fmt.Sprintf("Failed to get logs: %v", err), http.StatusInternalServerError)
		return
	}
	defer stream.Close()

	w.Header().Set("Content-Type", "text/plain")
	io.Copy(w, stream)
}

func (s *Server) handlePodLogsStream(w http.ResponseWriter, r *http.Request) {
	envName := getEnvFromContext(r.Context())
	vars := mux.Vars(r)
	podName := vars["pod"]
	s.streamPodLogs(w, r, envName, podName)
}

func (s *Server) handlePodLogsStreamByName(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	envName := vars["name"]
	podName := vars["pod"]
	s.streamPodLogs(w, r, envName, podName)
}

func (s *Server) streamPodLogs(w http.ResponseWriter, r *http.Request, envName, podName string) {
	env := &ephemeralv1alpha1.EphemeralEnv{}
	if err := s.client.Get(r.Context(), client.ObjectKey{Name: envName, Namespace: "default"}, env); err != nil {
		http.Error(w, "Environment not found", http.StatusNotFound)
		return
	}

	// Upgrade to WebSocket
	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Error(err, "Failed to upgrade to WebSocket")
		return
	}
	defer conn.Close()

	if s.kubeClient == nil {
		conn.WriteMessage(websocket.TextMessage, []byte("Error: Kubernetes client not available"))
		return
	}

	opts := &corev1.PodLogOptions{
		Follow: true,
	}

	req := s.kubeClient.CoreV1().Pods(env.Status.ActiveNamespace).GetLogs(podName, opts)
	stream, err := req.Stream(r.Context())
	if err != nil {
		conn.WriteMessage(websocket.TextMessage, []byte(fmt.Sprintf("Error: %v", err)))
		return
	}
	defer stream.Close()

	// Stream logs to WebSocket
	buf := make([]byte, 4096)
	for {
		n, err := stream.Read(buf)
		if err != nil {
			if err != io.EOF {
				conn.WriteMessage(websocket.TextMessage, []byte(fmt.Sprintf("Error: %v", err)))
			}
			break
		}
		if n > 0 {
			if err := conn.WriteMessage(websocket.TextMessage, buf[:n]); err != nil {
				break
			}
		}
	}
}

func (s *Server) handleExtendTTL(w http.ResponseWriter, r *http.Request) {
	envName := getEnvFromContext(r.Context())
	s.extendTTL(w, r, envName)
}

func (s *Server) handleExtendTTLByName(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	envName := vars["name"]
	s.extendTTL(w, r, envName)
}

func (s *Server) extendTTL(w http.ResponseWriter, r *http.Request, envName string) {
	var req struct {
		Duration string `json:"duration"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.jsonError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	duration, err := time.ParseDuration(req.Duration)
	if err != nil {
		s.jsonError(w, "Invalid duration format", http.StatusBadRequest)
		return
	}

	env := &ephemeralv1alpha1.EphemeralEnv{}
	if err := s.client.Get(r.Context(), client.ObjectKey{Name: envName, Namespace: "default"}, env); err != nil {
		s.jsonError(w, "Environment not found", http.StatusNotFound)
		return
	}

	// Extend TTL by updating expiration time
	if env.Status.ExpirationTime != nil {
		newExpiration := env.Status.ExpirationTime.Add(duration)
		env.Status.ExpirationTime = &metav1.Time{Time: newExpiration}
	} else {
		newExpiration := time.Now().Add(duration)
		env.Status.ExpirationTime = &metav1.Time{Time: newExpiration}
	}

	if err := s.client.Status().Update(r.Context(), env); err != nil {
		s.jsonError(w, fmt.Sprintf("Failed to extend TTL: %v", err), http.StatusInternalServerError)
		return
	}

	s.jsonResponse(w, map[string]interface{}{
		"status":        "extended",
		"newExpiration": env.Status.ExpirationTime,
		"ttlRemaining":  time.Until(env.Status.ExpirationTime.Time).Round(time.Second).String(),
	})
}

// =============================================================================
// Helper Functions
// =============================================================================

func (s *Server) renderTemplate(w http.ResponseWriter, name string, data interface{}) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	// First lookup the named template, then execute it
	tmpl := s.templates.Lookup(name)
	if tmpl == nil {
		log.Error(fmt.Errorf("template not found"), "Failed to find template", "template", name)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	if err := tmpl.Execute(w, data); err != nil {
		log.Error(err, "Failed to render template", "template", name)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

func (s *Server) jsonResponse(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

func (s *Server) jsonError(w http.ResponseWriter, message string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": message})
}

func isPodReady(pod *corev1.Pod) bool {
	for _, cond := range pod.Status.Conditions {
		if cond.Type == corev1.PodReady && cond.Status == corev1.ConditionTrue {
			return true
		}
	}
	return false
}

func getPodRestarts(pod *corev1.Pod) int32 {
	var restarts int32
	for _, cs := range pod.Status.ContainerStatuses {
		restarts += cs.RestartCount
	}
	return restarts
}

// SanitizeEnvName sanitizes an environment name for use in hostnames
func SanitizeEnvName(name string) string {
	// Convert to lowercase and replace invalid characters
	name = strings.ToLower(name)
	name = strings.ReplaceAll(name, "_", "-")
	return name
}
