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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	datamoverv1alpha1 "a-cup-of.coffee/datamover-operator/api/v1alpha1"
)

var _ = Describe("DataMoverPopulator Controller", func() {
	Context("When reconciling a resource", func() {
		const (
			namespace = "default"
			timeout   = time.Second * 10
			interval  = time.Millisecond * 250
		)

		ctx := context.Background()

		It("should create a population job when PVC is bound", func() {
			populatorName := "test-populator-1"
			pvcName := "test-pvc-1"
			secretName := "test-secret-1"

			typeNamespacedName := types.NamespacedName{
				Name:      pvcName,
				Namespace: namespace,
			}

			By("creating the custom resource for the Kind DataMoverPopulator")
			populator := &datamoverv1alpha1.DataMoverPopulator{
				ObjectMeta: metav1.ObjectMeta{
					Name:      populatorName,
					Namespace: namespace,
				},
				Spec: datamoverv1alpha1.DataMoverPopulatorSpec{
					SecretName: secretName,
					Path:       "s3://test-bucket/test-path/",
					AdditionalEnv: []corev1.EnvVar{
						{
							Name:  "TEST_ENV",
							Value: "test-value",
						},
					},
				},
			}
			err := k8sClient.Create(ctx, populator)
			Expect(err).NotTo(HaveOccurred())

			By("creating the secret for storage credentials")
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      secretName,
					Namespace: namespace,
				},
				Type: corev1.SecretTypeOpaque,
				Data: map[string][]byte{
					"AWS_ACCESS_KEY_ID":     []byte("test-access-key"),
					"AWS_SECRET_ACCESS_KEY": []byte("test-secret-key"),
					"BUCKET_HOST":           []byte("s3.amazonaws.com"),
					"BUCKET_NAME":           []byte("test-bucket"),
				},
			}
			err = k8sClient.Create(ctx, secret)
			Expect(err).NotTo(HaveOccurred())

			By("creating the PVC with dataSourceRef")
			pvc := &corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      pvcName,
					Namespace: namespace,
				},
				Spec: corev1.PersistentVolumeClaimSpec{
					AccessModes: []corev1.PersistentVolumeAccessMode{
						corev1.ReadWriteOnce,
					},
					Resources: corev1.VolumeResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceStorage: resource.MustParse("1Gi"),
						},
					},
					DataSourceRef: &corev1.TypedObjectReference{
						APIGroup: func() *string {
							group := "datamover.a-cup-of.coffee"
							return &group
						}(),
						Kind: "DataMoverPopulator",
						Name: populatorName,
					},
				},
			}
			err = k8sClient.Create(ctx, pvc)
			Expect(err).NotTo(HaveOccurred())

			By("Simulating PVC bound state for testing")
			// Update PVC status to simulate it being bound by a storage provisioner
			Eventually(func() error {
				if err := k8sClient.Get(ctx, typeNamespacedName, pvc); err != nil {
					return err
				}
				pvc.Status.Phase = corev1.ClaimBound
				return k8sClient.Status().Update(ctx, pvc)
			}, timeout, interval).Should(Succeed())

			By("Reconciling the created resource")
			controllerReconciler := &DataMoverPopulatorReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			By("Checking if population job was created")
			jobName := fmt.Sprintf("datamover-populator-%s", pvcName)
			foundJob := &batchv1.Job{}
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{
					Name:      jobName,
					Namespace: namespace,
				}, foundJob)
			}, timeout, interval).Should(Succeed())

			By("Verifying job configuration")
			Expect(foundJob.Spec.Template.Spec.Containers).To(HaveLen(1))
			container := foundJob.Spec.Template.Spec.Containers[0]
			Expect(container.Image).To(Equal("ghcr.io/qjoly/datamover-rclone:latest"))
			Expect(container.VolumeMounts).To(HaveLen(2))

			// Check environment variables
			var sourcePathFound, populationModeFound bool
			for _, env := range container.Env {
				if env.Name == "SOURCE_PATH" && env.Value == "s3://test-bucket/test-path/" {
					sourcePathFound = true
				}
				if env.Name == "POPULATION_MODE" && env.Value == "true" {
					populationModeFound = true
				}
			}
			Expect(sourcePathFound).To(BeTrue())
			Expect(populationModeFound).To(BeTrue())

			By("Cleanup resources")
			Expect(k8sClient.Delete(ctx, pvc)).To(Succeed())
			Expect(k8sClient.Delete(ctx, populator)).To(Succeed())
			Expect(k8sClient.Delete(ctx, secret)).To(Succeed())
		})

		It("should use custom image configuration when specified", func() {
			populatorName := "custom-image-populator"
			pvcName := "custom-pvc"
			secretName := "custom-secret"

			By("Creating the secret for storage credentials")
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      secretName,
					Namespace: namespace,
				},
				Type: corev1.SecretTypeOpaque,
				Data: map[string][]byte{
					"AWS_ACCESS_KEY_ID":     []byte("test-access-key"),
					"AWS_SECRET_ACCESS_KEY": []byte("test-secret-key"),
					"BUCKET_HOST":           []byte("s3.amazonaws.com"),
					"BUCKET_NAME":           []byte("test-bucket"),
				},
			}
			err := k8sClient.Create(ctx, secret)
			Expect(err).NotTo(HaveOccurred())

			By("Creating a DataMoverPopulator with custom image")
			customPopulator := &datamoverv1alpha1.DataMoverPopulator{
				ObjectMeta: metav1.ObjectMeta{
					Name:      populatorName,
					Namespace: namespace,
				},
				Spec: datamoverv1alpha1.DataMoverPopulatorSpec{
					SecretName: secretName,
					Path:       "s3://test-bucket/custom-path/",
					Image: &datamoverv1alpha1.ImageSpec{
						Repository: "myregistry.com/custom-rclone",
						Tag:        "v2.0.0",
						PullPolicy: corev1.PullNever,
					},
				},
			}
			err = k8sClient.Create(ctx, customPopulator)
			Expect(err).NotTo(HaveOccurred())

			By("Creating a PVC with custom populator")
			customPVC := &corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      pvcName,
					Namespace: namespace,
				},
				Spec: corev1.PersistentVolumeClaimSpec{
					AccessModes: []corev1.PersistentVolumeAccessMode{
						corev1.ReadWriteOnce,
					},
					Resources: corev1.VolumeResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceStorage: resource.MustParse("1Gi"),
						},
					},
					DataSourceRef: &corev1.TypedObjectReference{
						APIGroup: func() *string {
							group := "datamover.a-cup-of.coffee"
							return &group
						}(),
						Kind: "DataMoverPopulator",
						Name: populatorName,
					},
				},
			}
			err = k8sClient.Create(ctx, customPVC)
			Expect(err).NotTo(HaveOccurred())

			By("Simulating PVC bound state for custom test")
			// Update PVC status to simulate it being bound by a storage provisioner
			Eventually(func() error {
				if err := k8sClient.Get(ctx, types.NamespacedName{Name: pvcName, Namespace: namespace}, customPVC); err != nil {
					return err
				}
				customPVC.Status.Phase = corev1.ClaimBound
				return k8sClient.Status().Update(ctx, customPVC)
			}, timeout, interval).Should(Succeed())

			By("Reconciling the custom populator")
			controllerReconciler := &DataMoverPopulatorReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      pvcName,
					Namespace: namespace,
				},
			})
			Expect(err).NotTo(HaveOccurred())

			By("Checking if custom job was created with correct image")
			jobName := fmt.Sprintf("datamover-populator-%s", pvcName)
			foundJob := &batchv1.Job{}
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{
					Name:      jobName,
					Namespace: namespace,
				}, foundJob)
			}, timeout, interval).Should(Succeed())

			By("Verifying custom image configuration")
			Expect(foundJob.Spec.Template.Spec.Containers).To(HaveLen(1))
			container := foundJob.Spec.Template.Spec.Containers[0]
			Expect(container.Image).To(Equal("myregistry.com/custom-rclone:v2.0.0"))
			Expect(container.ImagePullPolicy).To(Equal(corev1.PullNever))

			By("Cleanup custom resources")
			Expect(k8sClient.Delete(ctx, customPVC)).To(Succeed())
			Expect(k8sClient.Delete(ctx, customPopulator)).To(Succeed())
			Expect(k8sClient.Delete(ctx, secret)).To(Succeed())
		})
	})
})
