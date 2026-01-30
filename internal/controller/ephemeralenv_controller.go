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

package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"

	ephemeralv1alpha1 "github.com/maciekmm/kubeEphemerals/api/v1alpha1"
	"github.com/maciekmm/kubeEphemerals/internal/helm"
)

// Component status constants
const (
	statusError    = "Error"
	statusDeployed = "Deployed"
)

// EphemeralEnvReconciler reconciles a EphemeralEnv object
type EphemeralEnvReconciler struct {
	client.Client
	Scheme            *runtime.Scheme
	HelmClient        helm.Client
	OperatorService   string // Service name for the operator UI (for admin HTTPRoute)
	OperatorNamespace string // Namespace where the operator is deployed
}

// Finalizer name for cleanup
const ephemeralEnvFinalizer = "ephemeral.ephemeralenv.io/finalizer"

// Labels used by the operator
const (
	LabelManagedBy = "app.kubernetes.io/managed-by"
	LabelOwner     = "ephemeral.ephemeralenv.io/owner"
	LabelRouteType = "ephemeral.ephemeralenv.io/route-type"
	ManagedByValue = "ephemeral-operator"

	// BaseDomain is the base domain for preview environments
	BaseDomain = "preview.example.com"

	// RouteType labels for different HTTPRoutes
	RouteTypeApp   = "app"
	RouteTypeAdmin = "admin"

	// Developer kubeconfig RBAC: SA and Role names in each env namespace
	DeveloperAccessSA     = "developer-access"
	NSAdminRoleName       = "ns-admin"
	DeveloperAccessBindingName = "developer-access-ns-admin"
)

