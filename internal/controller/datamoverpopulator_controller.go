/*
Copyright 2025.

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
	"fmt"
	"time"

	"github.com/go-logr/logr"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	datamoverv1alpha1 "a-cup-of.coffee/datamover-operator/api/v1alpha1"
)

// DataMoverPopulatorReconciler reconciles PVCs with DataMoverPopulator dataSourceRef
type DataMoverPopulatorReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	Log    logr.Logger
}

// +kubebuilder:rbac:groups="",resources=persistentvolumeclaims,verbs=get;list;watch;update;patch;delete
// +kubebuilder:rbac:groups="",resources=persistentvolumes,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=datamover.a-cup-of.coffee,resources=datamoverpopulators,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch
// +kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch

func (r *DataMoverPopulatorReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	// Get the PVC
	var pvc corev1.PersistentVolumeClaim
	if err := r.Get(ctx, req.NamespacedName, &pvc); err != nil {
		if errors.IsNotFound(err) {
			log.Info("PVC not found, likely deleted")
			return ctrl.Result{}, nil
		}
		log.Error(err, "Failed to get PVC")
		return ctrl.Result{}, err
	}

	// Check if this PVC has a DataMoverPopulator dataSourceRef
	if pvc.Spec.DataSourceRef == nil ||
		pvc.Spec.DataSourceRef.APIGroup == nil ||
		*pvc.Spec.DataSourceRef.APIGroup != "datamover.a-cup-of.coffee" ||
		pvc.Spec.DataSourceRef.Kind != "DataMoverPopulator" {
		// Not our PVC, ignore
		return ctrl.Result{}, nil
	}

	// Get the DataMoverPopulator
	populatorName := pvc.Spec.DataSourceRef.Name
	var populator datamoverv1alpha1.DataMoverPopulator
	if err := r.Get(ctx, types.NamespacedName{
		Name:      populatorName,
		Namespace: pvc.Namespace,
	}, &populator); err != nil {
		if errors.IsNotFound(err) {
			// Check if PVC is already bound/completed - if so, don't keep retrying
			if pvc.Status.Phase == corev1.ClaimBound {
				log.Info("PVC is already bound, ignoring missing DataMoverPopulator", "pvc", pvc.Name, "populator", populatorName)
				return ctrl.Result{}, nil
			}
			// Check if PVC is already marked as populated
			if pvc.Annotations != nil {
				if populated, exists := pvc.Annotations["datamover.a-cup-of.coffee/populated"]; exists && populated == "true" {
					log.Info("PVC already populated, ignoring missing DataMoverPopulator", "pvc", pvc.Name, "populator", populatorName)
					return ctrl.Result{}, nil
				}
			}

			log.Info("DataMoverPopulator not found, will retry later", "populator", populatorName)
			return ctrl.Result{RequeueAfter: time.Second * 10}, nil
		}
		log.Error(err, "Failed to get DataMoverPopulator", "populator", populatorName)
		return ctrl.Result{}, err
	}

	// VolumePopulator pattern: We need to create a "prime" PVC first
	// Check if already populated AND cleanup is complete
	if pvc.Annotations != nil {
		if populated, exists := pvc.Annotations["datamover.a-cup-of.coffee/populated"]; exists && populated == "true" {
			// Also check that cleanup is complete (no prime PVC exists)
			primePVCName := fmt.Sprintf("%s-prime", pvc.Name)
			var primePVC corev1.PersistentVolumeClaim
			if err := r.Get(ctx, types.NamespacedName{
				Name:      primePVCName,
				Namespace: pvc.Namespace,
			}, &primePVC); errors.IsNotFound(err) {
				// Prime PVC is gone, we're truly done
				log.Info("PVC already populated and cleanup complete, nothing to do")
				return ctrl.Result{}, nil
			} else if err != nil {
				log.Error(err, "Error checking prime PVC existence")
				return ctrl.Result{}, err
			} else {
				// Prime PVC still exists, continue with cleanup
				log.Info("PVC marked as populated but cleanup not complete, continuing with prime PVC cleanup")
			}
		}
	}

	return r.ensurePopulationJob(ctx, &pvc, &populator)

}

func (r *DataMoverPopulatorReconciler) ensurePopulationJob(ctx context.Context, pvc *corev1.PersistentVolumeClaim, populator *datamoverv1alpha1.DataMoverPopulator) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	// Check if PVC is already populated
	if pvc.Annotations != nil {
		if populated, exists := pvc.Annotations["datamover.a-cup-of.coffee/populated"]; exists && populated == "true" {
			log.Info("PVC already populated", "pvc", pvc.Name)
			return ctrl.Result{}, nil
		}

		// Check if cleanup is in progress - if so, don't create or process jobs
		if cleanup, exists := pvc.Annotations["datamover.a-cup-of.coffee/cleanup-in-progress"]; exists && cleanup == "true" {
			log.Info("Cleanup is in progress, not processing any population job", "pvc", pvc.Name)
			// Continue with finalization if prime PVC exists
			primePVCName := fmt.Sprintf("%s-prime", pvc.Name)
			var primePVC corev1.PersistentVolumeClaim
			if err := r.Get(ctx, types.NamespacedName{
				Name:      primePVCName,
				Namespace: pvc.Namespace,
			}, &primePVC); err == nil {
				log.Info("Prime PVC exists during cleanup, continuing with finalization")
				return r.finalizePopulation(ctx, pvc, &primePVC, populator)
			}
			return ctrl.Result{RequeueAfter: time.Second * 3}, nil
		}

		if populating, exists := pvc.Annotations["datamover.a-cup-of.coffee/populating"]; exists && populating == "true" {
			log.Info("PVC is currently being populated, checking prime PVC status", "pvc", pvc.Name)
			// Continue to check prime PVC status, don't exit early
		}
	}

	primePVCName := fmt.Sprintf("%s-prime", pvc.Name)
	var primePVC corev1.PersistentVolumeClaim
	err := r.Get(ctx, types.NamespacedName{
		Name:      primePVCName,
		Namespace: pvc.Namespace,
	}, &primePVC)

	if err == nil {
		// Prime PVC exists, check if it's being deleted
		if primePVC.DeletionTimestamp != nil {
			log.Info("Prime PVC is being deleted, waiting for cleanup to complete", "primePVC", primePVCName)
			return ctrl.Result{RequeueAfter: time.Second * 5}, nil
		}
		// Prime PVC exists and is not being deleted, continue with normal flow
	} else if !errors.IsNotFound(err) {
		log.Error(err, "Failed to get prime PVC")
		return ctrl.Result{}, err
	}

	if errors.IsNotFound(err) {
		// Before creating a new prime PVC, check if we're in a cleanup state
		if pvc.Annotations != nil {
			// Don't create new jobs if cleanup is in progress
			if cleanup, exists := pvc.Annotations["datamover.a-cup-of.coffee/cleanup-in-progress"]; exists && cleanup == "true" {
				log.Info("Cleanup is in progress, not creating new population job", "pvc", pvc.Name)
				return ctrl.Result{RequeueAfter: time.Second * 3}, nil
			}

			if populating, exists := pvc.Annotations["datamover.a-cup-of.coffee/populating"]; exists && populating == "true" {
				log.Info("PVC is marked as populating but prime PVC not found, cleanup may be in progress", "pvc", pvc.Name)
				return ctrl.Result{RequeueAfter: time.Second * 5}, nil
			}
		}

		// Create the prime PVC
		primePVC = corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name:      primePVCName,
				Namespace: pvc.Namespace,
				Labels: map[string]string{
					"datamover.a-cup-of.coffee/prime-for": pvc.Name,
					"datamover.a-cup-of.coffee/populator": populator.Name,
				},
			},
			Spec: corev1.PersistentVolumeClaimSpec{
				AccessModes:      pvc.Spec.AccessModes,
				Resources:        pvc.Spec.Resources,
				StorageClassName: pvc.Spec.StorageClassName,
				VolumeMode:       pvc.Spec.VolumeMode,
				// Importantly: NO dataSourceRef here
			},
		}

		log.Info("Creating prime PVC for population", "primePVC", primePVCName)
		if err := r.Create(ctx, &primePVC); err != nil {
			log.Error(err, "Failed to create prime PVC")
			return ctrl.Result{}, err
		}
		return ctrl.Result{RequeueAfter: time.Second * 10}, nil
	} else if err != nil {
		log.Error(err, "Failed to get prime PVC")
		return ctrl.Result{}, err
	}

	if primePVC.Status.Phase != corev1.ClaimBound {
		log.Info("Prime PVC not yet bound, waiting", "phase", primePVC.Status.Phase)
		return ctrl.Result{RequeueAfter: time.Second * 10}, nil
	}

	jobName := fmt.Sprintf("datamover-populator-%s", primePVCName)
	var existingJob batchv1.Job
	err = r.Get(ctx, types.NamespacedName{
		Name:      jobName,
		Namespace: pvc.Namespace,
	}, &existingJob)

	if err == nil {
		// Job exists, check its status
		if existingJob.Status.Succeeded > 0 {
			log.Info("Population job completed successfully")

			// Step 4: Now we need to finalize the population by binding original PVC to prime volume
			return r.finalizePopulation(ctx, pvc, &primePVC, populator)
		}
		if existingJob.Status.Failed > 0 {
			log.Info("Population job failed, will retry")
			if err := r.Delete(ctx, &existingJob); err != nil {
				log.Error(err, "Failed to delete failed job")
				return ctrl.Result{}, err
			}
			return ctrl.Result{RequeueAfter: time.Minute * 2}, nil
		}
		// Job is still running
		log.Info("Population job is still running")
		return ctrl.Result{RequeueAfter: time.Minute * 1}, nil
	}

	if !errors.IsNotFound(err) {
		log.Error(err, "Failed to get population job")
		return ctrl.Result{}, err
	}

	// Create population job to populate the prime PVC
	job, err := r.createPopulationJob(ctx, &primePVC, populator) // Use primePVC instead of original PVC
	if err != nil {
		log.Error(err, "Failed to create population job")
		return ctrl.Result{}, err
	}

	if err := r.Create(ctx, job); err != nil {
		log.Error(err, "Failed to create population job")
		return ctrl.Result{}, err
	}

	log.Info("Created population job", "job", jobName)
	return ctrl.Result{RequeueAfter: time.Minute * 1}, nil
}

func (r *DataMoverPopulatorReconciler) finalizePopulation(ctx context.Context, originalPVC *corev1.PersistentVolumeClaim, primePVC *corev1.PersistentVolumeClaim, populator *datamoverv1alpha1.DataMoverPopulator) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	if primePVC.Annotations == nil {
		primePVC.Annotations = make(map[string]string)
	}

	primeUpdated := false
	if _, exists := primePVC.Annotations["datamover.a-cup-of.coffee/populated"]; !exists {
		primePVC.Annotations["datamover.a-cup-of.coffee/populated"] = "true"
		primePVC.Annotations["datamover.a-cup-of.coffee/populated-by"] = populator.Name
		primePVC.Annotations["datamover.a-cup-of.coffee/populated-at"] = time.Now().Format(time.RFC3339)
		primeUpdated = true
	}

	if primeUpdated {
		if err := r.Update(ctx, primePVC); err != nil {
			log.Error(err, "Failed to update prime PVC with population annotations")
			return ctrl.Result{}, err
		}
		log.Info("Prime PVC marked as populated", "primePVC", primePVC.Name)
	}

	// Mark the original PVC as "populating" (not "populated" yet - that comes after cleanup)
	if originalPVC.Annotations == nil {
		originalPVC.Annotations = make(map[string]string)
	}

	originalUpdated := false
	if _, exists := originalPVC.Annotations["datamover.a-cup-of.coffee/populating"]; !exists {
		originalPVC.Annotations["datamover.a-cup-of.coffee/populating"] = "true"
		originalPVC.Annotations["datamover.a-cup-of.coffee/populated-by"] = populator.Name
		originalPVC.Annotations["datamover.a-cup-of.coffee/populated-at"] = time.Now().Format(time.RFC3339)
		originalPVC.Annotations["datamover.a-cup-of.coffee/prime-pvc"] = primePVC.Name
		originalUpdated = true
	}

	if originalUpdated {
		if err := r.Update(ctx, originalPVC); err != nil {
			log.Error(err, "Failed to update original PVC with populating annotations")
			return ctrl.Result{}, err
		}
		log.Info("Original PVC marked as populating", "originalPVC", originalPVC.Name)
	}

	log.Info("Population finalized",
		"originalPVC", originalPVC.Name,
		"originalStatus", originalPVC.Status.Phase,
		"primePVC", primePVC.Name,
		"primeStatus", primePVC.Status.Phase,
		"primeVolume", primePVC.Spec.VolumeName)

	if originalPVC.Status.Phase == corev1.ClaimPending && primePVC.Status.Phase == corev1.ClaimBound {
		log.Info("Transferring volume from prime PVC to original PVC",
			"originalPVC", originalPVC.Name,
			"primePVC", primePVC.Name,
			"volume", primePVC.Spec.VolumeName)

		var pv corev1.PersistentVolume
		if err := r.Get(ctx, types.NamespacedName{Name: primePVC.Spec.VolumeName}, &pv); err != nil {
			log.Error(err, "Failed to get PersistentVolume", "volume", primePVC.Spec.VolumeName)
			return ctrl.Result{}, err
		}

		var freshOriginalPVC corev1.PersistentVolumeClaim
		if err := r.Get(ctx, types.NamespacedName{
			Name:      originalPVC.Name,
			Namespace: originalPVC.Namespace,
		}, &freshOriginalPVC); err != nil {
			log.Error(err, "Failed to get fresh copy of original PVC")
			return ctrl.Result{}, err
		}

		freshOriginalPVC.Spec.VolumeName = primePVC.Spec.VolumeName
		if freshOriginalPVC.Annotations == nil {
			freshOriginalPVC.Annotations = make(map[string]string)
		}
		freshOriginalPVC.Annotations["datamover.a-cup-of.coffee/cleanup-in-progress"] = "true"

		if err := r.Update(ctx, &freshOriginalPVC); err != nil {
			log.Error(err, "Failed to update original PVC with volume name and cleanup annotation")
			return ctrl.Result{}, err
		}
		log.Info("Updated original PVC with volume name and marked cleanup in progress", "volume", primePVC.Spec.VolumeName)

		jobName := fmt.Sprintf("datamover-populator-%s", primePVC.Name)
		var existingJob batchv1.Job
		jobErr := r.Get(ctx, types.NamespacedName{
			Name:      jobName,
			Namespace: primePVC.Namespace,
		}, &existingJob)

		if jobErr == nil {
			if existingJob.DeletionTimestamp == nil {
				log.Info("Deleting population job and its pods to release PVC", "job", jobName)

				// Use Background propagation policy to automatically delete child pods
				deletePolicy := metav1.DeletePropagationBackground
				deleteOptions := &client.DeleteOptions{
					PropagationPolicy: &deletePolicy,
				}

				if err := r.Delete(ctx, &existingJob, deleteOptions); err != nil {
					log.Error(err, "Failed to delete population job")
					return ctrl.Result{}, err
				}
				log.Info("Job deletion initiated, waiting for completion")
			} else {
				log.Info("Job is already being deleted, waiting for completion", "job", jobName)
			}
			return ctrl.Result{RequeueAfter: time.Second * 3}, nil
		} else if !errors.IsNotFound(jobErr) {
			log.Error(jobErr, "Failed to check job existence")
			return ctrl.Result{}, jobErr
		}

		log.Info("Job is deleted, now deleting prime PVC", "primePVC", primePVC.Name)
		var latestPrimePVC corev1.PersistentVolumeClaim
		if err := r.Get(ctx, types.NamespacedName{
			Name:      primePVC.Name,
			Namespace: primePVC.Namespace,
		}, &latestPrimePVC); err != nil {
			log.Error(err, "Failed to get latest prime PVC")
			return ctrl.Result{}, err
		}

		if latestPrimePVC.DeletionTimestamp != nil {
			log.Info("Prime PVC is already being deleted, waiting for completion", "primePVC", latestPrimePVC.Name)
		} else {
			log.Info("Deleting prime PVC", "primePVC", latestPrimePVC.Name)
			if err := r.Delete(ctx, &latestPrimePVC); err != nil {
				log.Error(err, "Failed to delete prime PVC")
				return ctrl.Result{}, err
			}
			log.Info("Initiated deletion of prime PVC", "primePVC", latestPrimePVC.Name)
		}

		// var deletedPrimePVC corev1.PersistentVolumeClaim
		// err := r.Get(ctx, types.NamespacedName{
		// 	Name:      primePVC.Name,
		// 	Namespace: primePVC.Namespace,
		// }, &deletedPrimePVC)

		log.Info("Prime PVC deleted, clearing PV claimRef to allow rebinding", "primePVC", primePVC.Name)

		maxRetries := 3
		for i := 0; i < maxRetries; i++ {
			var freshPV corev1.PersistentVolume
			// Get fresh copy to avoid version conflicts
			if err := r.Get(ctx, types.NamespacedName{Name: primePVC.Spec.VolumeName}, &freshPV); err != nil {
				log.Error(err, "Failed to get PersistentVolume", "volume", primePVC.Spec.VolumeName)
				return ctrl.Result{}, err
			}

			freshPV.Spec.ClaimRef = nil

			if err := r.Update(ctx, &freshPV); err != nil {
				if errors.IsConflict(err) && i < maxRetries-1 {
					log.Info("PV update conflict, retrying", "attempt", i+1, "volume", freshPV.Name)
					time.Sleep(time.Millisecond * 100 * time.Duration(i+1)) // exponential backoff
					continue
				}
				log.Error(err, "Failed to clear PV claimRef after retries")
				return ctrl.Result{}, err
			}

			log.Info("Successfully cleared PV claimRef, allowing rebinding to original PVC",
				"originalPVC", originalPVC.Name,
				"volume", freshPV.Name)
			break
		}

		// Final step: Mark the original PVC as fully populated with retry
		maxPVCRetries := 3
		for i := 0; i < maxPVCRetries; i++ {
			var freshFinalPVC corev1.PersistentVolumeClaim
			if err := r.Get(ctx, types.NamespacedName{
				Name:      originalPVC.Name,
				Namespace: originalPVC.Namespace,
			}, &freshFinalPVC); err != nil {
				log.Error(err, "Failed to get fresh copy of original PVC for final update")
				return ctrl.Result{}, err
			}

			if freshFinalPVC.Annotations == nil {
				freshFinalPVC.Annotations = make(map[string]string)
			}
			freshFinalPVC.Annotations["datamover.a-cup-of.coffee/populated"] = "true"
			// Remove the cleanup and populating annotations
			delete(freshFinalPVC.Annotations, "datamover.a-cup-of.coffee/populating")
			delete(freshFinalPVC.Annotations, "datamover.a-cup-of.coffee/cleanup-in-progress")

			if err := r.Update(ctx, &freshFinalPVC); err != nil {
				if errors.IsConflict(err) && i < maxPVCRetries-1 {
					log.Info("PVC final update conflict, retrying", "attempt", i+1, "pvc", freshFinalPVC.Name)
					time.Sleep(time.Millisecond * 100 * time.Duration(i+1))
					continue
				}
				log.Error(err, "Failed to mark original PVC as fully populated after retries")
				return ctrl.Result{}, err
			}

			log.Info("Original PVC marked as fully populated - VolumePopulator process complete",
				"originalPVC", originalPVC.Name)
			break
		}

		// Requeue to verify the binding is complete
		return ctrl.Result{RequeueAfter: time.Second * 5}, nil
	}

	return ctrl.Result{}, nil
}

func (r *DataMoverPopulatorReconciler) createPopulationJob(ctx context.Context, pvc *corev1.PersistentVolumeClaim, populator *datamoverv1alpha1.DataMoverPopulator) (*batchv1.Job, error) {
	log := log.FromContext(ctx)

	// Get the secret for storage credentials
	var secret corev1.Secret
	if err := r.Get(ctx, types.NamespacedName{
		Name:      populator.Spec.SecretName,
		Namespace: pvc.Namespace,
	}, &secret); err != nil {
		log.Error(err, "Failed to get storage credentials secret", "secret", populator.Spec.SecretName)
		return nil, err
	}

	// Build environment variables from secret
	var envVars []corev1.EnvVar
	for key := range secret.Data {
		envVars = append(envVars, corev1.EnvVar{
			Name: key,
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: populator.Spec.SecretName,
					},
					Key: key,
				},
			},
		})
	}

	// Add source path as environment variable
	envVars = append(envVars, corev1.EnvVar{
		Name:  "SOURCE_PATH",
		Value: populator.Spec.Path,
	})

	envVars = append(envVars, corev1.EnvVar{
		Name:  "POPULATION_MODE",
		Value: "true",
	})

	if len(populator.Spec.AdditionalEnv) > 0 {
		envVars = append(envVars, populator.Spec.AdditionalEnv...)
	}

	var image string
	var pullPolicy corev1.PullPolicy
	if populator.Spec.Image != nil {
		imageName := populator.Spec.Image.Repository
		if imageName == "" {
			imageName = "ghcr.io/qjoly/datamover-rclone"
		}

		imageTag := populator.Spec.Image.Tag
		if imageTag == "" {
			imageTag = "latest"
		}

		image = fmt.Sprintf("%s:%s", imageName, imageTag)

		pullPolicy = populator.Spec.Image.PullPolicy
		if pullPolicy == "" {
			pullPolicy = corev1.PullAlways
		}
	} else {
		image = "ghcr.io/qjoly/datamover-rclone:latest"
		pullPolicy = corev1.PullAlways
	}

	jobName := fmt.Sprintf("datamover-populator-%s", pvc.Name)
	backoffLimit := int32(2)

	// Security context for Pod Security Standards compliance
	runAsNonRoot := true
	runAsUser := int64(65534) // nobody user
	runAsGroup := int64(65534)
	allowPrivilegeEscalation := false

	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      jobName,
			Namespace: pvc.Namespace,
			Labels: map[string]string{
				"app.kubernetes.io/created-by":        "datamover-populator",
				"datamover.a-cup-of.coffee/populator": populator.Name,
				"datamover.a-cup-of.coffee/pvc":       pvc.Name,
			},
		},
		Spec: batchv1.JobSpec{
			BackoffLimit: &backoffLimit,
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					SecurityContext: &corev1.PodSecurityContext{
						RunAsNonRoot: &runAsNonRoot,
						RunAsUser:    &runAsUser,
						RunAsGroup:   &runAsGroup,
						SeccompProfile: &corev1.SeccompProfile{
							Type: corev1.SeccompProfileTypeRuntimeDefault,
						},
					},
					InitContainers: []corev1.Container{{
						Name:    "chown-data",
						Image:   image,
						Env:     envVars,
						Command: []string{"sh", "-c", "chmod a+rwx /data/"},
						SecurityContext: &corev1.SecurityContext{
							RunAsUser:  func() *int64 { i := int64(0); return &i }(),
							RunAsGroup: func() *int64 { i := int64(0); return &i }(),
							Capabilities: &corev1.Capabilities{
								Add: []corev1.Capability{"CHOWN"},
							},
						},
						VolumeMounts: []corev1.VolumeMount{
							{
								Name:      "target-data",
								MountPath: "/data/",
							},
						},
					}},
					Containers: []corev1.Container{{
						Name:            "population",
						Image:           image,
						ImagePullPolicy: pullPolicy,
						Env:             envVars,
						SecurityContext: &corev1.SecurityContext{
							AllowPrivilegeEscalation: &allowPrivilegeEscalation,
							RunAsNonRoot:             &runAsNonRoot,
							RunAsUser:                &runAsUser,
							RunAsGroup:               &runAsGroup,
							Capabilities: &corev1.Capabilities{
								Drop: []corev1.Capability{"ALL"},
							},
							SeccompProfile: &corev1.SeccompProfile{
								Type: corev1.SeccompProfileTypeRuntimeDefault,
							},
						},
						VolumeMounts: []corev1.VolumeMount{
							{
								Name:      "target-data",
								MountPath: "/data/",
							},
							{
								Name:      "config-dir",
								MountPath: "/config",
							},
						},
					}},
					Volumes: []corev1.Volume{
						{
							Name: "target-data",
							VolumeSource: corev1.VolumeSource{
								PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
									ClaimName: pvc.Name,
								},
							},
						},
						{
							Name: "config-dir",
							VolumeSource: corev1.VolumeSource{
								EmptyDir: &corev1.EmptyDirVolumeSource{},
							},
						},
					},
					RestartPolicy: corev1.RestartPolicyNever,
				},
			},
		},
	}

	// Set owner reference to PVC
	if err := controllerutil.SetControllerReference(pvc, job, r.Scheme); err != nil {
		return nil, err
	}

	return job, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *DataMoverPopulatorReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1.PersistentVolumeClaim{}).
		Owns(&batchv1.Job{}).
		Complete(r)
}
