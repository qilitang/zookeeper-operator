/*
Copyright 2023.

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
	"github.com/go-logr/logr"
	zookeeperv1 "github.com/qilitang/zookeeper-operator/api/v1"
	"github.com/qilitang/zookeeper-operator/operator"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"reflect"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"time"
)

// ZookeeperClusterReconciler reconciles a ZookeeperCluster object
type ZookeeperClusterReconciler struct {
	client.Client
	Scheme         *runtime.Scheme
	RemoteRequest  *operator.RemoteRequest
	Log            logr.Logger
	OwnerReference metav1.OwnerReference
}

//+kubebuilder:rbac:groups=zookeeper.qilitang.top,resources=zookeeperclusters,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=zookeeper.qilitang.top,resources=zookeeperclusters/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=zookeeper.qilitang.top,resources=zookeeperclusters/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the ZookeeperCluster object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.14.1/pkg/reconcile
func (r *ZookeeperClusterReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := r.Log.WithValues("zookeeper cluster", req.NamespacedName)
	cluster := &zookeeperv1.ZookeeperCluster{}
	err := r.Get(ctx, req.NamespacedName, cluster)
	if err != nil {
		if errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			return ctrl.Result{}, nil
		}
		// Error reading the object - requeue the request.
		log.Error(err, "Failed to get zookeeperCluster")
		return ctrl.Result{}, err
	} else if cluster.DeletionTimestamp != nil {
		log.Info(fmt.Sprintf("cluster %s has be deleted, skip", cluster.Name))
		return ctrl.Result{}, nil
	}
	if err := r.syncReplicas(ctx, cluster); err != nil {
		log.Info(fmt.Sprintf("cluster %s sync replica failed, after 30s retry", cluster.Name))
		return ctrl.Result{Requeue: true, RequeueAfter: 30 * time.Second}, err
	}

	if err := r.UpdateCluster(ctx, cluster); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

func (r *ZookeeperClusterReconciler) setUpdatePredicate() predicate.Predicate {
	return predicate.Funcs{
		UpdateFunc: func(event event.UpdateEvent) bool {
			curSet := event.ObjectNew.(*appsv1.StatefulSet)
			oldSet := event.ObjectOld.(*appsv1.StatefulSet)

			if reflect.DeepEqual(curSet.Spec, oldSet.Spec) &&
				reflect.DeepEqual(curSet.Status, oldSet.Status) {
				return false
			}

			return true
		},
	}
}

// SetupWithManager sets up the controller with the Manager.
func (r *ZookeeperClusterReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&zookeeperv1.ZookeeperCluster{}).
		Owns(&appsv1.StatefulSet{}, builder.WithPredicates(r.setUpdatePredicate())).
		Complete(r)
}