// +kubebuilder:rbac:groups=ephemeral.ephemeralenv.io,resources=ephemeralenvs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=ephemeral.ephemeralenv.io,resources=ephemeralenvs/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=ephemeral.ephemeralenv.io,resources=ephemeralenvs/finalizers,verbs=update
// +kubebuilder:rbac:groups=ephemeral.ephemeralenv.io,resources=environmenttemplates,verbs=get;list;watch
// +kubebuilder:rbac:groups=ephemeral.ephemeralenv.io,resources=environmenttemplates/status,verbs=get;update;patch
// +kubebuilder:rbac:groups="",resources=namespaces,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list
// +kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=serviceaccounts,verbs=get;list;watch;create;update;patch
// +kubebuilder:rbac:groups="",resources=serviceaccounts/token,verbs=create
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=roles,verbs=get;list;watch;create;update;patch
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=rolebindings,verbs=get;list;watch;create;update;patch
// +kubebuilder:rbac:groups=networking.k8s.io,resources=networkpolicies,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=httproutes,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=referencegrants,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *EphemeralEnvReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := logf.FromContext(ctx)
	logger.Info("Reconciling EphemeralEnv", "namespacedName", req.NamespacedName)

	// Step 1: Fetch the EphemeralEnv instance
	ephemeralEnv := &ephemeralv1alpha1.EphemeralEnv{}
	if err := r.Get(ctx, req.NamespacedName, ephemeralEnv); err != nil {
		if apierrors.IsNotFound(err) {
			// CR was deleted, nothing to do
			logger.Info("EphemeralEnv resource not found, likely deleted")
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, fmt.Errorf("failed to fetch EphemeralEnv: %w", err)
	}

	// Step 2: Handle deletion - cleanup all resources when CR is deleted
	if !ephemeralEnv.DeletionTimestamp.IsZero() {
		logger.Info("EphemeralEnv is being deleted, running cleanup")
		return r.handleDeletion(ctx, ephemeralEnv)
	}

	// Step 3: Add finalizer if not present
	if !controllerutil.ContainsFinalizer(ephemeralEnv, ephemeralEnvFinalizer) {
		logger.Info("Adding finalizer to EphemeralEnv")
		controllerutil.AddFinalizer(ephemeralEnv, ephemeralEnvFinalizer)
		if err := r.Update(ctx, ephemeralEnv); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to add finalizer: %w", err)
		}
		// Re-fetch the object after update to get the latest version
		if err := r.Get(ctx, req.NamespacedName, ephemeralEnv); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to re-fetch EphemeralEnv after adding finalizer: %w", err)
		}
	}

	// Step 4: Initialize status if needed
	if ephemeralEnv.Status.Phase == "" {
		ephemeralEnv.Status.Phase = ephemeralv1alpha1.PhasePending
	}

	// Step 5: Calculate and set expiration time if not set
	if ephemeralEnv.Status.ExpirationTime == nil {
		expirationTime := ephemeralEnv.CalculateExpirationTime()
		ephemeralEnv.Status.ExpirationTime = &expirationTime
		logger.Info("Set expiration time", "expirationTime", expirationTime.Time)
	}

	// Step 6: Check TTL expiration
	if ephemeralEnv.IsExpired() {
		logger.Info("EphemeralEnv has expired, initiating cleanup")
		return r.handleExpiration(ctx, ephemeralEnv)
	}

	// Step 7: Ensure namespace exists
	if err := r.ensureNamespace(ctx, ephemeralEnv); err != nil {
		logger.Error(err, "Failed to ensure namespace")
		ephemeralEnv.Status.Phase = ephemeralv1alpha1.PhaseFailed
		ephemeralEnv.Status.Message = fmt.Sprintf("Failed to create namespace: %v", err)
		if statusErr := r.Status().Update(ctx, ephemeralEnv); statusErr != nil {
			logger.Error(statusErr, "Failed to update status after namespace error")
		}
		return ctrl.Result{}, err
	}

	// Step 7b: Ensure developer-access RBAC (SA, Role, RoleBinding) for kubeconfig self-service
	if err := r.ensureDeveloperAccessRBAC(ctx, ephemeralEnv); err != nil {
		logger.Error(err, "Failed to ensure developer-access RBAC")
		ephemeralEnv.Status.Phase = ephemeralv1alpha1.PhaseFailed
		ephemeralEnv.Status.Message = fmt.Sprintf("Failed to create developer RBAC: %v", err)
		if statusErr := r.Status().Update(ctx, ephemeralEnv); statusErr != nil {
			logger.Error(statusErr, "Failed to update status after RBAC error")
		}
		return ctrl.Result{}, err
	}

	// Step 8: Ensure NetworkPolicy if isolation is enabled
	if err := r.ensureNetworkPolicy(ctx, ephemeralEnv); err != nil {
		logger.Error(err, "Failed to ensure NetworkPolicy")
		ephemeralEnv.Status.Phase = ephemeralv1alpha1.PhaseFailed
		ephemeralEnv.Status.Message = fmt.Sprintf("Failed to create NetworkPolicy: %v", err)
		if statusErr := r.Status().Update(ctx, ephemeralEnv); statusErr != nil {
			logger.Error(statusErr, "Failed to update status after NetworkPolicy error")
		}
		return ctrl.Result{}, err
	}

	// Step 9: Ensure ReferenceGrant for cross-namespace routing
	if err := r.ensureReferenceGrant(ctx, ephemeralEnv); err != nil {
		logger.Error(err, "Failed to ensure ReferenceGrant")
		ephemeralEnv.Status.Phase = ephemeralv1alpha1.PhaseFailed
		ephemeralEnv.Status.Message = fmt.Sprintf("Failed to create ReferenceGrant: %v", err)
		if statusErr := r.Status().Update(ctx, ephemeralEnv); statusErr != nil {
			logger.Error(statusErr, "Failed to update status after ReferenceGrant error")
		}
		return ctrl.Result{}, err
	}

	// Step 10-12: Handle Helm deployment and HTTPRoute creation
	// If ServicePort is set, we need to deploy Helm first, then discover the service
	effectiveServiceName := ephemeralEnv.Spec.Gateway.ServiceName

	if ephemeralEnv.Spec.ServicePort != nil {
		// Port-based discovery mode: Deploy Helm first, then discover service
		logger.Info("Using port-based service discovery", "port", *ephemeralEnv.Spec.ServicePort)

		// Step 10a: Deploy Helm chart first (so services are created) if spec has helm/components
		if r.HelmClient != nil {
			components := r.resolveComponents(ctx, ephemeralEnv)
			if len(components) > 0 {
				if err := r.ensureHelmRelease(ctx, ephemeralEnv); err != nil {
					logger.Error(err, "Failed to ensure Helm release")
					ephemeralEnv.Status.Phase = ephemeralv1alpha1.PhaseFailed
					ephemeralEnv.Status.Message = fmt.Sprintf("Failed to deploy Helm chart: %v", err)
					meta.SetStatusCondition(&ephemeralEnv.Status.Conditions, metav1.Condition{
						Type:               ephemeralv1alpha1.ConditionTypeHelmDeployed,
						Status:             metav1.ConditionFalse,
						Reason:             "HelmDeploymentFailed",
						Message:            fmt.Sprintf("Helm deployment failed: %v", err),
						LastTransitionTime: metav1.Now(),
					})
					if statusErr := r.Status().Update(ctx, ephemeralEnv); statusErr != nil {
						logger.Error(statusErr, "Failed to update status after Helm error")
					}
					return ctrl.Result{}, err
				}
			}
		}

		// Step 10b: Discover service by port
		discoveredService, err := r.discoverService(ctx, ephemeralEnv, *ephemeralEnv.Spec.ServicePort)
		if err != nil {
			logger.Info("Service discovery pending, will retry", "error", err.Error())
			ephemeralEnv.Status.Message = fmt.Sprintf("Waiting for service discovery: %v", err)
			if statusErr := r.Status().Update(ctx, ephemeralEnv); statusErr != nil {
				logger.Error(statusErr, "Failed to update status")
			}
			// Requeue after short delay to allow services to be created
			return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
		}
		effectiveServiceName = discoveredService
		logger.Info("Service discovered", "serviceName", effectiveServiceName, "port", *ephemeralEnv.Spec.ServicePort)
	}
	// else: Traditional mode - Deploy Helm after HTTPRoute (existing behavior)

	// Step 10c/10: Ensure HTTPRoute for Gateway API routing
	if err := r.ensureHTTPRoute(ctx, ephemeralEnv, effectiveServiceName); err != nil {
		logger.Error(err, "Failed to ensure HTTPRoute")
		ephemeralEnv.Status.Phase = ephemeralv1alpha1.PhaseFailed
		ephemeralEnv.Status.Message = fmt.Sprintf("Failed to create HTTPRoute: %v", err)
		if statusErr := r.Status().Update(ctx, ephemeralEnv); statusErr != nil {
			logger.Error(statusErr, "Failed to update status after HTTPRoute error")
		}
		return ctrl.Result{}, err
	}

	// Step 11: Ensure Admin HTTPRoute for dashboard UI (admin.pr-123.domain.com -> Operator UI)
	if r.OperatorService != "" && r.OperatorNamespace != "" {
		if err := r.ensureAdminHTTPRoute(ctx, ephemeralEnv, r.OperatorService, r.OperatorNamespace); err != nil {
			logger.Error(err, "Failed to ensure Admin HTTPRoute")
			// Non-fatal: log but don't fail the reconciliation
			// The admin dashboard is optional - app routing is what matters
		}
	}

	// Step 12: Deploy Helm chart if HelmClient is available and spec has helm/components (only if not already deployed above)
	if ephemeralEnv.Spec.ServicePort == nil && r.HelmClient != nil {
		components := r.resolveComponents(ctx, ephemeralEnv)
		if len(components) > 0 {
			if err := r.ensureHelmRelease(ctx, ephemeralEnv); err != nil {
				logger.Error(err, "Failed to ensure Helm release")
				ephemeralEnv.Status.Phase = ephemeralv1alpha1.PhaseFailed
				ephemeralEnv.Status.Message = fmt.Sprintf("Failed to deploy Helm chart: %v", err)
				meta.SetStatusCondition(&ephemeralEnv.Status.Conditions, metav1.Condition{
					Type:               ephemeralv1alpha1.ConditionTypeHelmDeployed,
					Status:             metav1.ConditionFalse,
					Reason:             "HelmDeploymentFailed",
					Message:            fmt.Sprintf("Helm deployment failed: %v", err),
					LastTransitionTime: metav1.Now(),
				})
				if statusErr := r.Status().Update(ctx, ephemeralEnv); statusErr != nil {
					logger.Error(statusErr, "Failed to update status after Helm error")
				}
				return ctrl.Result{}, err
			}
		}
		// No helm/components: skip deploy (e.g. minimal env for kubeconfig self-service only)
	}

	// Update phase to Active if we've successfully created all resources
	ephemeralEnv.Status.Phase = ephemeralv1alpha1.PhaseActive
	ephemeralEnv.Status.Message = "Environment is active and accessible"
	ephemeralEnv.Status.LastReconcileTime = &metav1.Time{Time: time.Now()}

	// Update status
	if err := r.Status().Update(ctx, ephemeralEnv); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to update status: %w", err)
	}

	// Requeue before expiration
	requeueAfter := time.Until(ephemeralEnv.Status.ExpirationTime.Time)
	if requeueAfter < 0 {
		requeueAfter = time.Second
	}

	logger.Info("Reconciliation complete", "requeueAfter", requeueAfter)
	return ctrl.Result{RequeueAfter: requeueAfter}, nil
}

