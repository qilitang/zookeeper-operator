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
	"context"
	"encoding/json"
	"fmt"
	"github.com/go-logr/logr"
	"github.com/qilitang/zookeeper-operator/utils"
	"k8s.io/apimachinery/pkg/types"
	"reflect"
	"sigs.k8s.io/controller-runtime/pkg/client"

	zookeeperv1 "github.com/qilitang/zookeeper-operator/api/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
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

func (c *ZookeeperChecker) CheckVolumeSizeChangedApplyToPVC() UpdateZookeeperFunction {
	return func(cluster *zookeeperv1.ZookeeperCluster, set *appsv1.StatefulSet, updateFlag *bool) error {
		*updateFlag = false
		errs := make([]error, 0)

		// sts 下的每一个 pvc annotation 都更新
		for i := 0; i < int(*set.Spec.Replicas); i++ {
			// todo confirm pvcName
			pvcName := fmt.Sprintf("data-%s-%d", set.Name, i)
			pvc := &corev1.PersistentVolumeClaim{}
			err := c.Client.Get(context.Background(), types.NamespacedName{Namespace: set.Namespace, Name: pvcName}, pvc)
			if err != nil {
				errs = append(errs, err)
				continue
			}
			needUpdate := false
			oldStorageSize := pvc.Spec.Resources.Requests.Storage()
			newStorageSize := &cluster.Spec.ZookeeperResources.Storage.Size
			if !reflect.DeepEqual(oldStorageSize, newStorageSize) {
				needUpdate = true
			}
			if needUpdate {
				newPvc := pvc.DeepCopy()
				newPvc.Spec.Resources.Requests = corev1.ResourceList{
					corev1.ResourceStorage: cluster.Spec.ZookeeperResources.Storage.Size,
				}
				err := c.Client.Update(context.Background(), newPvc)
				if err != nil {
					errs = append(errs, err)
				}
			}
		}

		if len(errs) > 0 {
			return errs[0]
		}
		return nil
	}
}

func (c *ZookeeperChecker) CheckResourceLimitChanged() UpdateZookeeperFunction {
	return func(cluster *zookeeperv1.ZookeeperCluster, set *appsv1.StatefulSet, updateFlag *bool) error {
		needUpdate := false
		oldResource := &corev1.ResourceRequirements{}
		for _, container := range set.Spec.Template.Spec.Containers {
			if container.Name == utils.ZookeeperContainerName {
				oldResource = container.Resources.DeepCopy()
			}
		}
		newResource := cluster.Spec.ZookeeperResources.Resources.DeepCopy()
		if !reflect.DeepEqual(oldResource, newResource) {
			needUpdate = true
		}

		if needUpdate {
			containers := make([]corev1.Container, 0)
			for _, container := range set.Spec.Template.Spec.Containers {
				if container.Name == utils.ZookeeperContainerName {
					container.Resources = *newResource
					// 更改 env 设置
					for _, env := range container.Env {
						if env.Name == utils.ZookeeperJVMFLAGSEnvName {
							env.Value = fmt.Sprintf("-Xms%dm", utils.ChangeBToMBWithJVMRatio(newResource.Limits.Memory().Value()))
							continue
						}
						if env.Name == utils.ZookeeperHeapEnvName {
							env.Value = fmt.Sprintf("%d", utils.ChangeBToMBWithJVMRatio(newResource.Limits.Memory().Value()))
						}
					}
					container.Env = append(container.Env, []corev1.EnvVar{}...)
					containers = append(containers, container)
				} else {
					containers = append(containers, container)
				}
			}

			set.Spec.Template.Spec.Containers = containers
			oldData, _ := json.Marshal(oldResource)
			newData, _ := json.Marshal(newResource)
			c.Log.Info(fmt.Sprintf("CheckResourceLimitChanged, update set %s: %v -> %v", set.Name, string(oldData), string(newData)))
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
