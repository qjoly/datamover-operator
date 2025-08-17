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
	"sigs.k8s.io/controller-runtime/pkg/log"

	datamoverv1alpha1 "a-cup-of.coffee/datamover-operator/api/v1alpha1"
	"a-cup-of.coffee/datamover-operator/internal/metrics"
)

const (
	PhaseInitial     = ""
	PhaseCreatingPVC = "CreatingClonedPVC"
	PhasePVCReady    = "ClonedPVCReady"
	PhaseCreatingPod = "CreatingPod"
	PhaseCleaningUp  = "CleaningUp"
	PhaseCompleted   = "Completed"
	PhaseFailed      = "Failed"
)

// DataMoverReconciler reconciles a DataMover object
type DataMoverReconciler struct {
	client.Client
	Scheme     *runtime.Scheme
	Log        logr.Logger
	PhaseStart map[string]time.Time // Track phase start times for metrics
}

// +kubebuilder:rbac:groups=datamover.a-cup-of.coffee,resources=datamovers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=datamover.a-cup-of.coffee,resources=datamovers/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=datamover.a-cup-of.coffee,resources=datamovers/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=persistentvolumeclaims,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch
func (r *DataMoverReconciler) Reconcile(
	ctx context.Context,
	req ctrl.Request,
) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Initialize phase tracking map if needed
	if r.PhaseStart == nil {
		r.PhaseStart = make(map[string]time.Time)
	}

	// 1. Get the DataMover instance
	var dataMover datamoverv1alpha1.DataMover
	if err := r.Get(ctx, req.NamespacedName, &dataMover); err != nil {
		if errors.IsNotFound(err) {
			logger.Info("DataMover resource not found. Ignoring since object must be deleted.")
			// Clean up metrics for deleted resource
			metrics.DataMoverCurrentPhase.DeleteLabelValues(req.Name, req.Namespace)
			//nolint:staticcheck // QF1008: Keeping explicit field name for clarity
			delete(r.PhaseStart, req.NamespacedName.String())
			return ctrl.Result{}, nil
		}
		logger.Error(err, "Failed to get DataMover")
		metrics.RecordError("get_datamover", dataMover.Status.Phase, req.Namespace)
		return ctrl.Result{}, err
	}

	// Update current phase metric
	metrics.SetCurrentPhase(
		dataMover.Name,
		dataMover.Namespace,
		metrics.GetPhaseMetricValue(dataMover.Status.Phase),
	)

	// Use a switch on the current phase to manage the lifecycle
	switch dataMover.Status.Phase {
	case PhaseInitial:
		// Initial phase: create cloned PVC
		logger.Info("Phase: Creating cloned PVC")
		metrics.RecordOperationStart(PhaseCreatingPVC, dataMover.Namespace)
		//nolint:staticcheck // QF1008: Keeping explicit field name for clarity
		r.PhaseStart[req.NamespacedName.String()+"-"+PhaseCreatingPVC] = time.Now()
		return r.createClonedPVC(ctx, &dataMover)
	case PhaseCreatingPVC:
		// Wait for PVC availability
		logger.Info("Phase: Waiting for cloned PVC to be bound")
		return r.waitForPVCBound(ctx, &dataMover)
	case PhasePVCReady:
		// PVC ready: create verification Pod
		logger.Info("Phase: Creating verification Pod")
		// Record PVC creation phase completion
		//nolint:staticcheck // QF1008: Keeping explicit field name for clarity
		if startTime, exists := r.PhaseStart[req.NamespacedName.String()+"-"+PhaseCreatingPVC]; exists {
			duration := time.Since(startTime).Seconds()
			metrics.RecordPhaseDuration(PhaseCreatingPVC, dataMover.Namespace, duration)
			//nolint:staticcheck // QF1008: Keeping explicit field name for clarity
			delete(r.PhaseStart, req.NamespacedName.String()+"-"+PhaseCreatingPVC)
		}
		metrics.RecordOperationSuccess(PhaseCreatingPVC, dataMover.Namespace)
		metrics.RecordOperationStart(PhaseCreatingPod, dataMover.Namespace)
		//nolint:staticcheck // QF1008: Keeping explicit field name for clarity
		r.PhaseStart[req.NamespacedName.String()+"-"+PhaseCreatingPod] = time.Now()
		return r.createVerificationJob(ctx, &dataMover)
	case PhaseCreatingPod:
		// Wait for job to complete
		logger.Info("Phase: Waiting for job to complete")
		return r.waitForJobCompletion(ctx, &dataMover)
	case PhaseCleaningUp:
		// Clean up cloned PVC if requested
		logger.Info("Phase: Cleaning up cloned PVC")
		return r.cleanupClonedPVC(ctx, &dataMover)
	case PhaseCompleted:
		// Completed, do nothing
		logger.Info("Phase: Completed. No more actions.")
		return ctrl.Result{}, nil
	case PhaseFailed:
		// Failed, do nothing
		logger.Info("Phase: Failed. No more actions.")
		// Record failure for any ongoing phase
		for key := range r.PhaseStart {
			//nolint:staticcheck // QF1008: Keeping explicit field name for clarity
			if req.NamespacedName.String() == key[:len(req.NamespacedName.String())] {
				delete(r.PhaseStart, key)
			}
		}
		metrics.RecordOperationFailure(dataMover.Status.Phase, dataMover.Namespace)
		metrics.RecordDataSyncOperation("failure", dataMover.Namespace)
		return ctrl.Result{}, nil
	default:
		logger.Info("Unknown phase, re-queuing.")
		return ctrl.Result{Requeue: true}, nil
	}
}