// handleDeletion cleans up all resources when EphemeralEnv is deleted
func (r *EphemeralEnvReconciler) handleDeletion(ctx context.Context, env *ephemeralv1alpha1.EphemeralEnv) (ctrl.Result, error) {
	logger := logf.FromContext(ctx)

	if controllerutil.ContainsFinalizer(env, ephemeralEnvFinalizer) {
		logger.Info("Running cleanup for deleted EphemeralEnv", "name", env.Name)

		// Delete the Admin HTTPRoute first (it's in a different namespace)
		if err := r.cleanupAdminHTTPRoute(ctx, env); err != nil {
			logger.Error(err, "Failed to cleanup Admin HTTPRoute")
			// Continue with cleanup even if this fails
		}

		// Delete the app HTTPRoute (it's in a different namespace)
		if err := r.cleanupHTTPRoute(ctx, env); err != nil {
			logger.Error(err, "Failed to cleanup HTTPRoute")
			// Continue with cleanup even if this fails
		}

		// Uninstall Helm release before deleting namespace
		if err := r.uninstallHelmRelease(ctx, env); err != nil {
			logger.Error(err, "Failed to uninstall Helm release")
			// Continue with cleanup even if this fails
		}

		// Delete the namespace (this will cascade delete NetworkPolicy, ReferenceGrant, pods, etc.)
		if err := r.cleanupNamespace(ctx, env); err != nil {
			logger.Error(err, "Failed to cleanup namespace")
			// Requeue to retry cleanup
			return ctrl.Result{RequeueAfter: 5 * time.Second}, err
		}

		// Remove finalizer to allow deletion to complete
		logger.Info("Removing finalizer from EphemeralEnv", "name", env.Name)
		controllerutil.RemoveFinalizer(env, ephemeralEnvFinalizer)
		if err := r.Update(ctx, env); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to remove finalizer: %w", err)
		}

		logger.Info("Cleanup completed for EphemeralEnv", "name", env.Name)
	}

	return ctrl.Result{}, nil
}

// ensureNamespace creates or updates the namespace for the EphemeralEnv
func (r *EphemeralEnvReconciler) ensureNamespace(ctx context.Context, env *ephemeralv1alpha1.EphemeralEnv) error {
	logger := logf.FromContext(ctx)
	namespaceName := env.GetNamespaceName()

	// Define the desired namespace
	namespace := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: namespaceName,
			Labels: map[string]string{
				LabelManagedBy: ManagedByValue,
				LabelOwner:     env.Name,
			},
		},
	}

	// Use CreateOrUpdate for idempotency
	result, err := controllerutil.CreateOrUpdate(ctx, r.Client, namespace, func() error {
		// Ensure labels are set (mutation function)
		if namespace.Labels == nil {
			namespace.Labels = make(map[string]string)
		}
		namespace.Labels[LabelManagedBy] = ManagedByValue
		namespace.Labels[LabelOwner] = env.Name
		return nil
	})

	if err != nil {
		return fmt.Errorf("failed to create/update namespace %s: %w", namespaceName, err)
	}

	logger.Info("Namespace reconciled", "namespace", namespaceName, "result", result)

	// Update status with namespace name
	env.Status.ActiveNamespace = namespaceName

	// Set NamespaceReady condition
	meta.SetStatusCondition(&env.Status.Conditions, metav1.Condition{
		Type:               ephemeralv1alpha1.ConditionTypeNamespaceReady,
		Status:             metav1.ConditionTrue,
		Reason:             "NamespaceCreated",
		Message:            fmt.Sprintf("Namespace %s is ready", namespaceName),
		LastTransitionTime: metav1.Now(),
	})

	return nil
}

// ensureDeveloperAccessRBAC creates ServiceAccount "developer-access", Role "ns-admin" (full access in namespace),
// and RoleBinding binding the SA to the Role so developers can download a restricted kubeconfig.
func (r *EphemeralEnvReconciler) ensureDeveloperAccessRBAC(ctx context.Context, env *ephemeralv1alpha1.EphemeralEnv) error {
	logger := logf.FromContext(ctx)
	namespaceName := env.Status.ActiveNamespace
	if namespaceName == "" {
		namespaceName = env.GetNamespaceName()
	}

	// 1. ServiceAccount developer-access
	sa := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      DeveloperAccessSA,
			Namespace: namespaceName,
			Labels: map[string]string{
				LabelManagedBy: ManagedByValue,
				LabelOwner:     env.Name,
			},
		},
	}
	result, err := controllerutil.CreateOrUpdate(ctx, r.Client, sa, func() error {
		if sa.Labels == nil {
			sa.Labels = make(map[string]string)
		}
		sa.Labels[LabelManagedBy] = ManagedByValue
		sa.Labels[LabelOwner] = env.Name
		return nil
	})
	if err != nil {
		return fmt.Errorf("failed to create/update ServiceAccount %s: %w", DeveloperAccessSA, err)
	}
	logger.Info("ServiceAccount reconciled", "namespace", namespaceName, "name", DeveloperAccessSA, "result", result)

	// 2. Role ns-admin: full access within namespace
	role := &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{
			Name:      NSAdminRoleName,
			Namespace: namespaceName,
			Labels: map[string]string{
				LabelManagedBy: ManagedByValue,
				LabelOwner:     env.Name,
			},
		},
		Rules: []rbacv1.PolicyRule{
			{
				APIGroups: []string{"*"},
				Resources: []string{"*"},
				Verbs:     []string{"*"},
			},
		},
	}
	result, err = controllerutil.CreateOrUpdate(ctx, r.Client, role, func() error {
		if role.Labels == nil {
			role.Labels = make(map[string]string)
		}
		role.Labels[LabelManagedBy] = ManagedByValue
		role.Labels[LabelOwner] = env.Name
		role.Rules = []rbacv1.PolicyRule{
			{APIGroups: []string{"*"}, Resources: []string{"*"}, Verbs: []string{"*"}},
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("failed to create/update Role %s: %w", NSAdminRoleName, err)
	}
	logger.Info("Role reconciled", "namespace", namespaceName, "name", NSAdminRoleName, "result", result)

	// 3. RoleBinding: developer-access SA -> ns-admin Role
	roleBinding := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      DeveloperAccessBindingName,
			Namespace: namespaceName,
			Labels: map[string]string{
				LabelManagedBy: ManagedByValue,
				LabelOwner:     env.Name,
			},
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      rbacv1.ServiceAccountKind,
				Name:      DeveloperAccessSA,
				Namespace: namespaceName,
			},
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: rbacv1.GroupName,
			Kind:     "Role",
			Name:     NSAdminRoleName,
		},
	}
	result, err = controllerutil.CreateOrUpdate(ctx, r.Client, roleBinding, func() error {
		if roleBinding.Labels == nil {
			roleBinding.Labels = make(map[string]string)
		}
		roleBinding.Labels[LabelManagedBy] = ManagedByValue
		roleBinding.Labels[LabelOwner] = env.Name
		roleBinding.Subjects = []rbacv1.Subject{
			{Kind: rbacv1.ServiceAccountKind, Name: DeveloperAccessSA, Namespace: namespaceName},
		}
		roleBinding.RoleRef = rbacv1.RoleRef{APIGroup: rbacv1.GroupName, Kind: "Role", Name: NSAdminRoleName}
		return nil
	})
	if err != nil {
		return fmt.Errorf("failed to create/update RoleBinding %s: %w", DeveloperAccessBindingName, err)
	}
	logger.Info("RoleBinding reconciled", "namespace", namespaceName, "name", DeveloperAccessBindingName, "result", result)

	return nil
}

