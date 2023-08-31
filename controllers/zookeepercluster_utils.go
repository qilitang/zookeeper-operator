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
	"sigs.k8s.io/controller-runtime/pkg/log"
	"strings"
	"time"
)

func (r *ZookeeperClusterReconciler) syncReplicas(ctx context.Context, cluster *zookeeperv1.ZookeeperCluster) error {
	log := r.Log.WithValues("zookeeper cluster ", "syncReplica")
	setList := &v1.StatefulSetList{}
	matchLabels := utils.CopyMap(operator.NewClusterLabel(cluster))
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
	// 创建基础资源
	// sync zookeeperCluster sub resource
	shareResource := operator.ClusterSubResources{Cluster: cluster, Client: r.Client, Log: r.Log}

	err := options.SyncShareResources(r.Client, options.ReferenceToOwner(r.OwnerReference, cluster), []options.ResourcesCreator{
		shareResource.CreateHeadlessService,
		shareResource.CreateLog4JQuietConfigMap,
		shareResource.CreateLog4JConfigMap,
		shareResource.CreateCustomConfigMap,
		shareResource.CreateScriptConfigMap,
	}...)
	if err != nil {
		return err
	}
	// 按照顺序去创建 sts 资源
	for serverIndex := 0; serverIndex < nums; serverIndex++ {
		opts := make([]options.StatefulSetResourcesOpts, 0)
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
				shareResource.CreateReplicaHeadlessService(setName),
				shareResource.CreateDynamicConfigMap(ctx, serverIndex))
			if err != nil {
				return err
			}
			// sync resource
			log.Info("statefulset not found, begin create",
				"StatefulSet.Name", setName, "StatefulSet.Namespace", cluster.Namespace)
			opts = append(opts,
				options.WithStatefulSetCommon(setName, cluster.Namespace, setReplica, operator.NewClusterLabel(cluster), operator.NewClusterAnnotations(cluster)),
				operator.WithInitContainer(cluster, cluster.Spec.Image, serverIndex),
				operator.WithZookeeperContainer(cluster, cluster.Spec.Image, serverIndex),
				options.WithShareProcess(),
				options.WithPVCSelector(r.Client),
				options.WithStatefulSetPvc(cluster),
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
			if err := r.pollStatefulSet(ctx, set); err != nil {
				return err
			}
			if err := r.reConfigCluster(ctx, cluster, serverIndex); err != nil {
				return err
			}
			continue
		}
		updateFlag, err := r.DoSyncSetUpdate(ctx, set, cluster)
		if err != nil {
			return err
		}
		if updateFlag {
			log.Info("success update statefulset", "Statefulse.Name", set.Name)
			break
		}
		if err := r.pollStatefulSet(ctx, set); err != nil {
			return err
		}
		if err := r.reConfigCluster(ctx, cluster, serverIndex); err != nil {
			return err
		}
	}
	// 释放资源
	if err := r.deleteReplicasResources(ctx, cluster); err != nil {
		return err
	}
	// 保证每个节点都在集群中
	// 1. 可执行 echo stat | nc ocalhost 2181
	// 2. 节点都含有 Mode 字段
	//if err := r.checkAllNodeRole(ctx, cluster); err != nil {
	//	return err
	//}
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
		RemoteRequest: r.RemoteRequest,
		Client:        r.Client,
		Log:           r.Log.WithName("ZookeeperResourcesStatus"),
		Cluster:       newCluster,
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