// --- STEP LOGIC ---

func (r *DataMoverReconciler) createClonedPVC(
	ctx context.Context,
	dm *datamoverv1alpha1.DataMover,
) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	clonedPVCName := fmt.Sprintf("%s-cloned-%d", dm.Spec.SourcePVC, time.Now().Unix())

	// Get the source PVC size for cloning
	var sourcePVC corev1.PersistentVolumeClaim
	if err := r.Get(ctx, types.NamespacedName{Name: dm.Spec.SourcePVC, Namespace: dm.Namespace}, &sourcePVC); err != nil {
		logger.Error(err, "Failed to get source PVC to determine size")
		metrics.RecordError("source_pvc_not_found", PhaseCreatingPVC, dm.Namespace)
		metrics.RecordPVCCloneOperation("failure", dm.Namespace)
		return ctrl.Result{}, err
	}
	pvcSize := sourcePVC.Spec.Resources.Requests[corev1.ResourceStorage]

	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      clonedPVCName,
			Namespace: dm.Namespace,
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: sourcePVC.Spec.AccessModes,
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: pvcSize,
				},
			},
			DataSource: &corev1.TypedLocalObjectReference{
				Kind: "PersistentVolumeClaim",
				Name: dm.Spec.SourcePVC,
			},
			StorageClassName: sourcePVC.Spec.StorageClassName,
		},
	}

	if err := r.Create(ctx, pvc); err != nil {
		logger.Error(err, "Failed to create cloned PVC")
		metrics.RecordError("pvc_creation_failed", PhaseCreatingPVC, dm.Namespace)
		metrics.RecordPVCCloneOperation("failure", dm.Namespace)
		return ctrl.Result{}, err
	}

	logger.Info("Successfully created cloned PVC", "pvcName", clonedPVCName)
	metrics.RecordPVCCloneOperation("started", dm.Namespace)

	dm.Status.Phase = PhaseCreatingPVC
	dm.Status.RestoredPVCName = clonedPVCName
	if err := r.Status().Update(ctx, dm); err != nil {
		metrics.RecordError("status_update_failed", PhaseCreatingPVC, dm.Namespace)
		return ctrl.Result{}, err
	}

	return ctrl.Result{Requeue: true}, nil
}