// ensureNetworkPolicy creates a deny-all NetworkPolicy if isolation is enabled
func (r *EphemeralEnvReconciler) ensureNetworkPolicy(ctx context.Context, env *ephemeralv1alpha1.EphemeralEnv) error {
	// Skip if isolation is disabled
	if !env.Spec.IsIsolationEnabled() {
		return nil
	}

	logger := logf.FromContext(ctx)
	namespaceName := env.Status.ActiveNamespace

	// Define a deny-all NetworkPolicy
	networkPolicy := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "deny-all",
			Namespace: namespaceName,
			Labels: map[string]string{
				LabelManagedBy: ManagedByValue,
				LabelOwner:     env.Name,
			},
		},
		Spec: networkingv1.NetworkPolicySpec{
			// Empty PodSelector selects all pods in the namespace
			PodSelector: metav1.LabelSelector{},
			// Specify both Ingress and Egress policy types
			PolicyTypes: []networkingv1.PolicyType{
				networkingv1.PolicyTypeIngress,
				networkingv1.PolicyTypeEgress,
			},
			// Empty Ingress and Egress rules = deny all traffic
			Ingress: []networkingv1.NetworkPolicyIngressRule{},
			Egress:  []networkingv1.NetworkPolicyEgressRule{},
		},
	}

	// Use CreateOrUpdate for idempotency
	result, err := controllerutil.CreateOrUpdate(ctx, r.Client, networkPolicy, func() error {
		// Ensure labels are set
		if networkPolicy.Labels == nil {
			networkPolicy.Labels = make(map[string]string)
		}
		networkPolicy.Labels[LabelManagedBy] = ManagedByValue
		networkPolicy.Labels[LabelOwner] = env.Name
		return nil
	})

	if err != nil {
		return fmt.Errorf("failed to create/update NetworkPolicy: %w", err)
	}

	logger.Info("NetworkPolicy reconciled", "namespace", namespaceName, "result", result)

	// Set NetworkPolicyApplied condition
	meta.SetStatusCondition(&env.Status.Conditions, metav1.Condition{
		Type:               ephemeralv1alpha1.ConditionTypeNetworkPolicyApplied,
		Status:             metav1.ConditionTrue,
		Reason:             "NetworkPolicyCreated",
		Message:            "Deny-all NetworkPolicy has been applied",
		LastTransitionTime: metav1.Now(),
	})

	return nil
}

// handleExpiration handles the cleanup when an EphemeralEnv has expired
func (r *EphemeralEnvReconciler) handleExpiration(ctx context.Context, env *ephemeralv1alpha1.EphemeralEnv) (ctrl.Result, error) {
	logger := logf.FromContext(ctx)

	// Set phase to Expired
	env.Status.Phase = ephemeralv1alpha1.PhaseExpired
	env.Status.Message = "Environment has expired and is being cleaned up"

	// Set EnvironmentExpired condition
	meta.SetStatusCondition(&env.Status.Conditions, metav1.Condition{
		Type:               ephemeralv1alpha1.ConditionTypeEnvironmentExpired,
		Status:             metav1.ConditionTrue,
		Reason:             "TTLExpired",
		Message:            "The environment TTL has expired",
		LastTransitionTime: metav1.Now(),
	})

	// Delete the HTTPRoute first (it's in a different namespace)
	if err := r.cleanupHTTPRoute(ctx, env); err != nil {
		logger.Error(err, "Failed to cleanup HTTPRoute")
		// Continue with namespace cleanup even if HTTPRoute cleanup fails
	}

	// Uninstall Helm release before deleting namespace
	if err := r.uninstallHelmRelease(ctx, env); err != nil {
		logger.Error(err, "Failed to uninstall Helm release")
		// Continue with namespace cleanup even if Helm uninstall fails
	}

	// Delete the namespace (this will cascade delete NetworkPolicy, ReferenceGrant, and other resources)
	if err := r.cleanupNamespace(ctx, env); err != nil {
		logger.Error(err, "Failed to cleanup namespace")
		env.Status.Message = fmt.Sprintf("Environment expired but cleanup failed: %v", err)
		if statusErr := r.Status().Update(ctx, env); statusErr != nil {
			logger.Error(statusErr, "Failed to update status after cleanup error")
		}
		// Requeue to retry cleanup
		return ctrl.Result{RequeueAfter: 30 * time.Second}, err
	}

	// Update status
	env.Status.LastReconcileTime = &metav1.Time{Time: time.Now()}
	if err := r.Status().Update(ctx, env); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to update status: %w", err)
	}

	logger.Info("EphemeralEnv cleanup completed", "name", env.Name)
	return ctrl.Result{}, nil // No requeue - cleanup is done
}

// cleanupNamespace deletes the namespace associated with the EphemeralEnv
func (r *EphemeralEnvReconciler) cleanupNamespace(ctx context.Context, env *ephemeralv1alpha1.EphemeralEnv) error {
	logger := logf.FromContext(ctx)
	namespaceName := env.GetNamespaceName()

	// Check if namespace exists
	namespace := &corev1.Namespace{}
	err := r.Get(ctx, client.ObjectKey{Name: namespaceName}, namespace)
	if err != nil {
		if apierrors.IsNotFound(err) {
			// Namespace doesn't exist, nothing to delete
			logger.Info("Namespace already deleted or doesn't exist", "namespace", namespaceName)
			return nil
		}
		return fmt.Errorf("failed to get namespace %s: %w", namespaceName, err)
	}

	// Check if already being deleted
	if namespace.DeletionTimestamp != nil {
		logger.Info("Namespace already being deleted", "namespace", namespaceName)
		return nil
	}

	// Delete the namespace
	logger.Info("Deleting namespace", "namespace", namespaceName)
	if err := r.Delete(ctx, namespace); err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("failed to delete namespace %s: %w", namespaceName, err)
	}

	logger.Info("Namespace deletion initiated", "namespace", namespaceName)
	return nil
}

