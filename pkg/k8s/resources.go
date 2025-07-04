package k8s

import (
	"context"
	"fmt"
	"strings"
	"time"

	helmv2 "github.com/fluxcd/helm-controller/api/v2beta1"
	kustomizev1 "github.com/fluxcd/kustomize-controller/api/v1"
	sourcev1 "github.com/fluxcd/source-controller/api/v1"
	sourcev1beta2 "github.com/fluxcd/source-controller/api/v1beta2"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ResourceType represents the type of FluxCD resource
type ResourceType string

const (
	ResourceTypeGitRepository  ResourceType = "GitRepository"
	ResourceTypeHelmRepository ResourceType = "HelmRepository"
	ResourceTypeKustomization  ResourceType = "Kustomization"
	ResourceTypeHelmRelease    ResourceType = "HelmRelease"
)

// Resource represents a generic FluxCD resource
type Resource struct {
	Type        ResourceType  `json:"type"`
	Name        string        `json:"name"`
	Namespace   string        `json:"namespace"`
	Ready       bool          `json:"ready"`
	Status      string        `json:"status"`
	Message     string        `json:"message"`
	Age         time.Duration `json:"age"`
	LastUpdate  time.Time     `json:"last_update"`
	Conditions  []Condition   `json:"conditions"`
	Suspended   bool          `json:"suspended"`
	Source      string        `json:"source,omitempty"`
	Path        string        `json:"path,omitempty"`
	Revision    string        `json:"revision,omitempty"`
	URL         string        `json:"url,omitempty"`
	Chart       string        `json:"chart,omitempty"`
	Version     string        `json:"version,omitempty"`
}

// Condition represents a status condition
type Condition struct {
	Type               string    `json:"type"`
	Status             string    `json:"status"`
	Reason             string    `json:"reason"`
	Message            string    `json:"message"`
	LastTransitionTime time.Time `json:"lastTransitionTime"`
}

// safeList wraps client.List with panic recovery
func (c *Client) safeList(ctx context.Context, list client.ObjectList, opts ...client.ListOption) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic in kubernetes client list operation: %v", r)
		}
	}()
	
	return c.List(ctx, list, opts...)
}

// ListGitRepositories lists all GitRepository resources
func (c *Client) ListGitRepositories(ctx context.Context, namespace string) ([]Resource, error) {
	var gitRepos sourcev1.GitRepositoryList
	opts := []client.ListOption{}
	if namespace != "" {
		// Additional safety check for namespace parameter
		if len(namespace) > 0 && namespace != "<nil>" {
			opts = append(opts, client.InNamespace(namespace))
		}
	}

	if err := c.safeList(ctx, &gitRepos, opts...); err != nil {
		// Check if this is a "no matches for kind" error by looking at the error string
		errStr := ""
		if err != nil {
			errStr = err.Error()
		}

		isCRDMissing := client.IgnoreNotFound(err) == nil ||
			(errStr != "" && (
				strings.Contains(errStr, "no matches for kind") ||
				strings.Contains(errStr, "could not find the requested resource") ||
				strings.Contains(errStr, "the server could not find the requested resource")))

		if isCRDMissing {
			// CRD not available, return empty list
			return []Resource{}, nil
		}
		return nil, fmt.Errorf("failed to list GitRepositories: %w", err)
	}

	resources := make([]Resource, 0, len(gitRepos.Items))
	for _, repo := range gitRepos.Items {
		resource := Resource{
			Type:       ResourceTypeGitRepository,
			Name:       repo.Name,
			Namespace:  repo.Namespace,
			Age:        time.Since(repo.CreationTimestamp.Time),
			LastUpdate: time.Now(),
			Suspended:  repo.Spec.Suspend,
			URL:        repo.Spec.URL,
		}

		// Parse status
		if repo.Status.Conditions != nil {
			for _, cond := range repo.Status.Conditions {
				resource.Conditions = append(resource.Conditions, Condition{
					Type:               cond.Type,
					Status:             string(cond.Status),
					Reason:             cond.Reason,
					Message:            cond.Message,
					LastTransitionTime: cond.LastTransitionTime.Time,
				})

				if cond.Type == "Ready" {
					resource.Ready = cond.Status == metav1.ConditionTrue
					resource.Status = cond.Reason
					resource.Message = cond.Message
				}
			}
		}

		if repo.Status.Artifact != nil {
			resource.Revision = repo.Status.Artifact.Revision
		}

		resources = append(resources, resource)
	}

	return resources, nil
}

