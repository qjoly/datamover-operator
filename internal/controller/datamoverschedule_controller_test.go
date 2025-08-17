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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"

	datamoverv1alpha1 "a-cup-of.coffee/datamover-operator/api/v1alpha1"
)

var _ = Describe("DataMoverSchedule Controller", func() {
	Context("When reconciling a resource", func() {
		const (
			DataMoverScheduleName      = "test-datamoverschedule"
			DataMoverScheduleNamespace = "default"
			timeout                    = time.Second * 10
			duration                   = time.Second * 10
			interval                   = time.Millisecond * 250
		)

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name:      DataMoverScheduleName,
			Namespace: DataMoverScheduleNamespace,
		}

		datamoverschedule := &datamoverv1alpha1.DataMoverSchedule{}

		BeforeEach(func() {
			By("creating the custom resource for the Kind DataMoverSchedule")
			err := k8sClient.Get(ctx, typeNamespacedName, datamoverschedule)
			if err != nil && errors.IsNotFound(err) {
				resource := &datamoverv1alpha1.DataMoverSchedule{
					ObjectMeta: metav1.ObjectMeta{
						Name:      DataMoverScheduleName,
						Namespace: DataMoverScheduleNamespace,
					},
					Spec: datamoverv1alpha1.DataMoverScheduleSpec{
						Schedule:   "*/5 * * * *", // Every 5 minutes
						SourcePvc:  "test-pvc",
						SecretName: "test-secret",
					},
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}
		})

		AfterEach(func() {
			By("Cleanup the specific resource instance DataMoverSchedule")
			resource := &datamoverv1alpha1.DataMoverSchedule{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			Expect(err).NotTo(HaveOccurred())

			By("Cleanup the specific resource instance DataMoverSchedule")
			Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
		})

		It("should successfully reconcile the resource", func() {
			By("Reconciling the created resource")
			controllerReconciler := &DataMoverScheduleReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, ctrl.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			By("Checking if the custom resource status has been updated")
			Eventually(func() error {
				found := &datamoverv1alpha1.DataMoverSchedule{}
				return k8sClient.Get(ctx, typeNamespacedName, found)
			}, timeout, interval).Should(Succeed())
		})
	})
})