// cleanupHTTPRoute deletes the HTTPRoute associated with the EphemeralEnv
func (r *EphemeralEnvReconciler) cleanupHTTPRoute(ctx context.Context, env *ephemeralv1alpha1.EphemeralEnv) error {
	logger := logf.FromContext(ctx)
	routeName := env.GetHTTPRouteName()
	routeNamespace := env.Spec.Gateway.Namespace

	// Check if HTTPRoute exists
	httpRoute := &gatewayv1.HTTPRoute{}
	err := r.Get(ctx, client.ObjectKey{Name: routeName, Namespace: routeNamespace}, httpRoute)
	if err != nil {
		if apierrors.IsNotFound(err) {
			logger.Info("HTTPRoute already deleted or doesn't exist", "name", routeName, "namespace", routeNamespace)
			return nil
		}
		return fmt.Errorf("failed to get HTTPRoute %s/%s: %w", routeNamespace, routeName, err)
	}

	// Delete the HTTPRoute
	logger.Info("Deleting HTTPRoute", "name", routeName, "namespace", routeNamespace)
	if err := r.Delete(ctx, httpRoute); err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("failed to delete HTTPRoute %s/%s: %w", routeNamespace, routeName, err)
	}

	logger.Info("HTTPRoute deletion initiated", "name", routeName, "namespace", routeNamespace)
	return nil
}

// cleanupAdminHTTPRoute deletes the Admin HTTPRoute associated with the EphemeralEnv
func (r *EphemeralEnvReconciler) cleanupAdminHTTPRoute(ctx context.Context, env *ephemeralv1alpha1.EphemeralEnv) error {
	logger := logf.FromContext(ctx)
	routeName := fmt.Sprintf("admin-route-%s", env.Name)
	routeNamespace := env.Spec.Gateway.Namespace

	// Check if Admin HTTPRoute exists
	httpRoute := &gatewayv1.HTTPRoute{}
	err := r.Get(ctx, client.ObjectKey{Name: routeName, Namespace: routeNamespace}, httpRoute)
	if err != nil {
		if apierrors.IsNotFound(err) {
			logger.Info("Admin HTTPRoute already deleted or doesn't exist", "name", routeName, "namespace", routeNamespace)
			return nil
		}
		return fmt.Errorf("failed to get Admin HTTPRoute %s/%s: %w", routeNamespace, routeName, err)
	}

	// Delete the Admin HTTPRoute
	logger.Info("Deleting Admin HTTPRoute", "name", routeName, "namespace", routeNamespace)
	if err := r.Delete(ctx, httpRoute); err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("failed to delete Admin HTTPRoute %s/%s: %w", routeNamespace, routeName, err)
	}

	logger.Info("Admin HTTPRoute deletion initiated", "name", routeName, "namespace", routeNamespace)
	return nil
}

// ensureHTTPRoute creates or updates the HTTPRoute for the EphemeralEnv
// serviceName is the backend service name (either from spec or discovered via port)
func (r *EphemeralEnvReconciler) ensureHTTPRoute(ctx context.Context, env *ephemeralv1alpha1.EphemeralEnv, serviceName string) error {
	logger := logf.FromContext(ctx)
	routeName := env.GetHTTPRouteName()
	routeNamespace := env.Spec.Gateway.Namespace

	// Build the hostname
	hostname := gatewayv1.Hostname(fmt.Sprintf("%s.%s", env.Spec.Gateway.DomainPrefix, BaseDomain))

	// Parent Gateway reference
	gatewayNamespace := gatewayv1.Namespace(env.Spec.Gateway.Namespace)
	parentRef := gatewayv1.ParentReference{
		Name:      gatewayv1.ObjectName(env.Spec.Gateway.Name),
		Namespace: &gatewayNamespace,
	}

	// Determine target port: use ServicePort if set, otherwise Gateway.TargetPort
	var targetPort int32
	if env.Spec.ServicePort != nil {
		targetPort = *env.Spec.ServicePort
	} else {
		targetPort = env.Spec.Gateway.TargetPort
	}

	// Backend service reference
	serviceNamespace := gatewayv1.Namespace(env.Status.ActiveNamespace)
	port := gatewayv1.PortNumber(targetPort)
	backendRef := gatewayv1.HTTPBackendRef{
		BackendRef: gatewayv1.BackendRef{
			BackendObjectReference: gatewayv1.BackendObjectReference{
				Name:      gatewayv1.ObjectName(serviceName),
				Namespace: &serviceNamespace,
				Port:      &port,
			},
		},
	}

	// Create HTTPRoute
	httpRoute := &gatewayv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      routeName,
			Namespace: routeNamespace,
			Labels: map[string]string{
				LabelManagedBy: ManagedByValue,
				LabelOwner:     env.Name,
			},
		},
	}

	// Use CreateOrUpdate for idempotency
	result, err := controllerutil.CreateOrUpdate(ctx, r.Client, httpRoute, func() error {
		// Ensure labels are set
		if httpRoute.Labels == nil {
			httpRoute.Labels = make(map[string]string)
		}
		httpRoute.Labels[LabelManagedBy] = ManagedByValue
		httpRoute.Labels[LabelOwner] = env.Name

		// Set the spec
		httpRoute.Spec = gatewayv1.HTTPRouteSpec{
			CommonRouteSpec: gatewayv1.CommonRouteSpec{
				ParentRefs: []gatewayv1.ParentReference{parentRef},
			},
			Hostnames: []gatewayv1.Hostname{hostname},
			Rules: []gatewayv1.HTTPRouteRule{
				{
					BackendRefs: []gatewayv1.HTTPBackendRef{backendRef},
				},
			},
		}
		return nil
	})

	if err != nil {
		return fmt.Errorf("failed to create/update HTTPRoute %s/%s: %w", routeNamespace, routeName, err)
	}

	logger.Info("HTTPRoute reconciled", "name", routeName, "namespace", routeNamespace, "result", result)

	// Update status with AccessURL
	env.Status.AccessURL = fmt.Sprintf("https://%s", hostname)

	// Set HTTPRouteReady condition
	meta.SetStatusCondition(&env.Status.Conditions, metav1.Condition{
		Type:               ephemeralv1alpha1.ConditionTypeHTTPRouteReady,
		Status:             metav1.ConditionTrue,
		Reason:             "HTTPRouteCreated",
		Message:            fmt.Sprintf("HTTPRoute %s is ready", routeName),
		LastTransitionTime: metav1.Now(),
	})

	return nil
}