func (r *DataMoverReconciler) waitForPVCBound(
	ctx context.Context,
	dm *datamoverv1alpha1.DataMover,
) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	var pvc corev1.PersistentVolumeClaim
	pvcKey := types.NamespacedName{Name: dm.Status.RestoredPVCName, Namespace: dm.Namespace}

	if err := r.Get(ctx, pvcKey, &pvc); err != nil {
		logger.Error(err, "Failed to get cloned PVC")
		metrics.RecordError("pvc_get_failed", PhaseCreatingPVC, dm.Namespace)
		return ctrl.Result{}, err
	}

	if pvc.Status.Phase == corev1.ClaimBound {
		logger.Info("Cloned PVC is bound")
		metrics.RecordPVCCloneOperation("success", dm.Namespace)
		dm.Status.Phase = PhasePVCReady
		if err := r.Status().Update(ctx, dm); err != nil {
			metrics.RecordError("status_update_failed", PhasePVCReady, dm.Namespace)
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}

	logger.Info(
		"Waiting for cloned PVC to be bound...",
		"PVCName",
		dm.Status.RestoredPVCName,
		"CurrentPhase",
		pvc.Status.Phase,
	)
	return ctrl.Result{RequeueAfter: 15 * time.Second}, nil
}

func (r *DataMoverReconciler) createVerificationJob(
	ctx context.Context,
	dm *datamoverv1alpha1.DataMover,
) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	jobName := fmt.Sprintf("verify-%s", dm.Status.RestoredPVCName)

	// Build the list of environment variables
	envVars := make([]corev1.EnvVar, 0)

	// Add timestamp prefix configuration
	if dm.Spec.AddTimestampPrefix {
		envVars = append(envVars, corev1.EnvVar{
			Name:  "ADD_TIMESTAMP_PREFIX",
			Value: "true",
		})
	} else {
		envVars = append(envVars, corev1.EnvVar{
			Name:  "ADD_TIMESTAMP_PREFIX",
			Value: "false",
		})
	}

	// Add additional environment variables if specified
	if len(dm.Spec.AdditionalEnv) > 0 {
		envVars = append(envVars, dm.Spec.AdditionalEnv...)
	}

	// Get image configuration with defaults
	imageName := dm.Spec.Image.Name
	if imageName == "" {
		imageName = "ghcr.io/qjoly/datamover-rclone"
	}

	imageTag := dm.Spec.Image.Tag
	if imageTag == "" {
		imageTag = "latest"
	}

	pullPolicy := dm.Spec.Image.PullPolicy
	if pullPolicy == "" {
		pullPolicy = corev1.PullAlways
	}

	fullImageName := fmt.Sprintf("%s:%s", imageName, imageTag)

	// Set backoffLimit to 2 for 3 total attempts (initial + 2 retries)
	backoffLimit := int32(2)

	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      jobName,
			Namespace: dm.Namespace,
		},
		Spec: batchv1.JobSpec{
			BackoffLimit: &backoffLimit,
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					SecurityContext: &corev1.PodSecurityContext{
						RunAsNonRoot: &[]bool{true}[0],
						RunAsUser:    &[]int64{65534}[0], // nobody user
						RunAsGroup:   &[]int64{65534}[0], // nobody group
						FSGroup:      &[]int64{65534}[0],
						SeccompProfile: &corev1.SeccompProfile{
							Type: corev1.SeccompProfileTypeRuntimeDefault,
						},
					},
					Containers: []corev1.Container{{
						Name:            "rclone",
						Image:           fullImageName,
						ImagePullPolicy: pullPolicy,
						SecurityContext: &corev1.SecurityContext{
							AllowPrivilegeEscalation: &[]bool{false}[0],
							RunAsNonRoot:             &[]bool{true}[0],
							RunAsUser:                &[]int64{65534}[0],
							RunAsGroup:               &[]int64{65534}[0],
							ReadOnlyRootFilesystem:   &[]bool{true}[0],
							Capabilities: &corev1.Capabilities{
								Drop: []corev1.Capability{"ALL"},
							},
							SeccompProfile: &corev1.SeccompProfile{
								Type: corev1.SeccompProfileTypeRuntimeDefault,
							},
						},
						Env: envVars,
						EnvFrom: []corev1.EnvFromSource{
							{
								SecretRef: &corev1.SecretEnvSource{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: dm.Spec.SecretName,
									},
								},
							},
						},
						VolumeMounts: []corev1.VolumeMount{
							{
								Name:      "restored-data",
								MountPath: "/data/",
							},
						},
					}},
					Volumes: []corev1.Volume{
						{
							Name: "restored-data",
							VolumeSource: corev1.VolumeSource{
								PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
									ClaimName: dm.Status.RestoredPVCName,
								},
							},
						},
					},
					RestartPolicy: corev1.RestartPolicyNever,
				},
			},
		},
	}

	// Check if the job already exists
	foundJob := &batchv1.Job{}
	err := r.Get(ctx, types.NamespacedName{Name: jobName, Namespace: dm.Namespace}, foundJob)
	if err != nil && errors.IsNotFound(err) {
		if err := r.Create(ctx, job); err != nil {
			logger.Error(err, "Failed to create verification job")
			metrics.RecordError("job_creation_failed", PhaseCreatingPod, dm.Namespace)
			metrics.RecordPodCreationOperation("failure", dm.Namespace)
			return ctrl.Result{}, err
		}
		logger.Info("Successfully created verification job", "jobName", jobName)
		metrics.RecordPodCreationOperation("started", dm.Namespace)
		dm.Status.Phase = PhaseCreatingPod
		if err := r.Status().Update(ctx, dm); err != nil {
			metrics.RecordError("status_update_failed", PhaseCreatingPod, dm.Namespace)
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	} else if err != nil {
		logger.Error(err, "Failed to check if job exists")
		metrics.RecordError("job_get_failed", PhaseCreatingPod, dm.Namespace)
		return ctrl.Result{}, err
	}

	// If the job already exists, move to the next step
	logger.Info("Verification job already exists", "jobName", jobName)
	dm.Status.Phase = PhaseCreatingPod
	if err := r.Status().Update(ctx, dm); err != nil {
		metrics.RecordError("status_update_failed", PhaseCreatingPod, dm.Namespace)
		return ctrl.Result{}, err
	}

	return ctrl.Result{Requeue: true}, nil
}