// ListHelmRepositories lists all HelmRepository resources
func (c *Client) ListHelmRepositories(ctx context.Context, namespace string) ([]Resource, error) {
	// Add safety checks
	if c == nil {
		return nil, fmt.Errorf("kubernetes client is nil")
	}
	if c.Client == nil {
		return nil, fmt.Errorf("embedded kubernetes client is nil")
	}
	if ctx == nil {
		return nil, fmt.Errorf("context is nil")
	}

	opts := []client.ListOption{}
	if namespace != "" {
		// Additional safety check for namespace parameter
		if len(namespace) > 0 && namespace != "<nil>" {
			opts = append(opts, client.InNamespace(namespace))
		}
	}

	// Try v1beta2 first (latest), then fallback to v1 if available
	var helmRepos sourcev1beta2.HelmRepositoryList

	// Use safeList instead of direct List call
	var listErr error
	listErr = c.safeList(ctx, &helmRepos, opts...)

	if listErr != nil {
		// Check if this is a "no matches for kind" error by looking at the error string
		errStr := ""
		if listErr != nil {
			errStr = listErr.Error()
		}

		isCRDMissing := client.IgnoreNotFound(listErr) == nil ||
			(errStr != "" && (
				strings.Contains(errStr, "no matches for kind") ||
				strings.Contains(errStr, "could not find the requested resource") ||
				strings.Contains(errStr, "the server could not find the requested resource")))

		if isCRDMissing {
			// Resource not found or CRD not available - try v1 fallback
			var helmReposV1 sourcev1.HelmRepositoryList
			if errV1 := c.safeList(ctx, &helmReposV1, opts...); errV1 != nil {
				errV1Str := ""
				if errV1 != nil {
					errV1Str = errV1.Error()
				}

				isV1CRDMissing := client.IgnoreNotFound(errV1) == nil ||
					(errV1Str != "" && (
						strings.Contains(errV1Str, "no matches for kind") ||
						strings.Contains(errV1Str, "could not find the requested resource") ||
						strings.Contains(errV1Str, "the server could not find the requested resource")))

				if isV1CRDMissing {
					// Both v1beta2 and v1 are not available, return empty list
					return []Resource{}, nil
				}
				// Return the original v1beta2 error with additional context
				return nil, fmt.Errorf("failed to list HelmRepositories (tried v1beta2 and v1): v1beta2=%w, v1=%v", listErr, errV1)
			}

			// Convert v1 results to our format
			resources := make([]Resource, 0, len(helmReposV1.Items))
			for _, repo := range helmReposV1.Items {
				resource := Resource{
					Type:       ResourceTypeHelmRepository,
					Name:       repo.Name,
					Namespace:  repo.Namespace,
					Age:        time.Since(repo.CreationTimestamp.Time),
					LastUpdate: time.Now(),
					Suspended:  repo.Spec.Suspend,
					URL:        repo.Spec.URL,
				}

				// Parse status (v1 format)
				if repo.Status.Conditions != nil {
					for _, cond := range repo.Status.Conditions {
						resource.Conditions = append(resource.Conditions, Condition{
							Type:               cond.Type,
							Status:             string(cond.Status),
							Reason:             cond.Reason,
							Message:            cond.Message,
							LastTransitionTime: cond.LastTransitionTime.Time,
						})
					}
				}

				if len(repo.Status.Conditions) > 0 {
					lastCond := repo.Status.Conditions[len(repo.Status.Conditions)-1]
					resource.Status = string(lastCond.Status)
					resource.Message = lastCond.Message
					resource.Ready = lastCond.Status == metav1.ConditionTrue
				}

				resources = append(resources, resource)
			}
			return resources, nil
		}

		// For other errors, return them directly
		return nil, fmt.Errorf("failed to list HelmRepositories (v1beta2): %w", listErr)
	}

	// Process v1beta2 results normally
	resources := make([]Resource, 0, len(helmRepos.Items))
	for _, repo := range helmRepos.Items {
		resource := Resource{
			Type:       ResourceTypeHelmRepository,
			Name:       repo.Name,
			Namespace:  repo.Namespace,
			Age:        time.Since(repo.CreationTimestamp.Time),
			LastUpdate: time.Now(),
			Suspended:  repo.Spec.Suspend,
			URL:        repo.Spec.URL,
		}

		// Parse status (v1beta2 format)
		if repo.Status.Conditions != nil {
			for _, cond := range repo.Status.Conditions {
				resource.Conditions = append(resource.Conditions, Condition{
					Type:               cond.Type,
					Status:             string(cond.Status),
					Reason:             cond.Reason,
					Message:            cond.Message,
					LastTransitionTime: cond.LastTransitionTime.Time,
				})
			}
		}

		if len(repo.Status.Conditions) > 0 {
			lastCond := repo.Status.Conditions[len(repo.Status.Conditions)-1]
			resource.Status = string(lastCond.Status)
			resource.Message = lastCond.Message
			resource.Ready = lastCond.Status == metav1.ConditionTrue
		}

		resources = append(resources, resource)
	}

	return resources, nil
}

