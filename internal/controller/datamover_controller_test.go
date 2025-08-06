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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	datamoverv1alpha1 "a-cup-of.coffee/datamover-operator/api/v1alpha1"
)

var _ = Describe("DataMover Controller", func() {
	Context("When reconciling a resource", func() {
		const resourceName = "test-resource"

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: "default", // TODO(user):Modify as needed
		}
		datamover := &datamoverv1alpha1.DataMover{}

		BeforeEach(func() {
			By("creating the source PVC")
			sourcePVC := &corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-source-pvc",
					Namespace: "default",
				},
				Spec: corev1.PersistentVolumeClaimSpec{
					AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
					Resources: corev1.VolumeResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceStorage: resource.MustParse("1Gi"),
						},
					},
				},
			}
			err := k8sClient.Get(
				ctx,
				types.NamespacedName{Name: "test-source-pvc", Namespace: "default"},
				&corev1.PersistentVolumeClaim{},
			)
			if err != nil && errors.IsNotFound(err) {
				Expect(k8sClient.Create(ctx, sourcePVC)).To(Succeed())
			}

			By("creating the test secret")
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-secret",
					Namespace: "default",
				},
				Data: map[string][]byte{
					"AWS_ACCESS_KEY_ID":     []byte("test-access-key"),
					"AWS_SECRET_ACCESS_KEY": []byte("test-secret-key"),
				},
			}
			err = k8sClient.Get(
				ctx,
				types.NamespacedName{Name: "test-secret", Namespace: "default"},
				&corev1.Secret{},
			)
			if err != nil && errors.IsNotFound(err) {
				Expect(k8sClient.Create(ctx, secret)).To(Succeed())
			}

			By("creating the custom resource for the Kind DataMover")
			err = k8sClient.Get(ctx, typeNamespacedName, datamover)
			if err != nil && errors.IsNotFound(err) {
				resource := &datamoverv1alpha1.DataMover{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceName,
						Namespace: "default",
					},
					Spec: datamoverv1alpha1.DataMoverSpec{
						SourcePVC:            "test-source-pvc",
						SecretName:           "test-secret",
						AddTimestampPrefix:   true,
						DeletePvcAfterBackup: false, // Keep PVC for test verification
					},
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}
		})

		AfterEach(func() {
			// Cleanup logic after each test
			By("Cleanup the specific resource instance DataMover")
			resource := &datamoverv1alpha1.DataMover{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			if err == nil {
				Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
			}

			By("Cleanup the source PVC")
			sourcePVC := &corev1.PersistentVolumeClaim{}
			err = k8sClient.Get(
				ctx,
				types.NamespacedName{Name: "test-source-pvc", Namespace: "default"},
				sourcePVC,
			)
			if err == nil {
				Expect(k8sClient.Delete(ctx, sourcePVC)).To(Succeed())
			}

			By("Cleanup the test secret")
			secret := &corev1.Secret{}
			err = k8sClient.Get(
				ctx,
				types.NamespacedName{Name: "test-secret", Namespace: "default"},
				secret,
			)
			if err == nil {
				Expect(k8sClient.Delete(ctx, secret)).To(Succeed())
			}
		})
		It("should successfully reconcile the resource", func() {
			By("Reconciling the created resource")
			controllerReconciler := &DataMoverReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())
			// TODO(user): Add more specific assertions depending on your controller's reconciliation logic.
			// Example: If you expect a certain status condition after reconciliation, verify it here.
		})
	})
})
