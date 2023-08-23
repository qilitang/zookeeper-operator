/*


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

package operator

import (
	"fmt"
	"reflect"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	zookeeperv1 "github.com/qilitang/zookeeper-operator/api/v1"
)

type UpdateZookeeperFunction func(cluster *zookeeperv1.ZookeeperCluster, set *appsv1.StatefulSet, updateFlag *bool) error

func DoZookeeperUpdate(cluster *zookeeperv1.ZookeeperCluster, set *appsv1.StatefulSet, updateFlag *bool, funcList ...UpdateZookeeperFunction) error {
	for _, updater := range funcList {
		if err := updater(cluster, set, updateFlag); err != nil {
			return err
		}
	}
	return nil
}

type ZookeeperChecker struct {
	client.Client
	Log logr.Logger
}

//func (c *ZookeeperChecker) CheckVolumeSizeChanged() UpdateZookeeperFunction {
//	return func(cluster *zookeeperv1.ZookeeperCluster, set *appsv1.StatefulSet, updateFlag *bool) error {
//		*updateFlag = false
//		return options.UpdateVolumeSizeChanged(
//			options.ApplyCustomStorage(cluster.Spec.Storage, set.Name, cluster.Spec.CustomResources), c.Client, set)
//	}
//}

//func (c *ZookeeperChecker) CheckVolumeSizeChangedApplyToPVC() UpdateZookeeperFunction {
//	return func(cluster *mysqlv1.MysqlCluster, set *appsv1.StatefulSet, updateFlag *bool) error {
//		*updateFlag = false
//		errs := make([]error, 0)
//
//		// sts 下的每一个 pvc annotation 都更新
//		for i := 0; i < int(*set.Spec.Replicas); i++ {
//			// todo confirm pvcName
//			pvcName := fmt.Sprintf("data-%s-%d", set.Name, i)
//			pvc := &corev1.PersistentVolumeClaim{}
//			err := c.Client.Get(context.Background(), types.NamespacedName{Namespace: set.Namespace, Name: pvcName}, pvc)
//			if err != nil {
//				errs = append(errs, err)
//				continue
//			}
//			annotations := pvc.Annotations
//			if annotations == nil {
//				annotations = make(map[string]string)
//			}
//			newStorage := options.ApplyCustomStorage(cluster.Spec.Storage, set.Name, cluster.Spec.CustomResources)
//			needUpdate := false
//			oldStorageSize, exist := pvc.Annotations["storage"]
//			newStorageSize := fmt.Sprintf("%d", *newStorage.Size)
//			if !exist || oldStorageSize != newStorageSize {
//				needUpdate = true
//				annotations["storage"] = newStorageSize
//			}
//			pvc.Annotations = annotations
//			if needUpdate {
//				err := c.Client.Update(context.Background(), pvc)
//				if err != nil {
//					errs = append(errs, err)
//				}
//			}
//		}
//
//		if len(errs) > 0 {
//			return errs[0]
//		}
//		return nil
//	}
//}

func (c *ZookeeperChecker) CheckResourceLimitChanged() UpdateZookeeperFunction {
	return func(cluster *zookeeperv1.ZookeeperCluster, set *appsv1.StatefulSet, updateFlag *bool) error {
		needUpdate := false
		oldResource := set.Spec.Template.Spec.Containers[0].Resources.DeepCopy()

		//set.Spec.Template.Spec.Containers[0].Resources = *cluster.Spec.Resources.DeepCopy()

		if !reflect.DeepEqual(*oldResource, set.Spec.Template.Spec.Containers[0].Resources) {
			needUpdate = true
		}

		if needUpdate {
			c.Log.Info(fmt.Sprintf(" CheckResourceLimitChanged, update set: %v", set.Spec.Template.Spec.Containers[0].Resources))
			*updateFlag = true
			return nil
		}

		return nil
	}
}

func (c *ZookeeperChecker) CheckZookeeperCustomConfigChanged() UpdateZookeeperFunction {
	return func(cluster *zookeeperv1.ZookeeperCluster, set *appsv1.StatefulSet, updateFlag *bool) error {
		needUpdate := false
		if cluster.Spec.ZookeeperCustomConf.InitLimit == 0 {
			needUpdate = true
			cluster.Spec.ZookeeperCustomConf.InitLimit = 10
		}
		if cluster.Spec.ZookeeperCustomConf.TickTime == 0 {
			needUpdate = true
			cluster.Spec.ZookeeperCustomConf.TickTime = 2000
		}
		if cluster.Spec.ZookeeperCustomConf.SyncLimit == 0 {
			needUpdate = true
			cluster.Spec.ZookeeperCustomConf.SyncLimit = 2
		}
		if cluster.Spec.ZookeeperCustomConf.GlobalOutstandingLimit == 0 {
			needUpdate = true
			cluster.Spec.ZookeeperCustomConf.GlobalOutstandingLimit = 1000
		}
		if cluster.Spec.ZookeeperCustomConf.PreAllocSize == 0 {
			needUpdate = true
			cluster.Spec.ZookeeperCustomConf.PreAllocSize = 65536
		}
		if cluster.Spec.ZookeeperCustomConf.SnapCount == 0 {
			needUpdate = true
			cluster.Spec.ZookeeperCustomConf.SnapCount = 10000
		}
		if cluster.Spec.ZookeeperCustomConf.CommitLogCount == 0 {
			needUpdate = true
			cluster.Spec.ZookeeperCustomConf.CommitLogCount = 500
		}
		if cluster.Spec.ZookeeperCustomConf.SnapSizeLimitInKb == 0 {
			needUpdate = true
			cluster.Spec.ZookeeperCustomConf.SnapSizeLimitInKb = 4194304
		}
		if cluster.Spec.ZookeeperCustomConf.MaxClientCnxns == 0 {
			needUpdate = true
			cluster.Spec.ZookeeperCustomConf.MaxClientCnxns = 60
		}
		if cluster.Spec.ZookeeperCustomConf.MinSessionTimeout == 0 {
			needUpdate = true
			cluster.Spec.ZookeeperCustomConf.MinSessionTimeout = 2 * cluster.Spec.ZookeeperCustomConf.TickTime
		}
		if cluster.Spec.ZookeeperCustomConf.MaxSessionTimeout == 0 {
			needUpdate = true
			cluster.Spec.ZookeeperCustomConf.MaxSessionTimeout = 20 * cluster.Spec.ZookeeperCustomConf.TickTime
		}
		if cluster.Spec.ZookeeperCustomConf.AutoPurgeSnapRetainCount == 0 {
			needUpdate = true
			cluster.Spec.ZookeeperCustomConf.AutoPurgeSnapRetainCount = 3
		}
		if cluster.Spec.ZookeeperCustomConf.AutoPurgePurgeInterval == 0 {
			needUpdate = true
			cluster.Spec.ZookeeperCustomConf.AutoPurgePurgeInterval = 1
		}
		if needUpdate {
			c.Log.Info(fmt.Sprintf("CheckZookeeperCustomConfigChanged, update set: %v", set.Spec.Template.Spec.Containers[0].Resources))
			*updateFlag = true
		}
		return nil
	}
}

//func (c *MysqlDatabaseChecker) CheckResourceLimitChanged() UpdateMysqlDatabaseFunction {
//	return func(cluster *mysqlv1.MysqlCluster, set *appsv1.StatefulSet, updateFlag *bool) error {
//		needUpdate := false
//		hasConfig, hasChange := options.ApplyCustomerSetResources(set.Name, &set.Spec.Template.Spec.Containers[0], cluster.Annotations)
//		if hasChange {
//			needUpdate = true
//		}
//		if !hasConfig {
//			if set.Spec.Template.Spec.Containers[0].Resources.Requests.Cpu().Cmp(*cluster.Spec.Resources.Requests.Cpu()) != 0 ||
//				set.Spec.Template.Spec.Containers[0].Resources.Requests.Memory().Cmp(*cluster.Spec.Resources.Requests.Memory()) != 0 {
//				set.Spec.Template.Spec.Containers[0].Resources.Requests = cluster.Spec.Resources.Requests
//				needUpdate = true
//			}
//			if set.Spec.Template.Spec.Containers[0].Resources.Limits.Cpu().Cmp(*cluster.Spec.Resources.Limits.Cpu()) != 0 ||
//				set.Spec.Template.Spec.Containers[0].Resources.Limits.Memory().Cmp(*cluster.Spec.Resources.Limits.Memory()) != 0 {
//				set.Spec.Template.Spec.Containers[0].Resources.Limits = cluster.Spec.Resources.Limits
//				needUpdate = true
//			}
//		}
//
//		if needUpdate {
//			c.Log.Info(fmt.Sprintf(" CheckVolumeSizeChanged, update set: %v", set.Spec.Template.Spec.Containers[0].Resources))
//			*updateFlag = true
//			return nil
//		}
//
//		return nil
//	}
//}

//func (c *ZookeeperChecker) CheckReplicasChanged(replicas int32) UpdateZookeeperFunction {
//	return func(cluster *zookeeperv1.ZookeeperCluster, set *appsv1.StatefulSet, updateFlag *bool) error {
//		var replica int32 = 0
//		if replicas > 0 {
//			replica = 1
//		} else {
//			replica = 0
//		}
//
//		// check annotation
//		if cluster.Annotations != nil && cluster.Annotations[set.Name+apis.StsRunStatus] == apis.StsRunForbid {
//			c.Log.Info(fmt.Sprintf("sts %s is frobid, turn to 0. ", set.Name))
//			replica = 0
//		}
//
//		if replica != *set.Spec.Replicas {
//			set.Spec.Replicas = &replica
//			c.Log.Info(fmt.Sprintf(" CheckReplicasChanged, update set %s replica to: %d", set.Name, set.Spec.Replicas))
//			*updateFlag = true
//			return nil
//		}
//		return nil
//	}
//}

//func (c *ZookeeperChecker) CheckRollingRestart() UpdateZookeeperFunction {
//	return func(cluster *zookeeperv1.ZookeeperCluster, set *appsv1.StatefulSet, updateFlag *bool) error {
//		if cluster.Annotations == nil {
//			return nil
//		}
//		dbNumStr, ok := cluster.Annotations[apis.ClusterRestart]
//		if !ok {
//			return nil
//		}
//
//		if set.Annotations == nil {
//			set.Annotations = make(map[string]string)
//		}
//
//		if dbNumStr != set.Annotations[apis.ClusterRestart] {
//			*updateFlag = true
//			*set.Spec.Replicas = 0
//			c.Log.Info(fmt.Sprintf("set %s get %v, do Rolling Restart, set sts to 0. ", set.Name, set.Annotations[apis.ClusterRestart]))
//
//			set.Annotations[apis.ClusterRestart] = cluster.Annotations[apis.ClusterRestart]
//		}
//		return nil
//	}
//}

// TODO: recreate pod
