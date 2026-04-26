package controllers

import (
	"context"
	"fmt"
	"strings"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// defaultRequeuePart2 is the standard "success, but check again soon" backoff
// used by Wave-4 controllers that do not yet watch their child resources.
const defaultRequeuePart2 = 5 * time.Minute

// errKindMissing is returned when a projected CRD is not installed. Controllers
// treat this as a soft failure and update status-only.
var errKindMissing = fmt.Errorf("kind not installed")

// ensureUnstructured creates/updates a resource described by a dynamic
// template. It is the primitive used by controllers that project into
// third-party CRDs which may or may not be installed. When the target CRD
// is missing, errKindMissing is returned so the caller can fall back to
// status-only reconcile.
func ensureUnstructured(ctx context.Context, c client.Client, gvk schema.GroupVersionKind, namespace, name string, mutate func(u *unstructured.Unstructured)) error {
	existing := &unstructured.Unstructured{}
	existing.SetGroupVersionKind(gvk)
	err := c.Get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, existing)
	switch {
	case err == nil:
		mutate(existing)
		if uErr := c.Update(ctx, existing); uErr != nil {
			if isNoKindErr(uErr) {
				return errKindMissing
			}
			return uErr
		}
		return nil
	case apierrors.IsNotFound(err):
		u := &unstructured.Unstructured{}
		u.SetGroupVersionKind(gvk)
		u.SetNamespace(namespace)
		u.SetName(name)
		mutate(u)
		if cErr := c.Create(ctx, u); cErr != nil {
			if isNoKindErr(cErr) {
				return errKindMissing
			}
			return cErr
		}
		return nil
	default:
		if isNoKindErr(err) {
			return errKindMissing
		}
		return err
	}
}

// isNoKindErr detects "no kind X is registered" errors from the REST mapper.
func isNoKindErr(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "no matches for kind") ||
		strings.Contains(msg, "no kind is registered") ||
		strings.Contains(msg, "failed to get API group resources")
}

// setSpec writes spec into an unstructured object. Helper for controllers
// projecting a small typed spec onto a CRD they don't statically type.
func setSpec(u *unstructured.Unstructured, spec map[string]interface{}) {
	if u.Object == nil {
		u.Object = map[string]interface{}{}
	}
	u.Object["spec"] = spec
}

// unstructuredType is a local alias used by the projection helpers.
type unstructuredType = unstructured.Unstructured

// statusUpdate writes the status subresource; conflicts are swallowed so the
// caller can requeue naturally on the next observed generation.
func statusUpdate(ctx context.Context, c client.Client, obj client.Object) error {
	if err := c.Status().Update(ctx, obj); err != nil {
		if apierrors.IsConflict(err) {
			return nil
		}
		return err
	}
	return nil
}