// ListKustomizations lists all Kustomization resources
func (c *Client) ListKustomizations(ctx context.Context, namespace string) ([]Resource, error) {
	var kustomizations kustomizev1.KustomizationList
	opts := []client.ListOption{}
	if namespace != "" {
		// Additional safety check for namespace parameter
		if len(namespace) > 0 && namespace != "<nil>" {
			opts = append(opts, client.InNamespace(namespace))
		}
	}

	if err := c.safeList(ctx, &kustomizations, opts...); err != nil {
		// Check if this is a "no matches for kind" error by looking at the error string
		errStr := ""
		if err != nil {
			errStr = err.Error()
		}

		isCRDMissing := client.IgnoreNotFound(err) == nil ||
			(errStr != "" && (
				strings.Contains(errStr, "no matches for kind") ||
				strings.Contains(errStr, "could not find the requested resource") ||
				strings.Contains(errStr, "the server could not find the requested resource")))

		if isCRDMissing {
			// CRD not available, return empty list
			return []Resource{}, nil
		}
		return nil, fmt.Errorf("failed to list Kustomizations: %w", err)
	}

	resources := make([]Resource, 0, len(kustomizations.Items))
	for _, ks := range kustomizations.Items {
		resource := Resource{
			Type:       ResourceTypeKustomization,
			Name:       ks.Name,
			Namespace:  ks.Namespace,
			Age:        time.Since(ks.CreationTimestamp.Time),
			LastUpdate: time.Now(),
			Suspended:  ks.Spec.Suspend,
			Path:       ks.Spec.Path,
		}

		if ks.Spec.SourceRef.Kind == "GitRepository" {
			resource.Source = ks.Spec.SourceRef.Name
		}

		// Parse status
		if ks.Status.Conditions != nil {
			for _, cond := range ks.Status.Conditions {
				resource.Conditions = append(resource.Conditions, Condition{
					Type:               cond.Type,
					Status:             string(cond.Status),
					Reason:             cond.Reason,
					Message:            cond.Message,
					LastTransitionTime: cond.LastTransitionTime.Time,
				})

				if cond.Type == "Ready" {
					resource.Ready = cond.Status == metav1.ConditionTrue
					resource.Status = cond.Reason
					resource.Message = cond.Message
				}
			}
		}

		if ks.Status.LastAppliedRevision != "" {
			resource.Revision = ks.Status.LastAppliedRevision
		}

		resources = append(resources, resource)
	}

	return resources, nil
}

