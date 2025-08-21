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

// DataMoverPopulatorSpec defines the desired state of DataMoverPopulator
type DataMoverPopulatorSpec struct {
	// SecretName is the name of the secret containing storage credentials
	// +kubebuilder:validation:Required
	SecretName string `json:"secretName"`

	// Path is the path to the data source (e.g., S3 bucket path)
	// +kubebuilder:validation:Required
	Path string `json:"path"`

	// Image specifies the container image to use for the population job
	// +optional
	Image *ImageSpec `json:"image,omitempty"`

	// AdditionalEnv allows specifying additional environment variables for the population job
	// +optional
	AdditionalEnv []corev1.EnvVar `json:"additionalEnv,omitempty"`
}

// DataMoverPopulatorStatus defines the observed state of DataMoverPopulator
type DataMoverPopulatorStatus struct {
	// Conditions represent the latest available observations of the populator's state
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// ObservedGeneration is the most recent generation observed for this DataMoverPopulator
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="SECRET",type="string",JSONPath=".spec.secretName",description="Secret containing storage credentials"
// +kubebuilder:printcolumn:name="PATH",type="string",JSONPath=".spec.path",description="Source path for data population"
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:resource:categories=datamover

// DataMoverPopulator is the Schema for the datamoverpopulators API
type DataMoverPopulator struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DataMoverPopulatorSpec   `json:"spec,omitempty"`
	Status DataMoverPopulatorStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// DataMoverPopulatorList contains a list of DataMoverPopulator
type DataMoverPopulatorList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DataMoverPopulator `json:"items"`
}

func init() {
	SchemeBuilder.Register(&DataMoverPopulator{}, &DataMoverPopulatorList{})
}
