package controllers

import (
	"context"
	"fmt"
	zookeeperv1 "github.com/qilitang/zookeeper-operator/api/v1"
	"github.com/qilitang/zookeeper-operator/common/options"
	"github.com/qilitang/zookeeper-operator/common/status"
	"github.com/qilitang/zookeeper-operator/operator"
	"github.com/qilitang/zookeeper-operator/utils"
	"golang.org/x/sync/errgroup"
	v1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"strings"
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
		err = r.Get(ctx, types.NamespacedName{Name: setName, Namespace: cluster.Namespace}, set)
		if errors.IsNotFound(err) {
			err = options.SyncShareResources(r.Client, options.ReferenceToOwner(r.OwnerReference, cluster),
				shareResource.CreateReplicaHeadlessService(setName))
			if err != nil {
				return err
			}
			// sync resource
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
		if err := r.reConfigCluster(ctx, cluster, set, serverIndex); err != nil {
			return err
		}

	}
	// 释放资源
	if err := r.deleteReplicasResources(ctx, cluster); err != nil {
		return err
	}
	return nil
}

func (r *ZookeeperClusterReconciler) UpdateCluster(ctx context.Context, cluster *zookeeperv1.ZookeeperCluster) error {
	newCluster := &zookeeperv1.ZookeeperCluster{}
	_ = r.Get(ctx, client.ObjectKey{
		Namespace: cluster.Namespace,
		Name:      cluster.Name,
	}, newCluster)
	newCluster = newCluster.DeepCopy()
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

func (r *ZookeeperClusterReconciler) deleteReplicasResources(ctx context.Context, cluster *zookeeperv1.ZookeeperCluster) error {
	log := r.Log.WithValues("zookeeper cluster", "delete replica")
	g, _ := errgroup.WithContext(ctx)
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
	start, end := int(cluster.Spec.Replicas), len(setList.Items)+1
	for i := start; end > 0 && i < end; i++ {
		setName := options.GetClusterReplicaSetName(cluster.Name, i)
		g.Go(func() error {
			set := &v1.StatefulSet{}
			err := r.Get(ctx, types.NamespacedName{Name: setName, Namespace: cluster.Namespace}, set)
			if errors.IsNotFound(err) {
				return nil
			}
			if err != nil {
				return err
			}
			log.Info(fmt.Sprintf("will delete replica %s", set.Name))
			return r.Client.Delete(ctx, set)
		})
		g.Go(func() error {
			svcName := setName + "-headless"
			svc := &corev1.Service{}
			err := r.Get(ctx, types.NamespacedName{Name: svcName, Namespace: cluster.Namespace}, svc)
			if errors.IsNotFound(err) {
				return nil
			}
			if err != nil {
				return err
			}
			return r.Delete(ctx, svc)
		})
		g.Go(func() error {
			removeCmd := []string{
				"sh",
				"-c",
				fmt.Sprintf("zkCli.sh reconfig -remove %d", i),
			}
			_, err := r.Exec(ctx, cluster, removeCmd)
			if err != nil {
				return err
			}
			return nil
		})
		if err := g.Wait(); err != nil {
			log.Info("remove resources is failed: %v", err)
			return err
		}
		continue
	}
	return nil
}

func (r *ZookeeperClusterReconciler) Exec(ctx context.Context, cluster *zookeeperv1.ZookeeperCluster, cmd []string) (string, error) {
	firstPodName := options.GetClusterReplicaSetName(cluster.Name, 0) + "-0"
	stdout, stderr, err := r.RemoteRequest.Exec(ctx, cluster.Namespace, firstPodName, "zookeeper", cmd)
	if err != nil {
		return "", fmt.Errorf("exec cmd %v failed, %v", cmd, err)
	}
	if stderr != "" {
		return "", fmt.Errorf("exec addCmd %v Failed: %s", cmd, stderr)
	}
	return stdout, nil
}

func (r *ZookeeperClusterReconciler) reConfigCluster(ctx context.Context, cluster *zookeeperv1.ZookeeperCluster, set *v1.StatefulSet, id int) error {
	if id == 0 {
		return nil
	}
	cmd := []string{
		"sh",
		"-c",
		"echo \"conf\" | nc localhost 2181 | grep \"server\\.\"",
	}
	stdout, err := r.Exec(ctx, cluster, cmd)
	if err != nil {
		return err
	}
	oldAllServersHosts := make(map[string]bool, 0)
	for _, server := range strings.Split(stdout, "\n") {
		if strings.Contains(server, "server.") {
			oldAllServersHosts[server] = true
		}
	}
	var allServersHosts []string
	for _, line := range strings.Split(operator.WithDynamicConfig1(cluster, id), "\n") {
		if strings.Contains(line, "server.") {
			allServersHosts = append(allServersHosts, line)
		}
	}
	for _, server := range allServersHosts {
		if !oldAllServersHosts[server] {
			addCmd := []string{
				"sh",
				"-c",
				fmt.Sprintf("zkCli.sh reconfig -add server.%d=zk1-replica%d-headless.default.svc.cluster.local:2888:3888:participant\\;2181", id, id),
			}
			_, err = r.Exec(ctx, cluster, addCmd)
			if err != nil {
				return err
			}
		}
	}
	return nil
}