// ensureAdminHTTPRoute creates an HTTPRoute for the admin dashboard (admin.pr-123.domain.com -> Operator UI)
func (r *EphemeralEnvReconciler) ensureAdminHTTPRoute(ctx context.Context, env *ephemeralv1alpha1.EphemeralEnv, operatorService string, operatorNamespace string) error {
	logger := logf.FromContext(ctx)
	routeName := fmt.Sprintf("admin-route-%s", env.Name)
	routeNamespace := env.Spec.Gateway.Namespace

	// Build the admin hostname: admin.pr-123.preview.example.com
	hostname := gatewayv1.Hostname(fmt.Sprintf("admin.%s.%s", env.Spec.Gateway.DomainPrefix, BaseDomain))

	// Parent Gateway reference
	gatewayNamespace := gatewayv1.Namespace(env.Spec.Gateway.Namespace)
	parentRef := gatewayv1.ParentReference{
		Name:      gatewayv1.ObjectName(env.Spec.Gateway.Name),
		Namespace: &gatewayNamespace,
	}

	// Backend service reference - points to the Operator's UI service
	opNs := gatewayv1.Namespace(operatorNamespace)
	port := gatewayv1.PortNumber(8080) // UI server port
	backendRef := gatewayv1.HTTPBackendRef{
		BackendRef: gatewayv1.BackendRef{
			BackendObjectReference: gatewayv1.BackendObjectReference{
				Name:      gatewayv1.ObjectName(operatorService),
				Namespace: &opNs,
				Port:      &port,
			},
		},
	}

	// Create HTTPRoute for admin
	httpRoute := &gatewayv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      routeName,
			Namespace: routeNamespace,
			Labels: map[string]string{
				LabelManagedBy: ManagedByValue,
				LabelOwner:     env.Name,
				LabelRouteType: RouteTypeAdmin,
			},
		},
	}

	// Use CreateOrUpdate for idempotency
	result, err := controllerutil.CreateOrUpdate(ctx, r.Client, httpRoute, func() error {
		// Ensure labels are set
		if httpRoute.Labels == nil {
			httpRoute.Labels = make(map[string]string)
		}
		httpRoute.Labels[LabelManagedBy] = ManagedByValue
		httpRoute.Labels[LabelOwner] = env.Name
		httpRoute.Labels[LabelRouteType] = RouteTypeAdmin

		// Set the spec
		httpRoute.Spec = gatewayv1.HTTPRouteSpec{
			CommonRouteSpec: gatewayv1.CommonRouteSpec{
				ParentRefs: []gatewayv1.ParentReference{parentRef},
			},
			Hostnames: []gatewayv1.Hostname{hostname},
			Rules: []gatewayv1.HTTPRouteRule{
				{
					BackendRefs: []gatewayv1.HTTPBackendRef{backendRef},
				},
			},
		}
		return nil
	})

	if err != nil {
		return fmt.Errorf("failed to create/update admin HTTPRoute %s/%s: %w", routeNamespace, routeName, err)
	}

	logger.Info("Admin HTTPRoute reconciled", "name", routeName, "namespace", routeNamespace, "result", result)

	// Update status with admin URL
	env.Status.AdminURL = fmt.Sprintf("https://admin.%s.%s", env.Spec.Gateway.DomainPrefix, BaseDomain)

	return nil
}

// ensureReferenceGrant creates a ReferenceGrant for cross-namespace routing
func (r *EphemeralEnvReconciler) ensureReferenceGrant(ctx context.Context, env *ephemeralv1alpha1.EphemeralEnv) error {
	// Only needed if HTTPRoute and Service are in different namespaces
	if env.Spec.Gateway.Namespace == env.Status.ActiveNamespace {
		return nil // Same namespace, no grant needed
	}

	logger := logf.FromContext(ctx)
	grantName := fmt.Sprintf("grant-%s", env.Name)
	grantNamespace := env.Status.ActiveNamespace

	// Create ReferenceGrant
	refGrant := &gatewayv1beta1.ReferenceGrant{
		ObjectMeta: metav1.ObjectMeta{
			Name:      grantName,
			Namespace: grantNamespace,
			Labels: map[string]string{
				LabelManagedBy: ManagedByValue,
				LabelOwner:     env.Name,
			},
		},
	}

	// Use CreateOrUpdate for idempotency
	result, err := controllerutil.CreateOrUpdate(ctx, r.Client, refGrant, func() error {
		// Ensure labels are set
		if refGrant.Labels == nil {
			refGrant.Labels = make(map[string]string)
		}
		refGrant.Labels[LabelManagedBy] = ManagedByValue
		refGrant.Labels[LabelOwner] = env.Name

		// Set the spec
		refGrant.Spec = gatewayv1beta1.ReferenceGrantSpec{
			From: []gatewayv1beta1.ReferenceGrantFrom{
				{
					Group:     gatewayv1beta1.Group("gateway.networking.k8s.io"),
					Kind:      gatewayv1beta1.Kind("HTTPRoute"),
					Namespace: gatewayv1beta1.Namespace(env.Spec.Gateway.Namespace),
				},
			},
			To: []gatewayv1beta1.ReferenceGrantTo{
				{
					Group: gatewayv1beta1.Group(""),
					Kind:  gatewayv1beta1.Kind("Service"),
				},
			},
		}
		return nil
	})

	if err != nil {
		return fmt.Errorf("failed to create/update ReferenceGrant %s/%s: %w", grantNamespace, grantName, err)
	}

	logger.Info("ReferenceGrant reconciled", "name", grantName, "namespace", grantNamespace, "result", result)
	return nil
}

// ensureHelmRelease deploys the Helm chart(s) specified in the EphemeralEnv spec
// It supports both legacy single-chart (Helm field) and new multi-chart (Components field) deployments
func (r *EphemeralEnvReconciler) ensureHelmRelease(ctx context.Context, env *ephemeralv1alpha1.EphemeralEnv) error {
	logger := logf.FromContext(ctx)
	namespace := env.Status.ActiveNamespace

	// Determine which deployment mode to use
	components := r.resolveComponents(ctx, env)

	if len(components) == 0 {
		return fmt.Errorf("no components to deploy: either helm or components must be specified")
	}

	// Deploy each component
	componentStatuses := make([]ephemeralv1alpha1.ComponentStatus, 0, len(components))
	for _, comp := range components {
		status, err := r.deployComponent(ctx, env, namespace, comp)
		if err != nil {
			logger.Error(err, "Failed to deploy component", "component", comp.Name)
			return fmt.Errorf("failed to deploy component %s: %w", comp.Name, err)
		}
		componentStatuses = append(componentStatuses, status)
	}

	// Update status with component statuses
	env.Status.ComponentStatuses = componentStatuses

	// Set HelmDeployed condition
	meta.SetStatusCondition(&env.Status.Conditions, metav1.Condition{
		Type:               ephemeralv1alpha1.ConditionTypeHelmDeployed,
		Status:             metav1.ConditionTrue,
		Reason:             "HelmInstalled",
		Message:            fmt.Sprintf("All %d components deployed successfully", len(components)),
		LastTransitionTime: metav1.Now(),
	})

	return nil
}

