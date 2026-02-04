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
	"errors"

	"sigs.k8s.io/controller-runtime/pkg/client"

	ephemeralv1alpha1 "github.com/xrizur/kubeEphemerals/api/v1alpha1"
)

// EnvHandler performs environment list/create/delete with owner-based isolation.
type EnvHandler struct {
	Client           client.Client
	AdminUser        string
	DefaultNamespace string
}

// ListEnvs returns environments visible to the current user: own envs or all if user is admin.
func (h *EnvHandler) ListEnvs(ctx context.Context) ([]ephemeralv1alpha1.EphemeralEnv, error) {
	user := UserFromContext(ctx)
	var list ephemeralv1alpha1.EphemeralEnvList
	if err := h.Client.List(ctx, &list, client.InNamespace(h.DefaultNamespace)); err != nil {
		return nil, err
	}
	out := make([]ephemeralv1alpha1.EphemeralEnv, 0, len(list.Items))
	for i := range list.Items {
		env := &list.Items[i]
		if user == h.AdminUser || env.Spec.Owner == user {
			out = append(out, *env)
		}
	}
	return out, nil
}

// CreateEnv creates the environment and forces spec.owner to the current user.
func (h *EnvHandler) CreateEnv(ctx context.Context, env *ephemeralv1alpha1.EphemeralEnv) error {
	user := UserFromContext(ctx)
	env.Spec.Owner = user
	return h.Client.Create(ctx, env)
}

// DeleteEnv deletes the environment if the current user is owner or admin; otherwise returns ErrForbidden.
func (h *EnvHandler) DeleteEnv(ctx context.Context, name string) error {
	user := UserFromContext(ctx)
	env := &ephemeralv1alpha1.EphemeralEnv{}
	key := client.ObjectKey{Name: name, Namespace: h.DefaultNamespace}
	if err := h.Client.Get(ctx, key, env); err != nil {
		return err
	}
	if user != h.AdminUser && env.Spec.Owner != user {
		return ErrForbidden
	}
	return h.Client.Delete(ctx, env)
}

var ErrForbidden = errors.New("forbidden")

// CanAccessEnv returns true if currentUser can access an env owned by owner (currentUser is owner or admin).
func CanAccessEnv(owner, currentUser, adminUser string) bool {
	return currentUser == adminUser || owner == currentUser
}