func (r *ZookeeperClusterReconciler) DoSyncSetUpdate(ctx context.Context, set *v1.StatefulSet, cluster *zookeeperv1.ZookeeperCluster) (updateFlag bool, err error) {
	updateFlag = false
	newSet := set.DeepCopy()
	//replicas := cluster.Spec.Replicas
	checker := &operator.ZookeeperChecker{
		Client: r.Client,
		Log:    r.Log.WithName("ZookeeperChecker"),
	}

	err = operator.DoZookeeperUpdate(cluster, newSet, &updateFlag,
		checker.CheckVolumeSizeChangedApplyToPVC(),
		checker.CheckResourceLimitChanged())
	//checker.CheckReplicasChanged(replices),
	//checker.CheckRollingRestart())
	if err != nil {
		return
	}
	if updateFlag {
		newSet.Status.ReadyReplicas = 0
		newSet.Status.Replicas = 0
		annotations := utils.CopyMap(newSet.Annotations)
		annotations[utils.AnnotationsRoleKey] = utils.AnnotationsRoleNotReady
		newSet.Annotations = annotations
		newSet.Spec.Template.Annotations = annotations
		if err = r.Client.Update(ctx, newSet); err != nil {
			return
		}
	}
	//else {
	//	setStatus := status.StatefulSetResourcesStatus{
	//		StatefulSet: set,
	//		Client:      r.Client,
	//		Log:         r.Log.WithName("StatefulSetResourcesStatus"),
	//	}
	//	if setStatus.IsReady() {
	//		r.Log.Info(fmt.Sprintf("set %s is ready, update cluster ready replicas. ", set.Name))
	//		if replicas > 0 {
	//			cluster.Status.Replicas += 1
	//			cluster.Status.ReadyReplicas += set.Status.ReadyReplicas
	//		}
	//	}
	//}
	return
}

func (r *ZookeeperClusterReconciler) deleteReplicasResources(ctx context.Context, cluster *zookeeperv1.ZookeeperCluster) error {
	log := r.Log.WithValues("zookeeper cluster", "delete replica")
	g, _ := errgroup.WithContext(ctx)
	setList := &v1.StatefulSetList{}
	matchLabels := utils.CopyMap(operator.NewClusterLabel(cluster))
	setOpts := []client.ListOption{
		client.InNamespace(cluster.Namespace),
		client.MatchingLabels(matchLabels),
	}
	if err := r.List(ctx, setList, setOpts...); err != nil {
		log.Error(err, "Failed to list statefulsets",
			"zookeeperCluster.Namespace", cluster.GetNamespace(), "zookeeperCluster.Name", cluster.GetName())
		return err
	}
	start, end := int(cluster.Spec.Replicas), len(setList.Items)
	if start == end {
		return nil
	}
	for i := start; end > 0 && i < end; i++ {
		setName := options.GetClusterReplicaSetName(cluster.Name, i)
		// 先执行清除  后执行 删除 sts 操作
		removeCmd := []string{
			"sh",
			"-c",
			fmt.Sprintf("zkCli.sh reconfig -remove %d", i),
		}
		_, err := r.Exec(ctx, "", cluster, removeCmd)
		if err != nil {
			return err
		}
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
		if err := g.Wait(); err != nil {
			log.Info("remove resources is failed: %v", err)
			return err
		}
		continue
	}
	return nil
}

func (r *ZookeeperClusterReconciler) Exec(ctx context.Context, podName string, cluster *zookeeperv1.ZookeeperCluster, cmd []string) (string, error) {
	firstPodName := options.GetClusterReplicaSetName(cluster.Name, 0) + "-0"
	if podName == "" {
		podName = firstPodName
	}
	stdout, stderr, err := r.RemoteRequest.Exec(ctx, cluster.Namespace, podName, utils.ZookeeperContainerName, cmd)
	if err != nil {
		return "", fmt.Errorf("exec cmd %v failed, %v", cmd, err)
	}
	if stderr != "" {
		return "", fmt.Errorf("exec cmd %v Failed: %s", cmd, stderr)
	}
	return stdout, nil
}

