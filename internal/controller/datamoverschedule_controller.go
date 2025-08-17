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
	"sort"
	"time"

	"github.com/go-logr/logr"
	"github.com/robfig/cron/v3"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	datamoverv1alpha1 "a-cup-of.coffee/datamover-operator/api/v1alpha1"
)

// DataMoverScheduleReconciler reconciles a DataMoverSchedule object
type DataMoverScheduleReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Log      logr.Logger
	Recorder record.EventRecorder
}

// +kubebuilder:rbac:groups=datamover.a-cup-of.coffee,resources=datamoverschedules,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=datamover.a-cup-of.coffee,resources=datamoverschedules/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=datamover.a-cup-of.coffee,resources=datamoverschedules/finalizers,verbs=update
// +kubebuilder:rbac:groups=datamover.a-cup-of.coffee,resources=datamovers,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *DataMoverScheduleReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Fetch the DataMoverSchedule instance
	var dataMoverSchedule datamoverv1alpha1.DataMoverSchedule
	if err := r.Get(ctx, req.NamespacedName, &dataMoverSchedule); err != nil {
		logger.Error(err, "unable to fetch DataMoverSchedule")
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Don't schedule anything if suspended
	if dataMoverSchedule.Spec.Suspend {
		logger.V(1).Info("DataMoverSchedule is suspended, skipping")
		return ctrl.Result{}, nil
	}

	// Parse the cron schedule
	cronSchedule, err := cron.ParseStandard(dataMoverSchedule.Spec.Schedule)
	if err != nil {
		logger.Error(err, "unable to parse cron schedule", "schedule", dataMoverSchedule.Spec.Schedule)
		r.Recorder.Eventf(&dataMoverSchedule, corev1.EventTypeWarning, "InvalidSchedule",
			"Invalid cron schedule: %s", dataMoverSchedule.Spec.Schedule)
		return ctrl.Result{}, err
	}

	// Get all DataMover jobs created by this DataMoverSchedule
	var childDataMovers datamoverv1alpha1.DataMoverList
	if err := r.List(ctx, &childDataMovers, client.InNamespace(req.Namespace),
		client.MatchingLabels{"datamoverschedule": req.Name}); err != nil {
		logger.Error(err, "unable to list child DataMovers")
		return ctrl.Result{}, err
	}

	// Separate active and finished jobs
	var activeJobs []*datamoverv1alpha1.DataMover
	var successfulJobs []*datamoverv1alpha1.DataMover
	var failedJobs []*datamoverv1alpha1.DataMover

	for i := range childDataMovers.Items {
		dataMover := &childDataMovers.Items[i]
		switch dataMover.Status.Phase {
		case "Completed":
			successfulJobs = append(successfulJobs, dataMover)
		case "Failed":
			failedJobs = append(failedJobs, dataMover)
		default:
			activeJobs = append(activeJobs, dataMover)
		}
	}

	// Sort jobs by creation timestamp
	sort.Slice(successfulJobs, func(i, j int) bool {
		return successfulJobs[i].CreationTimestamp.Before(&successfulJobs[j].CreationTimestamp)
	})
	sort.Slice(failedJobs, func(i, j int) bool {
		return failedJobs[i].CreationTimestamp.Before(&failedJobs[j].CreationTimestamp)
	})

	// Clean up old jobs based on history limits
	successfulJobsHistoryLimit := int32(3)
	if dataMoverSchedule.Spec.SuccessfulJobsHistoryLimit != nil {
		successfulJobsHistoryLimit = *dataMoverSchedule.Spec.SuccessfulJobsHistoryLimit
	}

	failedJobsHistoryLimit := int32(1)
	if dataMoverSchedule.Spec.FailedJobsHistoryLimit != nil {
		failedJobsHistoryLimit = *dataMoverSchedule.Spec.FailedJobsHistoryLimit
	}

	// Delete old successful jobs
	if int32(len(successfulJobs)) > successfulJobsHistoryLimit {
		for i := 0; i < len(successfulJobs)-int(successfulJobsHistoryLimit); i++ {
			if err := r.Delete(ctx, successfulJobs[i], client.PropagationPolicy(metav1.DeletePropagationBackground)); err != nil {
				logger.Error(err, "unable to delete old successful DataMover", "datamover", successfulJobs[i].Name)
			} else {
				logger.V(1).Info("deleted old successful DataMover", "datamover", successfulJobs[i].Name)
			}
		}
	}

	// Delete old failed jobs
	if int32(len(failedJobs)) > failedJobsHistoryLimit {
		for i := 0; i < len(failedJobs)-int(failedJobsHistoryLimit); i++ {
			if err := r.Delete(ctx, failedJobs[i], client.PropagationPolicy(metav1.DeletePropagationBackground)); err != nil {
				logger.Error(err, "unable to delete old failed DataMover", "datamover", failedJobs[i].Name)
			} else {
				logger.V(1).Info("deleted old failed DataMover", "datamover", failedJobs[i].Name)
			}
		}
	}

	// Calculate next scheduled time
	now := time.Now()
	nextTime := cronSchedule.Next(now)

	// Check if we should create a new job
	var lastScheduleTime *metav1.Time
	if dataMoverSchedule.Status.LastScheduleTime != nil {
		lastScheduleTime = dataMoverSchedule.Status.LastScheduleTime
	}

	scheduledTime := cronSchedule.Next(now.Add(-time.Second))
	if scheduledTime.After(now) {
		// Next schedule is in the future, wait
		logger.V(1).Info("next schedule is in the future", "scheduledTime", scheduledTime)
		return ctrl.Result{RequeueAfter: nextTime.Sub(now)}, nil
	}

	// Check if we already created a job for this schedule
	if lastScheduleTime != nil && scheduledTime.Before(lastScheduleTime.Add(time.Minute)) {
		// We already created a job for this minute
		logger.V(1).Info("job already created for this schedule", "scheduledTime", scheduledTime)
		return ctrl.Result{RequeueAfter: nextTime.Sub(now)}, nil
	}

	// Create new DataMover job
	dataMoverName := fmt.Sprintf("%s-%d", dataMoverSchedule.Name, scheduledTime.Unix())
	dataMover := &datamoverv1alpha1.DataMover{
		ObjectMeta: metav1.ObjectMeta{
			Name:      dataMoverName,
			Namespace: dataMoverSchedule.Namespace,
			Labels: map[string]string{
				"datamoverschedule":          dataMoverSchedule.Name,
				"datamoverschedule-schedule": fmt.Sprintf("%d", scheduledTime.Unix()),
			},
		},
		Spec: datamoverv1alpha1.DataMoverSpec{
			SourcePVC:            dataMoverSchedule.Spec.SourcePvc,
			SecretName:           dataMoverSchedule.Spec.SecretName,
			AddTimestampPrefix:   dataMoverSchedule.Spec.AddTimestampPrefix,
			DeletePvcAfterBackup: dataMoverSchedule.Spec.DeletePvcAfterBackup,
			AdditionalEnv:        dataMoverSchedule.Spec.AdditionalEnv,
			Image:                dataMoverSchedule.Spec.Image,
		},
	}

	// Set DataMoverSchedule as owner of the DataMover
	if err := controllerutil.SetControllerReference(&dataMoverSchedule, dataMover, r.Scheme); err != nil {
		logger.Error(err, "unable to set controller reference")
		return ctrl.Result{}, err
	}

	if err := r.Create(ctx, dataMover); err != nil {
		logger.Error(err, "unable to create DataMover job", "datamover", dataMoverName)
		r.Recorder.Eventf(&dataMoverSchedule, corev1.EventTypeWarning, "JobCreationFailed",
			"Failed to create DataMover job: %s", dataMoverName)
		return ctrl.Result{}, err
	}

	logger.Info("created DataMover job", "datamover", dataMoverName, "scheduledTime", scheduledTime)
	r.Recorder.Eventf(&dataMoverSchedule, corev1.EventTypeNormal, "JobCreated",
		"Created DataMover job: %s", dataMoverName)

	// Update status
	now = time.Now()
	dataMoverSchedule.Status.LastScheduleTime = &metav1.Time{Time: scheduledTime}

	// Update active jobs list
	activeRefs := make([]corev1.ObjectReference, 0, len(activeJobs)+1)
	for _, job := range activeJobs {
		activeRefs = append(activeRefs, corev1.ObjectReference{
			Kind:      "DataMover",
			Namespace: job.Namespace,
			Name:      job.Name,
			UID:       job.UID,
		})
	}
	// Add the new job to active list
	activeRefs = append(activeRefs, corev1.ObjectReference{
		Kind:      "DataMover",
		Namespace: dataMover.Namespace,
		Name:      dataMover.Name,
		UID:       dataMover.UID,
	})

	dataMoverSchedule.Status.Active = activeRefs
	dataMoverSchedule.Status.ActiveJobs = int32(len(activeRefs))
	dataMoverSchedule.Status.SuccessfulJobs = int32(len(successfulJobs))
	dataMoverSchedule.Status.FailedJobs = int32(len(failedJobs))

	if err := r.Status().Update(ctx, &dataMoverSchedule); err != nil {
		logger.Error(err, "unable to update DataMoverSchedule status")
		return ctrl.Result{}, err
	}

	// Requeue for next schedule
	return ctrl.Result{RequeueAfter: nextTime.Sub(now)}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *DataMoverScheduleReconciler) SetupWithManager(mgr ctrl.Manager) error {
	if r.Recorder == nil {
		r.Recorder = mgr.GetEventRecorderFor("datamoverschedule-controller")
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&datamoverv1alpha1.DataMoverSchedule{}).
		Owns(&datamoverv1alpha1.DataMover{}).
		Complete(r)
}
