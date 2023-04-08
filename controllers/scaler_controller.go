/*
Copyright 2023 Alex Egorov.

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

package controllers

import (
	"context"
	"fmt"
	apiv1alpha1 "github.com/IAlexEgorov/scaler/api/v1alpha1"
	"github.com/go-logr/logr"
	v1 "k8s.io/api/apps/v1"
	v12 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"time"
)

var logger = log.Log.WithName("controller_scaler")

// ScalerReconciler reconciles a Scaler object
type ScalerReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=api.ialexegorov.neoflex.ru,resources=scalers,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=api.ialexegorov.neoflex.ru,resources=scalers/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=api.ialexegorov.neoflex.ru,resources=scalers/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Scaler object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.13.0/pkg/reconcile
func (r *ScalerReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	operatorLogger := logger
	operatorLogger.Info("Reconcile called")

	scaler := &apiv1alpha1.Scaler{}
	err := r.Get(ctx, req.NamespacedName, scaler)
	if err != nil {
		return ctrl.Result{}, nil
	}

	startTime := scaler.Spec.Start
	endTime := scaler.Spec.End
	replicas := scaler.Spec.Replicas

	currentTime := time.Now().UTC().Hour()

	if currentTime >= startTime && currentTime <= endTime {
		err := scaleDeploments(&operatorLogger, scaler, r, ctx, replicas)
		if err != nil {
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{RequeueAfter: time.Duration(30 * time.Second)}, nil
}

func scaleDeploments(operatorLogger *logr.Logger, scaler *apiv1alpha1.Scaler, r *ScalerReconciler, ctx context.Context, replicas int32) error {
	for _, deploy := range scaler.Spec.Deployments {
		operatorLogger.Info(fmt.Sprintf("Try to get deployment: '%v' ", deploy.Name))
		deployment := &v1.Deployment{}
		err := r.Get(ctx, types.NamespacedName{
			Namespace: deploy.Namespace,
			Name:      deploy.Name,
		}, deployment)
		if err != nil {
			scaler.Status.Status = apiv1alpha1.FAILED
			return err
		}
		operatorLogger.Info(fmt.Sprintf("Deployment: '%v' was received", deployment.Name))

		operatorLogger.Info(fmt.Sprintf("Check replicas for: '%v'", deployment.Name))
		if deployment.Spec.Replicas != &replicas {
			operatorLogger.Info(fmt.Sprintf("There are: %v replicas for '%v', trying to update", *deployment.Spec.Replicas, deployment.Name))
			deployment.Spec.Replicas = &replicas
			err := r.Update(ctx, deployment)
			if err != nil {
				scaler.Status.Status = apiv1alpha1.FAILED
				return err
			}

			scaler.Status.Status = apiv1alpha1.SUCCESS
			err = r.Status().Update(ctx, scaler)
			if err != nil {
				return err
			}

			operatorLogger.Info(fmt.Sprintf("There are: %v replicas for '%v'", *deployment.Spec.Replicas, deployment.Name))
			operatorLogger.Info("_______")
		}
	}

	operatorLogger.Info("Try to create new Test Pod")
	pod := &v12.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "default",
		},
		Spec: v12.PodSpec{
			Containers: []v12.Container{
				{
					Name:  "nginx",
					Image: "nginx",
				},
			},
		},
	}

	err := r.Create(ctx, pod)
	if err != nil {
		operatorLogger.Info("Test Pod already exist")
		return nil
	}

	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ScalerReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&apiv1alpha1.Scaler{}).
		Complete(r)
}
