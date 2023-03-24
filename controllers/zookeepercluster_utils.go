package controllers

import (
	"context"
	zookeeperv1 "github.com/qilitang/zookeeper-operator/api/v1"
	options "github.com/qilitang/zookeeper-operator/common/options"
	"github.com/qilitang/zookeeper-operator/common/status"
	"github.com/qilitang/zookeeper-operator/operator"
	"github.com/qilitang/zookeeper-operator/utils"
	v1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func (r *ZookeeperClusterReconciler) syncReplicas(ctx context.Context, cluster *zookeeperv1.ZookeeperCluster) error {
	log := r.Log.WithValues("zookeeper cluster ", "syncReplica")
	setList := &v1.StatefulSetList{}
	matchLabels := utils.CopyMap(operator.NewDatabaseLabel(cluster))
	setOpts := []client.ListOption{
		client.InNamespace(cluster.Namespace),
		client.MatchingLabels(matchLabels),
	}
	if err := r.List(ctx, setList, setOpts...); err != nil {
		log.Error(err, "Failed to list statefulsets",
			"zookeeperCluster.Namespace", cluster.GetNamespace(), "zookeeperCluster.Name", cluster.GetName())
		return err
	}
	setFlag := make(map[string]bool, 0)
	var setReplica int32 = 0
	nums := 0
	if cluster.Spec.Replicas > 0 {
		setReplica = 1
		nums = int(cluster.Spec.Replicas)
	} else {
		// replicas =0 表示要禁用集群，需要修改所有set复本为0
		setReplica = 0
		nums = len(setList.Items)
	}
	start, end := 0, nums-1
	// 创建基础资源
	// sync mysqlcluster sub resource
	shareResource := operator.ClusterSubResources{Cluster: cluster, Client: r.Client, Log: r.Log}

	err := options.SyncShareResources(r.Client, options.ReferenceToOwner(r.OwnerReference, cluster), []options.ResourcesCreator{
		shareResource.CreateHeadlessService,
		shareResource.CreateLog4JQuietConfigMap,
		shareResource.CreateLog4JConfigMap,
		shareResource.CreateCustomConfigMap,
		shareResource.CreateDynamicConfigMap,
	}...)
	if err != nil {
		return err
	}
	cluster.Status.Replicas = int32(end)
	// 按照顺序去创建 sts 资源
	for i := start; nums > 0 && i <= end; i++ {
		opts := make([]options.StatefulSetResourcesOpts, 0)
		serverIndex := i % nums
		setName := options.GetClusterReplicaSetName(cluster.Name, serverIndex)
		// 防止set重复检查
		if setFlag[setName] {
			continue
		}
		setFlag[setName] = true
		set := &v1.StatefulSet{}
		err := r.Get(ctx, types.NamespacedName{Name: setName, Namespace: cluster.Namespace}, set)
		if errors.IsNotFound(err) {
			log.Info("statefulset not found, begin create",
				"StatefulSet.Name", setName, "StatefulSet.Namespace", cluster.Namespace)
			opts = append(opts,
				options.WithStatefulSetCommon(setName, cluster.Namespace, setReplica, operator.NewDatabaseLabel(cluster)),
				operator.WithInitContainer(cluster, cluster.Spec.Image, serverIndex),
				operator.WithZookeeperContainer(cluster, cluster.Spec.Image, serverIndex),
				options.WithShareProcess(),
				options.WithPVCSelector(r.Client),
				//options.WithCustomerAnnotation(cluster.Annotations),
				//options.WithCustomerNodeLabelAnnotation(cluster.Annotations),
				options.WithOwnerReference(options.ReferenceToOwner(r.OwnerReference, cluster)),
			)
			// generate statefulset object
			newSet, err := options.NewStatefulSet(opts...)
			if err != nil {
				return err
			}
			//// append common tolerate
			//options.WithPodToleration(config.GlobalConfigControllerImpl.GetAppConfig().CommonTolerate, &newSet.Spec.Template.Spec)
			//// set imagePullPolicy
			//options.WithImagePullPolicyOfAllContainers(config.GlobalConfigControllerImpl.GetAppConfig().CurrentRuntimeEnv, &newSet.Spec.Template.Spec)

			// sync resource
			err = options.SyncShareResources(r.Client, options.ReferenceToOwner(r.OwnerReference, cluster),
				shareResource.CreateReplicaHeadlessService(newSet))
			if err != nil {
				return err
			}

			log.Info("create statefulset", "StatefulSet.Name", newSet.Name, "StatefulSet.Namespace", newSet.Namespace)
			if err := r.Create(ctx, newSet); err != nil && !errors.IsAlreadyExists(err) {
				return err
			}
			// check pod resource
			set = &v1.StatefulSet{}
			if err := r.Get(ctx, types.NamespacedName{Name: setName, Namespace: cluster.GetNamespace()}, set); err != nil {
				return err
			}
			setStatus := status.StatefulSetResourcesStatus{
				StatefulSet: set,
				Client:      r.Client,
				Log:         log.WithName("StatefulSetResourcesStatus"),
			}
			if !setStatus.IsReady() {
				log.Info("rolling update statefulset, statefulset is not ready, no update next set. ", "Statefulse.Name", set.Name)
				break
			}
			continue // concurrence create
		}
		setStatus := status.StatefulSetResourcesStatus{
			StatefulSet: set,
			Client:      r.Client,
			Log:         log.WithName("StatefulSetResourcesStatus"),
		}
		if !setStatus.IsReady() {
			log.Info("rolling update statefulset, statefulset is not ready, no update next set. ", "Statefulse.Name", set.Name)
			break
		}
		// check update set
		//log.Info("statefulse exists, begin sync", "Statefulse.Name", set.Name, "Statefulse.Namespae", set.Namespace)
		//updateFlag, err := r.DoSyncSetUpdate(set, cluster, apis.DBRoleReplica)
		//if err != nil {
		//	return err
		//}
		//// In order to prevent the pod from being unable to start due to insufficient resources
		//// after restarting with all the changed specifications, we changed to a rolling upgrade
		//if updateFlag {
		//	log.Info("rolling update statefulset success", "Statefulse.Name", set.Name)
		//	return nil
		//}

	}

	return nil
}

func (r *ZookeeperClusterReconciler) UpdateCluster(ctx context.Context, cluster *zookeeperv1.ZookeeperCluster) error {

	newCluster := cluster.DeepCopy()

	zookeeperStatus := operator.ZookeeperClusterResourcesStatus{
		Client:  r.Client,
		Log:     r.Log.WithName("ZookeeperResourcesStatus"),
		Cluster: newCluster,
	}

	err := zookeeperStatus.UpdateStatus()
	if err != nil {
		return err
	}
	if err := r.Status().Update(ctx, newCluster); err != nil {
		return err
	}
	return nil
}

//func (r *ZookeeperClusterReconciler) DoSyncSetUpdate(set *v1.StatefulSet, cluster *zookeeperv1.ZookeeperCluster, dbRole string) (updateFlag bool, err error) {
//	updateFlag = false
//	newSet := set.DeepCopy()
//	replices := cluster.Spec.Replicas
//
//	checker := &operator.ZookeeperChecker{
//		Client: r.Client,
//		Log:    r.Log.WithName("MysqlDatabaseChecker"),
//	}
//
//	err = operator.DoZookeeperUpdate(cluster, newSet, &updateFlag,
//		//checker.CheckVolumeSizeChanged(),
//		//checker.CheckVolumeSizeChangedApplyToPVC(),
//		checker.CheckResourceLimitChanged())
//		//checker.CheckZookeeperCustomConfigChanged())
//		//checker.CheckReplicasChanged(replices),
//		//checker.CheckRollingRestart())
//	if err != nil {
//		return
//	}
//
//	// sync statefulset headless
//	shareResource := operator.ClusterSubResources{Cluster: cluster, Client: r.Client, Log: r.Log}
//	err = options.SyncShareResources(r.Client, options.ReferenceToOwner(r.OwnerReference, cluster),
//		shareResource.CreateReplicaHeadlessService(newSet),
//		shareResource.CreateDatabaseConfigMap(newSet, dbRole, cluster.Spec.Version))
//	if err != nil {
//		return
//	}
//
//	if updateFlag {
//		// update statefulset
//		newSet.Status.ReadyReplicas = 0
//		if err = r.Client.Update(context.TODO(), newSet); err != nil {
//			return
//		}
//	} else {
//		setStatus := status.StatefulSetResourcesStatus{
//			StatefulSet: set,
//			Client:      r.Client,
//			Log:         r.Log.WithName("StatefulSetResourcesStatus"),
//		}
//		if setStatus.IsReady() {
//			r.Log.Info(fmt.Sprintf("set %s is ready, update cluster ready replicas. ", set.Name))
//			if replices > 0 && setStatus.IsForbidden(cluster.Annotations) {
//				cluster.Status.Replicas += 1
//				cluster.Status.ReadyReplicas += 1
//			} else {
//				cluster.Status.Replicas += 1
//				cluster.Status.ReadyReplicas += set.Status.ReadyReplicas
//			}
//		}
//	}
//
//	podName := fmt.Sprintf("%s-0", set.Name)
//	if !updateFlag { // set no update, check pending pod to deleted
//		err = utils.RemovePendingPod(r.Client, r.Log, podName, newSet,
//			utils.IsPodContainerResourceDifferent(r.Log),
//			utils.IsPodInsufficientResources(r.Log),
//			utils.IsPodStatusPhaseExpected(r.Log, corev1.PodPending))
//		if err != nil {
//			return
//		}
//	}
//
//	return
//}