// ListHelmReleases lists all HelmRelease resources
func (c *Client) ListHelmReleases(ctx context.Context, namespace string) ([]Resource, error) {
	var helmReleases helmv2.HelmReleaseList
	opts := []client.ListOption{}
	if namespace != "" {
		// Additional safety check for namespace parameter
		if len(namespace) > 0 && namespace != "<nil>" {
			opts = append(opts, client.InNamespace(namespace))
		}
	}

	if err := c.safeList(ctx, &helmReleases, opts...); err != nil {
		// Check if this is a "no matches for kind" error by looking at the error string
		errStr := ""
		if err != nil {
			errStr = err.Error()
		}

		isCRDMissing := client.IgnoreNotFound(err) == nil ||
			(errStr != "" && (
				strings.Contains(errStr, "no matches for kind") ||
				strings.Contains(errStr, "could not find the requested resource") ||
				strings.Contains(errStr, "the server could not find the requested resource")))

		if isCRDMissing {
			// CRD not available, return empty list
			return []Resource{}, nil
		}
		return nil, fmt.Errorf("failed to list HelmReleases: %w", err)
	}

	resources := make([]Resource, 0, len(helmReleases.Items))
	for _, hr := range helmReleases.Items {
		resource := Resource{
			Type:       ResourceTypeHelmRelease,
			Name:       hr.Name,
			Namespace:  hr.Namespace,
			Age:        time.Since(hr.CreationTimestamp.Time),
			LastUpdate: time.Now(),
			Suspended:  hr.Spec.Suspend,
			Chart:      hr.Spec.Chart.Spec.Chart,
			Version:    hr.Spec.Chart.Spec.Version,
		}

		if hr.Spec.Chart.Spec.SourceRef.Kind == "HelmRepository" {
			resource.Source = hr.Spec.Chart.Spec.SourceRef.Name
		}

		// Parse status
		if hr.Status.Conditions != nil {
			for _, cond := range hr.Status.Conditions {
				resource.Conditions = append(resource.Conditions, Condition{
					Type:               cond.Type,
					Status:             string(cond.Status),
					Reason:             cond.Reason,
					Message:            cond.Message,
					LastTransitionTime: cond.LastTransitionTime.Time,
				})

				if cond.Type == "Ready" {
					resource.Ready = cond.Status == metav1.ConditionTrue
					resource.Status = cond.Reason
					resource.Message = cond.Message
				}
			}
		}

		if hr.Status.LastAppliedRevision != "" {
			resource.Revision = hr.Status.LastAppliedRevision
		}

		resources = append(resources, resource)
	}

	return resources, nil
}

// SuspendResource suspends a FluxCD resource
func (c *Client) SuspendResource(ctx context.Context, resourceType ResourceType, name, namespace string) error {
	return c.updateSuspendStatus(ctx, resourceType, name, namespace, true)
}

// ResumeResource resumes a FluxCD resource
func (c *Client) ResumeResource(ctx context.Context, resourceType ResourceType, name, namespace string) error {
	return c.updateSuspendStatus(ctx, resourceType, name, namespace, false)
}