// resolveComponents returns the list of components to deploy
// Priority: Components > TemplateRef > Helm (legacy)
func (r *EphemeralEnvReconciler) resolveComponents(ctx context.Context, env *ephemeralv1alpha1.EphemeralEnv) []ephemeralv1alpha1.DeployedComponentSpec {
	logger := logf.FromContext(ctx)

	// Priority 1: Use Components if specified
	if len(env.Spec.Components) > 0 {
		logger.Info("Using components from EphemeralEnv spec", "count", len(env.Spec.Components))
		return env.Spec.Components
	}

	// Priority 2: Use TemplateRef if specified
	if env.Spec.TemplateRef != nil {
		template, err := r.getTemplate(ctx, env)
		if err != nil {
			logger.Error(err, "Failed to resolve template reference", "template", env.Spec.TemplateRef.Name)
			// Fall through to legacy Helm field
		} else {
			// Convert template components to deployed components
			components := make([]ephemeralv1alpha1.DeployedComponentSpec, 0, len(template.Spec.Components))
			for _, tc := range template.Spec.Components {
				components = append(components, ephemeralv1alpha1.DeployedComponentSpec(tc))
			}
			logger.Info("Using components from EnvironmentTemplate", "template", template.Name, "count", len(components))
			return components
		}
	}

	// Priority 3: Legacy Helm field
	if env.Spec.Helm != nil {
		logger.Info("Using legacy Helm field (single chart)")
		return []ephemeralv1alpha1.DeployedComponentSpec{
			{
				Name:        env.Spec.Helm.Chart,
				Repository:  env.Spec.Helm.Repository,
				Chart:       env.Spec.Helm.Chart,
				Version:     env.Spec.Helm.Version,
				ServiceName: env.Spec.Gateway.ServiceName,
				ServicePort: env.Spec.Gateway.TargetPort,
				Values:      env.Spec.Helm.Values,
				Primary:     true,
			},
		}
	}

	return nil
}

// getTemplate retrieves the referenced EnvironmentTemplate
func (r *EphemeralEnvReconciler) getTemplate(ctx context.Context, env *ephemeralv1alpha1.EphemeralEnv) (*ephemeralv1alpha1.EnvironmentTemplate, error) {
	if env.Spec.TemplateRef == nil {
		return nil, fmt.Errorf("no template reference specified")
	}

	namespace := env.Spec.TemplateRef.Namespace
	if namespace == "" {
		namespace = env.Namespace
	}

	template := &ephemeralv1alpha1.EnvironmentTemplate{}
	if err := r.Get(ctx, client.ObjectKey{
		Name:      env.Spec.TemplateRef.Name,
		Namespace: namespace,
	}, template); err != nil {
		return nil, fmt.Errorf("failed to get template %s/%s: %w", namespace, env.Spec.TemplateRef.Name, err)
	}

	return template, nil
}

// deployComponent deploys a single component and returns its status
func (r *EphemeralEnvReconciler) deployComponent(ctx context.Context, env *ephemeralv1alpha1.EphemeralEnv, namespace string, comp ephemeralv1alpha1.DeployedComponentSpec) (ephemeralv1alpha1.ComponentStatus, error) {
	logger := logf.FromContext(ctx)

	// Generate unique release name for this component
	releaseName := fmt.Sprintf("env-%s-%s", env.Name, sanitizeName(comp.Name))

	status := ephemeralv1alpha1.ComponentStatus{
		Name:        comp.Name,
		ReleaseName: releaseName,
	}

	// Check if already installed
	isInstalled, err := r.HelmClient.IsInstalled(ctx, releaseName, namespace)
	if err != nil {
		status.Status = statusError
		status.Message = fmt.Sprintf("Failed to check status: %v", err)
		return status, fmt.Errorf("failed to check Helm release status: %w", err)
	}

	if isInstalled {
		logger.Info("Helm release already installed, skipping", "release", releaseName, "namespace", namespace, "component", comp.Name)
		status.Status = statusDeployed
		status.Message = "Already installed"
		return status, nil
	}

	// Convert JSON values to map[string]any
	var values map[string]any
	if comp.Values != nil && len(comp.Values.Raw) > 0 {
		if err := json.Unmarshal(comp.Values.Raw, &values); err != nil {
			status.Status = statusError
			status.Message = fmt.Sprintf("Failed to parse values: %v", err)
			return status, fmt.Errorf("failed to parse Helm values: %w", err)
		}
	}

	// Prepare install options
	opts := helm.InstallOptions{
		ReleaseName: releaseName,
		Namespace:   namespace,
		RepoURL:     comp.Repository,
		ChartName:   comp.Chart,
		Version:     comp.Version,
		Values:      values,
	}

	logger.Info("Installing Helm chart",
		"release", releaseName,
		"namespace", namespace,
		"chart", comp.Chart,
		"version", comp.Version,
		"component", comp.Name)

	// Install the chart
	releaseInfo, err := r.HelmClient.Install(ctx, opts)
	if err != nil {
		status.Status = statusError
		status.Message = fmt.Sprintf("Failed to install: %v", err)
		return status, fmt.Errorf("failed to install Helm chart: %w", err)
	}

	logger.Info("Helm chart installed successfully",
		"release", releaseInfo.Name,
		"version", releaseInfo.Version,
		"status", releaseInfo.Status,
		"component", comp.Name)

	now := metav1.Now()
	status.Status = statusDeployed
	status.Message = fmt.Sprintf("Helm release %s installed successfully", releaseInfo.Name)
	status.LastDeployed = &now

	// For backward compatibility, update the legacy HelmRelease field for primary component
	if comp.Primary {
		env.Status.HelmRelease = releaseName
	}

	return status, nil
}