func (r *ZookeeperClusterReconciler) FindLeaderPodName(ctx context.Context, cluster *zookeeperv1.ZookeeperCluster) (string, error) {
	log := r.Log.WithValues("zookeeper cluster ", "find leader pod")
	podList := &corev1.PodList{}
	matchLabels := utils.CopyMap(operator.NewClusterLabel(cluster))
	setOpts := []client.ListOption{
		client.InNamespace(cluster.Namespace),
		client.MatchingLabels(matchLabels),
	}
	if err := r.List(ctx, podList, setOpts...); err != nil {
		log.Error(err, "Failed to list statefulsets",
			"zookeeperCluster.Namespace", cluster.GetNamespace(), "zookeeperCluster.Name", cluster.GetName())
		return "", err
	}
	cmd := []string{
		"sh",
		"-c",
		"echo stat | nc localhost 2181",
	}
	for _, pod := range podList.Items {
		stdout, err := r.Exec(ctx, pod.Name, cluster, cmd)
		if err != nil {
			continue
		}
		if strings.Contains(stdout, "Mode: leader") {
			return pod.Name, nil
		}
	}
	return "", fmt.Errorf("can not find leader pod")
}

func (r *ZookeeperClusterReconciler) reConfigCluster(ctx context.Context, cluster *zookeeperv1.ZookeeperCluster, serverIndex int) error {
	if serverIndex == 0 {
		return nil
	}
	// findLeader pod
	leaderPodName, err := r.FindLeaderPodName(ctx, cluster)
	if err != nil {
		return err
	}
	cmd := []string{
		"sh",
		"-c",
		"echo \"conf\" | nc localhost 2181 | grep \"server\\.\"",
	}
	stdout, err := r.Exec(ctx, leaderPodName, cluster, cmd)
	if err != nil {
		return err
	}
	oldAllServersHosts := make(map[string]bool, 0)
	for _, server := range strings.Split(stdout, "\n") {
		if strings.Contains(server, "server.") {
			oldAllServersHosts[strings.Split(server, ";")[0]] = true
		}
	}
	var allServersHosts []string
	for _, line := range strings.Split(operator.WithDynamicConfig(cluster, serverIndex), "\n") {
		if strings.Contains(line, "server.") {
			allServersHosts = append(allServersHosts, strings.Split(line, ";")[0])
		}
	}
	for _, server := range allServersHosts {
		if !oldAllServersHosts[server] {
			addCmd := []string{
				"sh",
				"-c",
				fmt.Sprintf("zkCli.sh reconfig -add %s",
					options.GetServerDomain(
						options.GetClusterReplicaSetName(cluster.Name, serverIndex),
						cluster.Namespace, serverIndex, true)),
			}
			_, err = r.Exec(ctx, leaderPodName, cluster, addCmd)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func (r *ZookeeperClusterReconciler) checkReplicaRole(ctx context.Context, cluster *zookeeperv1.ZookeeperCluster, set *v1.StatefulSet) error {
	cmd := []string{
		"sh",
		"-c",
		"echo stat | nc localhost 2181",
	}
	stdout, err := r.Exec(ctx, set.Name+"-0", cluster, cmd)
	if err != nil {
		return fmt.Errorf("exec cmd %v failed: %v", cmd, err)
	}
	newSet := set.DeepCopy()
	annotations := newSet.Annotations
	needUpdate := false
	if strings.Contains(stdout, "Mode: leader") {
		if annotations[utils.AnnotationsRoleKey] != utils.AnnotationsRoleLeader {
			annotations[utils.AnnotationsRoleKey] = utils.AnnotationsRoleLeader
			needUpdate = true
		}
	} else if strings.Contains(stdout, "Mode: follower") {
		if annotations[utils.AnnotationsRoleKey] != utils.AnnotationsRoleFollower {
			annotations[utils.AnnotationsRoleKey] = utils.AnnotationsRoleFollower
			needUpdate = true
		}
	} else {
		return fmt.Errorf("exec cmd %v in pod %s stdout is: [%s] ", cmd, set.Name+"-0", stdout)
	}
	if needUpdate {
		newSet.Annotations = annotations
		return r.Client.Update(ctx, newSet)
	}
	return nil
}

func (r *ZookeeperClusterReconciler) checkAllNodeRole(ctx context.Context, cluster *zookeeperv1.ZookeeperCluster) error {
	log := r.Log.WithValues("zookeeper cluster ", "find leader pod")
	setList := &v1.StatefulSetList{}
	matchLabels := utils.CopyMap(operator.NewClusterLabel(cluster))
	setOpts := []client.ListOption{
		client.InNamespace(cluster.Namespace),
		client.MatchingLabels(matchLabels),
	}
	if err := r.List(ctx, setList, setOpts...); err != nil {
		log.Error(err, "Failed to list pods",
			"zookeeperCluster.Namespace", cluster.GetNamespace(), "zookeeperCluster.Name", cluster.GetName())
		return err
	}
	for _, set := range setList.Items {
		if err := r.checkReplicaRole(ctx, cluster, &set); err != nil {
			return err
		}
	}
	return nil
}

func (r *ZookeeperClusterReconciler) pollStatefulSet(ctx context.Context, set *v1.StatefulSet) error {
	log := log.Log.WithValues("zookeeper Cluster", "pollStatefulSet")
	if utils.IsContextDone(ctx) {
		log.V(2).Info("ctx is done")
		return nil
	}
	opts := NewStatefulSetPollOptions()
	start := time.Now()
	for {
		if utils.IsContextDone(ctx) {
			log.V(2).Info("ctx is done")
			return nil
		}
		newSet := &v1.StatefulSet{}
		err := r.Client.Get(ctx, client.ObjectKey{Name: set.Name, Namespace: set.Namespace}, newSet)
		if err == nil {
			setStatus := status.StatefulSetResourcesStatus{
				RemoteRequest: r.RemoteRequest,
				StatefulSet:   newSet,
				Client:        r.Client,
				Log:           log.WithName("StatefulSetResourcesStatus"),
			}
			if setStatus.IsReady() {
				if setStatus.IsRoleReady() {
					return nil
				}
			}
			log.Info("rolling update statefulSet, is not ready, no update next set. ", "StatefulSet.Name", set.Name)
		} else {
			log.Info(fmt.Sprintf("statefulSet %s is not create yet ?", set.Name))
			return nil
		}
		// StatefulSet is either not created or generation is not yet reached
		if time.Since(start) >= opts.Timeout {
			// Timeout reached, no good result available, time to quit
			log.V(1).Info("%s/%s - TIMEOUT reached")
			return fmt.Errorf("waitStatefulSet(%s/%s) - wait timeout", set.Namespace, set.Name)
		}
		pollBack(ctx, opts)
	}
}

func pollBack(ctx context.Context, opts *StatefulSetPollOptions) {
	if ctx == nil {
		ctx = context.Background()
	}
	mainIntervalTimeout := time.After(opts.MainInterval)
	run := true
	for run {
		backgroundIntervalTimeout := time.After(opts.BackgroundInterval)
		select {
		case <-ctx.Done():
			// Context is done, nothing to do here more
			run = false
		case <-mainIntervalTimeout:
			// Timeout reached, nothing to do here more
			run = false
		case <-backgroundIntervalTimeout:
			// Function interval reached, time to call the func
		}
	}
}

// StatefulSetPollOptions specifies polling options
type StatefulSetPollOptions struct {
	StartBotheringAfterTimeout time.Duration
	CreateTimeout              time.Duration
	Timeout                    time.Duration
	MainInterval               time.Duration
	BackgroundInterval         time.Duration
}

// NewStatefulSetPollOptions creates new poll options
func NewStatefulSetPollOptions() *StatefulSetPollOptions {
	return &StatefulSetPollOptions{
		StartBotheringAfterTimeout: time.Duration(60) * time.Second,
		CreateTimeout:              time.Duration(30) * time.Second,
		Timeout:                    time.Duration(300000) * time.Second,
		MainInterval:               time.Duration(5) * time.Second,
		BackgroundInterval:         2 * time.Second,
	}
}
