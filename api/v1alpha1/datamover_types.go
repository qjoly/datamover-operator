package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// DataMoverSpec defines the desired state of DataMover
type DataMoverSpec struct {
	// The name of the source PersistentVolumeClaim (PVC) to clone.
	// +kubebuilder:validation:Required
	SourcePVC string `json:"sourcePvc"`

	// The name of the secret to mount in the verification pod.
	// +kubebuilder:validation:Required
	SecretName string `json:"secretName"`

	// Additional environment variables to add to the verification pod.
	// +kubebuilder:validation:Optional
	AdditionalEnv []corev1.EnvVar `json:"additionalEnv,omitempty"`

	// Whether to add a timestamp prefix to the destination folder in the bucket.
	// When true, data will be synced to a folder with format: YYYY-MM-DD-HHMMSS/
	// When false, data will be synced directly to the bucket root or configured path.
	// +kubebuilder:validation:Optional
	// +kubebuilder:default:=false
	AddTimestampPrefix bool `json:"addTimestampPrefix,omitempty"`

	// Whether to delete the cloned PVC after successful backup completion.
	// When true, the cloned PVC will be automatically deleted after successful data sync.
	// When false, the cloned PVC will be preserved for manual cleanup or further use.
	// +kubebuilder:validation:Optional
	// +kubebuilder:default:=false
	DeletePvcAfterBackup bool `json:"deletePvcAfterBackup,omitempty"`
}

// DataMoverStatus defines the observed state of DataMover
type DataMoverStatus struct {
	// Indicates the state of the cloning and verification process.
	Phase string `json:"phase,omitempty"`
	// A reference to the cloned PVC.
	RestoredPVCName string `json:"restoredPvcName,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="PHASE",type="string",JSONPath=".status.phase",description="Phase of the DataMover operation"
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=".metadata.creationTimestamp"
// DataMover is the Schema for the datamovers API
type DataMover struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DataMoverSpec   `json:"spec,omitempty"`
	Status DataMoverStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
// DataMoverList contains a list of DataMover
type DataMoverList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DataMover `json:"items"`
}

func init() {
	SchemeBuilder.Register(&DataMover{}, &DataMoverList{})
}
