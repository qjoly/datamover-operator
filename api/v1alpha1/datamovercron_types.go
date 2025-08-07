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

package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// DataMoverCronSpec defines the desired state of DataMoverCron
type DataMoverCronSpec struct {
	// Schedule defines the cron schedule for creating DataMover jobs
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Pattern=`^(\*|([0-9]|1[0-9]|2[0-9]|3[0-9]|4[0-9]|5[0-9])|\*\/([0-9]|1[0-9]|2[0-9]|3[0-9]|4[0-9]|5[0-9])) (\*|([0-9]|1[0-9]|2[0-3])|\*\/([0-9]|1[0-9]|2[0-3])) (\*|([1-9]|1[0-9]|2[0-9]|3[0-1])|\*\/([1-9]|1[0-9]|2[0-9]|3[0-1])) (\*|([1-9]|1[0-2])|\*\/([1-9]|1[0-2])) (\*|([0-6])|\*\/([0-6]))$`
	Schedule string `json:"schedule"`

	// SourcePvc is the name of the source PVC to clone
	// +kubebuilder:validation:Required
	SourcePvc string `json:"sourcePvc"`

	// SecretName is the name of the secret containing storage credentials
	// +kubebuilder:validation:Required
	SecretName string `json:"secretName"`

	// AddTimestampPrefix when true, creates timestamped folders (YYYY-MM-DD-HHMMSS/) for organized backups
	// +kubebuilder:default:=false
	// +optional
	AddTimestampPrefix bool `json:"addTimestampPrefix,omitempty"`

	// DeletePvcAfterBackup when true, automatically deletes the cloned PVC after successful backup
	// +kubebuilder:default:=false
	// +optional
	DeletePvcAfterBackup bool `json:"deletePvcAfterBackup,omitempty"`

	// AdditionalEnv allows specifying additional environment variables for the rclone job
	// +optional
	AdditionalEnv []corev1.EnvVar `json:"additionalEnv,omitempty"`

	// Suspend tells the controller to suspend subsequent executions, it does
	// not apply to already started executions. Defaults to false.
	// +kubebuilder:default:=false
	// +optional
	Suspend bool `json:"suspend,omitempty"`

	// SuccessfulJobsHistoryLimit is the number of successful finished jobs to retain.
	// Value must be non-negative integer. Defaults to 3.
	// +kubebuilder:default:=3
	// +kubebuilder:validation:Minimum=0
	// +optional
	SuccessfulJobsHistoryLimit *int32 `json:"successfulJobsHistoryLimit,omitempty"`

	// FailedJobsHistoryLimit is the number of failed finished jobs to retain.
	// Value must be non-negative integer. Defaults to 1.
	// +kubebuilder:default:=1
	// +kubebuilder:validation:Minimum=0
	// +optional
	FailedJobsHistoryLimit *int32 `json:"failedJobsHistoryLimit,omitempty"`
}

// DataMoverCronStatus defines the observed state of DataMoverCron
type DataMoverCronStatus struct {
	// Information when was the last time the job was successfully scheduled.
	// +optional
	LastScheduleTime *metav1.Time `json:"lastScheduleTime,omitempty"`

	// Information when was the last time the job successfully completed.
	// +optional
	LastSuccessfulTime *metav1.Time `json:"lastSuccessfulTime,omitempty"`

	// A list of pointers to currently running jobs.
	// +optional
	Active []corev1.ObjectReference `json:"active,omitempty"`

	// The number of currently running jobs.
	// +optional
	ActiveJobs int32 `json:"activeJobs,omitempty"`

	// The number of successful jobs.
	// +optional
	SuccessfulJobs int32 `json:"successfulJobs,omitempty"`

	// The number of failed jobs.
	// +optional
	FailedJobs int32 `json:"failedJobs,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Schedule",type="string",JSONPath=".spec.schedule"
// +kubebuilder:printcolumn:name="Suspend",type="boolean",JSONPath=".spec.suspend"
// +kubebuilder:printcolumn:name="Active",type="integer",JSONPath=".status.activeJobs"
// +kubebuilder:printcolumn:name="Last Schedule",type="date",JSONPath=".status.lastScheduleTime"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// DataMoverCron is the Schema for the datamovercrons API
type DataMoverCron struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DataMoverCronSpec   `json:"spec,omitempty"`
	Status DataMoverCronStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// DataMoverCronList contains a list of DataMoverCron
type DataMoverCronList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DataMoverCron `json:"items"`
}

func init() {
	SchemeBuilder.Register(&DataMoverCron{}, &DataMoverCronList{})
}