// sanitizeName converts a name to a valid Kubernetes resource name fragment
func sanitizeName(name string) string {
	// Replace spaces and special characters with dashes, lowercase
	result := make([]byte, 0, len(name))
	for i := 0; i < len(name); i++ {
		c := name[i]
		if (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '-' {
			result = append(result, c)
		} else if c >= 'A' && c <= 'Z' {
			result = append(result, c+32) // lowercase
		} else if c == ' ' || c == '_' {
			result = append(result, '-')
		}
	}
	// Ensure it starts and ends with alphanumeric
	if len(result) > 0 && result[0] == '-' {
		result = result[1:]
	}
	if len(result) > 0 && result[len(result)-1] == '-' {
		result = result[:len(result)-1]
	}
	// Limit length
	if len(result) > 20 {
		result = result[:20]
	}
	if len(result) == 0 {
		return "component"
	}
	return string(result)
}

// uninstallHelmRelease removes all Helm releases for the EphemeralEnv
func (r *EphemeralEnvReconciler) uninstallHelmRelease(ctx context.Context, env *ephemeralv1alpha1.EphemeralEnv) error {
	if r.HelmClient == nil {
		return nil
	}

	logger := logf.FromContext(ctx)
	namespace := env.Status.ActiveNamespace

	// Uninstall all tracked component releases
	if len(env.Status.ComponentStatuses) > 0 {
		for _, compStatus := range env.Status.ComponentStatuses {
			if compStatus.ReleaseName == "" {
				continue
			}

			isInstalled, err := r.HelmClient.IsInstalled(ctx, compStatus.ReleaseName, namespace)
			if err != nil {
				logger.Error(err, "Failed to check component status", "release", compStatus.ReleaseName)
				continue
			}

			if !isInstalled {
				logger.Info("Component release not found, skipping", "release", compStatus.ReleaseName)
				continue
			}

			logger.Info("Uninstalling component release", "release", compStatus.ReleaseName, "namespace", namespace)
			if err := r.HelmClient.Uninstall(ctx, compStatus.ReleaseName, namespace); err != nil {
				logger.Error(err, "Failed to uninstall component release", "release", compStatus.ReleaseName)
				// Continue trying to uninstall other components
			} else {
				logger.Info("Component release uninstalled successfully", "release", compStatus.ReleaseName)
			}
		}
		return nil
	}

	// Fallback: try legacy single release name (for backward compatibility)
	releaseName := fmt.Sprintf("env-%s", env.Name)

	// Check if installed
	isInstalled, err := r.HelmClient.IsInstalled(ctx, releaseName, namespace)
	if err != nil {
		return fmt.Errorf("failed to check Helm release status: %w", err)
	}

	if !isInstalled {
		logger.Info("Helm release not found, nothing to uninstall", "release", releaseName)
		return nil
	}

	logger.Info("Uninstalling Helm release", "release", releaseName, "namespace", namespace)

	if err := r.HelmClient.Uninstall(ctx, releaseName, namespace); err != nil {
		return fmt.Errorf("failed to uninstall Helm release %s: %w", releaseName, err)
	}

	logger.Info("Helm release uninstalled successfully", "release", releaseName)
	return nil
}

// getPrimaryComponentName returns the name of the primary component for heuristic matching
func (r *EphemeralEnvReconciler) getPrimaryComponentName(ctx context.Context, env *ephemeralv1alpha1.EphemeralEnv) string {
	// Check Components first
	for _, comp := range env.Spec.Components {
		if comp.Primary {
			return comp.Name
		}
	}
	// Check template
	if env.Spec.TemplateRef != nil {
		template, err := r.getTemplate(ctx, env)
		if err == nil {
			for _, comp := range template.Spec.Components {
				if comp.Primary {
					return comp.Name
				}
			}
		}
	}
	// Fallback to Helm chart name
	if env.Spec.Helm != nil {
		return env.Spec.Helm.Chart
	}
	return ""
}

// discoverService finds a Service in the namespace that exposes the target port.
// It ignores Headless services (ClusterIP: None), ExternalName services, and services with "metrics" in name.
// Returns the service name if found, or error if no match/multiple ambiguous matches.
func (r *EphemeralEnvReconciler) discoverService(ctx context.Context, env *ephemeralv1alpha1.EphemeralEnv, targetPort int32) (string, error) {
	logger := logf.FromContext(ctx)
	namespace := env.Status.ActiveNamespace

	// List all services in the namespace
	serviceList := &corev1.ServiceList{}
	if err := r.List(ctx, serviceList, client.InNamespace(namespace)); err != nil {
		return "", fmt.Errorf("failed to list services in namespace %s: %w", namespace, err)
	}

	// Filter and find matching services
	var matchingServices []corev1.Service
	for _, svc := range serviceList.Items {
		// Skip Headless services (ClusterIP: None)
		if svc.Spec.ClusterIP == corev1.ClusterIPNone {
			logger.V(1).Info("Skipping headless service", "service", svc.Name)
			continue
		}
		// Skip ExternalName services
		if svc.Spec.Type == corev1.ServiceTypeExternalName {
			logger.V(1).Info("Skipping ExternalName service", "service", svc.Name)
			continue
		}
		// Skip services with "metrics" in name (common technical services)
		if strings.Contains(strings.ToLower(svc.Name), "metrics") {
			logger.V(1).Info("Skipping metrics service", "service", svc.Name)
			continue
		}

		// Check if any port matches the target port
		for _, port := range svc.Spec.Ports {
			if port.Port == targetPort {
				matchingServices = append(matchingServices, svc)
				break
			}
		}
	}

	// Handle results
	switch len(matchingServices) {
	case 0:
		return "", fmt.Errorf("no service found exposing port %d in namespace %s (service may still be starting)", targetPort, namespace)
	case 1:
		logger.Info("Discovered service by port", "service", matchingServices[0].Name, "port", targetPort)
		return matchingServices[0].Name, nil
	default:
		// Multiple matches - apply heuristic
		componentName := r.getPrimaryComponentName(ctx, env)
		logger.Info("Multiple services match port, applying heuristic", "port", targetPort, "count", len(matchingServices), "componentName", componentName)

		// Sort by name length (prefer shorter names) as secondary criteria
		sort.Slice(matchingServices, func(i, j int) bool {
			return len(matchingServices[i].Name) < len(matchingServices[j].Name)
		})

		// First preference: name contains the component name
		if componentName != "" {
			for _, svc := range matchingServices {
				if strings.Contains(strings.ToLower(svc.Name), strings.ToLower(componentName)) {
					logger.Info("Selected service matching component name", "service", svc.Name, "component", componentName)
					return svc.Name, nil
				}
			}
		}

		// Fallback: shortest name
		selected := matchingServices[0].Name
		logger.Info("Selected service with shortest name (heuristic)", "service", selected, "warning", "multiple services match - consider specifying serviceName explicitly")
		return selected, nil
	}
}

// SetupWithManager sets up the controller with the Manager.
func (r *EphemeralEnvReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&ephemeralv1alpha1.EphemeralEnv{}).
		Owns(&corev1.Namespace{}).
		Owns(&networkingv1.NetworkPolicy{}).
		Named("ephemeralenv").
		Complete(r)
}