func (r *DataMoverReconciler) waitForJobCompletion(
	ctx context.Context,
	dm *datamoverv1alpha1.DataMover,
) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	jobName := fmt.Sprintf("verify-%s", dm.Status.RestoredPVCName)
	var job batchv1.Job
	jobKey := types.NamespacedName{Name: jobName, Namespace: dm.Namespace}

	if err := r.Get(ctx, jobKey, &job); err != nil {
		logger.Error(err, "Failed to get verification Job")
		metrics.RecordError("job_get_failed", PhaseCreatingPod, dm.Namespace)
		return ctrl.Result{}, err
	}

	// Check if job completed successfully
	if job.Status.Succeeded > 0 {
		logger.Info("Verification Job completed successfully.")
		// Record pod creation phase completion
		if startTime, exists := r.PhaseStart[types.NamespacedName{Name: dm.Name, Namespace: dm.Namespace}.String()+"-"+PhaseCreatingPod]; exists {
			duration := time.Since(startTime).Seconds()
			metrics.RecordPhaseDuration(PhaseCreatingPod, dm.Namespace, duration)
			delete(
				r.PhaseStart,
				types.NamespacedName{
					Name:      dm.Name,
					Namespace: dm.Namespace,
				}.String()+"-"+PhaseCreatingPod,
			)
		}
		metrics.RecordPodCreationOperation("success", dm.Namespace)
		metrics.RecordDataSyncOperation("success", dm.Namespace)

		// Check if we should delete the PVC after backup
		if dm.Spec.DeletePvcAfterBackup {
			logger.Info("DeletePvcAfterBackup enabled, moving to cleanup phase")
			dm.Status.Phase = PhaseCleaningUp
		} else {
			logger.Info("DeletePvcAfterBackup disabled, completing operation")
			dm.Status.Phase = PhaseCompleted
		}

		if err := r.Status().Update(ctx, dm); err != nil {
			metrics.RecordError("status_update_failed", dm.Status.Phase, dm.Namespace)
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}

	// Check if job failed (reached backoff limit or has failed conditions)
	if job.Status.Failed > 0 {
		// Check if we reached the backoff limit
		if job.Spec.BackoffLimit != nil && job.Status.Failed >= *job.Spec.BackoffLimit+1 {
			logger.Error(nil, "Verification Job failed after all retries. DataMover process failed.",
				"attempts", job.Status.Failed, "backoffLimit", *job.Spec.BackoffLimit)
		} else {
			logger.Error(nil, "Verification Job failed. DataMover process failed.",
				"attempts", job.Status.Failed)
		}
		metrics.RecordError("job_failed", PhaseCreatingPod, dm.Namespace)
		metrics.RecordPodCreationOperation("failure", dm.Namespace)
		dm.Status.Phase = PhaseFailed
		if err := r.Status().Update(ctx, dm); err != nil {
			metrics.RecordError("status_update_failed", PhaseFailed, dm.Namespace)
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	// Job is still running or pending
	logger.Info("Waiting for verification Job to complete...",
		"Active", job.Status.Active,
		"Succeeded", job.Status.Succeeded,
		"Failed", job.Status.Failed)
	return ctrl.Result{RequeueAfter: 15 * time.Second}, nil
}

func (r *DataMoverReconciler) cleanupClonedPVC(
	ctx context.Context,
	dm *datamoverv1alpha1.DataMover,
) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	if dm.Status.RestoredPVCName == "" {
		logger.Info("No cloned PVC to cleanup, completing operation")
		dm.Status.Phase = PhaseCompleted
		if err := r.Status().Update(ctx, dm); err != nil {
			metrics.RecordError("status_update_failed", PhaseCompleted, dm.Namespace)
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	// Check if PVC exists before trying to delete it
	var pvc corev1.PersistentVolumeClaim
	pvcKey := types.NamespacedName{Name: dm.Status.RestoredPVCName, Namespace: dm.Namespace}

	err := r.Get(ctx, pvcKey, &pvc)
	if err != nil {
		if errors.IsNotFound(err) {
			logger.Info(
				"Cloned PVC already deleted or not found",
				"pvcName",
				dm.Status.RestoredPVCName,
			)
			metrics.RecordPVCCleanupOperation("already_deleted", dm.Namespace)
			dm.Status.Phase = PhaseCompleted
			if err := r.Status().Update(ctx, dm); err != nil {
				metrics.RecordError("status_update_failed", PhaseCompleted, dm.Namespace)
				return ctrl.Result{}, err
			}
			return ctrl.Result{}, nil
		}
		logger.Error(err, "Failed to get cloned PVC for cleanup")
		metrics.RecordError("pvc_get_failed", PhaseCleaningUp, dm.Namespace)
		return ctrl.Result{}, err
	}

	// Delete the cloned PVC
	logger.Info("Deleting cloned PVC", "pvcName", dm.Status.RestoredPVCName)
	if err := r.Delete(ctx, &pvc); err != nil {
		logger.Error(err, "Failed to delete cloned PVC")
		metrics.RecordError("pvc_delete_failed", PhaseCleaningUp, dm.Namespace)
		metrics.RecordPVCCleanupOperation("failure", dm.Namespace)
		dm.Status.Phase = PhaseFailed
		if err := r.Status().Update(ctx, dm); err != nil {
			metrics.RecordError("status_update_failed", PhaseFailed, dm.Namespace)
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	logger.Info(
		"Successfully deleted cloned PVC, completing operation",
		"pvcName",
		dm.Status.RestoredPVCName,
	)
	metrics.RecordPVCCleanupOperation("success", dm.Namespace)
	dm.Status.Phase = PhaseCompleted
	if err := r.Status().Update(ctx, dm); err != nil {
		metrics.RecordError("status_update_failed", PhaseCompleted, dm.Namespace)
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *DataMoverReconciler) SetupWithManager(mgr ctrl.Manager) error {
	// We also need to "own" the created objects so that Reconcile is triggered if they change
	return ctrl.NewControllerManagedBy(mgr).
		For(&datamoverv1alpha1.DataMover{}).
		Owns(&corev1.PersistentVolumeClaim{}).
		Owns(&batchv1.Job{}).
		Complete(r)
}