// updateSuspendStatus updates the suspend status of a resource
func (c *Client) updateSuspendStatus(ctx context.Context, resourceType ResourceType, name, namespace string, suspend bool) error {
	var obj client.Object

	switch resourceType {
	case ResourceTypeGitRepository:
		obj = &sourcev1.GitRepository{}
	case ResourceTypeHelmRepository:
		obj = &sourcev1beta2.HelmRepository{}
	case ResourceTypeKustomization:
		obj = &kustomizev1.Kustomization{}
	case ResourceTypeHelmRelease:
		obj = &helmv2.HelmRelease{}
	default:
		return fmt.Errorf("unsupported resource type: %s", resourceType)
	}

	key := types.NamespacedName{Name: name, Namespace: namespace}
	if err := c.Get(ctx, key, obj); err != nil {
		return fmt.Errorf("failed to get %s/%s: %w", resourceType, name, err)
	}

	// Update suspend field based on resource type
	switch resourceType {
	case ResourceTypeGitRepository:
		repo := obj.(*sourcev1.GitRepository)
		repo.Spec.Suspend = suspend
	case ResourceTypeHelmRepository:
		repo := obj.(*sourcev1beta2.HelmRepository)
		repo.Spec.Suspend = suspend
	case ResourceTypeKustomization:
		ks := obj.(*kustomizev1.Kustomization)
		ks.Spec.Suspend = suspend
	case ResourceTypeHelmRelease:
		hr := obj.(*helmv2.HelmRelease)
		hr.Spec.Suspend = suspend
	}

	if err := c.Update(ctx, obj); err != nil {
		return fmt.Errorf("failed to update %s/%s: %w", resourceType, name, err)
	}

	return nil
}

// ReconcileResource triggers reconciliation of a FluxCD resource
func (c *Client) ReconcileResource(ctx context.Context, resourceType ResourceType, name, namespace string) error {
	var obj client.Object

	switch resourceType {
	case ResourceTypeGitRepository:
		obj = &sourcev1.GitRepository{}
	case ResourceTypeHelmRepository:
		obj = &sourcev1beta2.HelmRepository{}
	case ResourceTypeKustomization:
		obj = &kustomizev1.Kustomization{}
	case ResourceTypeHelmRelease:
		obj = &helmv2.HelmRelease{}
	default:
		return fmt.Errorf("unsupported resource type: %s", resourceType)
	}

	key := types.NamespacedName{Name: name, Namespace: namespace}
	if err := c.Get(ctx, key, obj); err != nil {
		return fmt.Errorf("failed to get %s/%s: %w", resourceType, name, err)
	}

	// Add reconcile annotation
	annotations := obj.GetAnnotations()
	if annotations == nil {
		annotations = make(map[string]string)
	}
	annotations["reconcile.fluxcd.io/requestedAt"] = time.Now().UTC().Format(time.RFC3339)
	obj.SetAnnotations(annotations)

	if err := c.Update(ctx, obj); err != nil {
		return fmt.Errorf("failed to update %s/%s: %w", resourceType, name, err)
	}

	return nil
}

// GetEvents returns Kubernetes events related to FluxCD resources
func (c *Client) GetEvents(ctx context.Context, namespace string) ([]corev1.Event, error) {
	// Get all events first, then filter in-memory since Kubernetes field selectors
	// don't support OR conditions for the same field or complex time comparisons
	oneHourAgo := time.Now().Add(-1 * time.Hour)
	eventList, err := c.CoreV1().Events(namespace).List(ctx, metav1.ListOptions{
		// Remove all field selectors to avoid API errors - do filtering in-memory instead
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list events: %w", err)
	}

	// Filter events to only include FluxCD-related resources from the last hour
	fluxEvents := make([]corev1.Event, 0)
	for _, event := range eventList.Items {
		// Time-based filtering - only include events from the last hour
		if event.FirstTimestamp.Time.Before(oneHourAgo) && event.LastTimestamp.Time.Before(oneHourAgo) {
			continue
		}

		apiVersion := event.InvolvedObject.APIVersion
		// Check if event is related to FluxCD resources
		if apiVersion == "source.toolkit.fluxcd.io/v1" ||
			apiVersion == "source.toolkit.fluxcd.io/v1beta2" ||
			apiVersion == "kustomize.toolkit.fluxcd.io/v1" ||
			apiVersion == "helm.toolkit.fluxcd.io/v2beta1" ||
			apiVersion == "helm.toolkit.fluxcd.io/v2" {
			fluxEvents = append(fluxEvents, event)
		}
	}

	return fluxEvents, nil
}
