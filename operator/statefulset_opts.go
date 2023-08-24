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
	zookeeperv1 "github.com/qilitang/zookeeper-operator/api/v1"
	options "github.com/qilitang/zookeeper-operator/common/options"
	"github.com/qilitang/zookeeper-operator/utils"
	apsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/utils/pointer"
)

func WithZookeeperContainer(cluster *zookeeperv1.ZookeeperCluster, zookeeperImageName string, index int) options.StatefulSetResourcesOpts {
	return CreateZookeeper(cluster, CreateZookeeperContainer(cluster, zookeeperImageName, index))
}

func CreateZookeeperContainer(cluster *zookeeperv1.ZookeeperCluster, zookeeperImageName string, index int) *corev1.Container {
	var resource corev1.ResourceRequirements

	if cluster.Spec.ZookeeperResources != nil {
		resource = *cluster.Spec.ZookeeperResources.Resources.DeepCopy()
	}
	c := corev1.Container{
		Env:       NewZookeeperContainerEnv(cluster, index),
		Name:      utils.ZookeeperContainerName,
		Image:     zookeeperImageName,
		Resources: resource,
		Ports: []corev1.ContainerPort{
			{
				Name:          "leader-port",
				ContainerPort: 3888,
				Protocol:      corev1.ProtocolTCP,
			},
			{
				Name:          "client-port",
				ContainerPort: 2181,
				Protocol:      corev1.ProtocolTCP,
			},
			{
				Name:          "quorum-port",
				ContainerPort: 2888,
				Protocol:      corev1.ProtocolTCP,
			},
			{
				Name:          "metrics-port",
				ContainerPort: 7000,
				Protocol:      corev1.ProtocolTCP,
			},
			{
				Name:          "admin-port",
				ContainerPort: 8080,
				Protocol:      corev1.ProtocolTCP,
			},
		},
		ReadinessProbe: &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				Exec: &corev1.ExecAction{
					Command: []string{"/bin/bash", "-c", "echo ruok | timeout 2 nc -w 2 localhost 2181 | grep imok"},
				},
			},
			PeriodSeconds: 5,
		},
		LivenessProbe: &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				Exec: &corev1.ExecAction{
					Command: []string{"/bin/bash", "-c", "echo ruok | timeout 2 nc -w 2 localhost 2181 | grep imok"},
				},
			},
		},
	}
	return &c

}

func WithInitContainer(cluster *zookeeperv1.ZookeeperCluster, zookeeperImage string, index int) options.StatefulSetResourcesOpts {
	return func(set *apsv1.StatefulSet) error {
		container, err := CreateInitContainer(cluster, zookeeperImage, index)
		if err != nil {
			return err
		}
		set.Spec.Template.Spec.InitContainers = append(set.Spec.Template.Spec.InitContainers, *container)
		return nil
	}
}

func CreateInitContainer(cluster *zookeeperv1.ZookeeperCluster, initImageName string, index int) (*corev1.Container, error) {
	env := NewInitContainerEnv(cluster, index)

	initContainer := &corev1.Container{
		Env:             env,
		Name:            "myid",
		Image:           initImageName,
		ImagePullPolicy: corev1.PullAlways,
		Command: []string{
			"/bin/sh",
			"-c",
			fmt.Sprintf(
				"echo $(MY_ID) > /data/myid && echo \"%s\" > /conf/zoo.cfg",
				WithCustomConfig(cluster),
			),
		},
	}
	initContainer.VolumeMounts = []corev1.VolumeMount{
		{
			MountPath: "/data",
			Name:      "data",
		},
		{
			MountPath: "/conf",
			Name:      "conf",
		},
	}

	return initContainer, nil
}

//func WithReplicaVolume(db *v1.MysqlDatabase, selfIdx, maxReplica int, owner metav1.OwnerReference) bricks.StatefulSetResourcesOpts {
//	return func(set *apsv1.StatefulSet) error {
//
//		var mysqlContainer *corev1.Container
//		for idx := range set.Spec.Template.Spec.Containers {
//			if set.Spec.Template.Spec.Containers[idx].Name == "mysql" {
//				mysqlContainer = &set.Spec.Template.Spec.Containers[idx]
//				break
//			}
//		}
//		if mysqlContainer == nil {
//			return fmt.Errorf("get mysql container failed. ")
//		}
//
//		archiveInfo := db.Spec.MountVolumes[apis.ArchivePVCPrefix]
//		if archiveInfo == nil {
//			return fmt.Errorf("get archive info failed: not existed. ")
//		}
//
//		/*
//			原主库名为集群名加0，双主模式后，两个主库分别为00，01
//			新增的共享binlog的卷名为：archivereplica+数据库名+0
//		*/
//		pvcManager := bricks.NewPVCOperatorWithGlobalConfig()
//		var name, mountPath, claimName string
//		for i := 0; i < maxReplica; i++ {
//			if i == selfIdx {
//				continue // 不设置自己的
//			}
//			name = fmt.Sprintf("%sreplica-%d", apis.ArchivePVCPrefix, i)
//			mountPath = fmt.Sprintf("/%sreplica/%s%d/", apis.ArchivePVCPrefix, db.Name, i)
//			claimName = fmt.Sprintf("%s-%s%d-0", apis.ArchivePVCPrefix, db.Name, i) // 其它stateulset的pvc
//			mysqlContainer.VolumeMounts = append(mysqlContainer.VolumeMounts, corev1.VolumeMount{
//				MountPath: mountPath,
//				Name:      name,
//				ReadOnly:  true,
//			})
//			set.Spec.Template.Spec.Volumes = append(set.Spec.Template.Spec.Volumes, corev1.Volume{
//				Name: name,
//				VolumeSource: corev1.VolumeSource{
//					PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
//						ClaimName: claimName,
//						ReadOnly:  true,
//					},
//				},
//			})
//
//			// 确保pvc已经存在
//			err := pvcManager.CreatePVC(set.Namespace, claimName, archiveInfo.StorageClassName, archiveInfo.MountSize,
//				db.Labels, owner, archiveInfo.PvcAccessMode, &volumetypes.MountOption{})
//			if err != nil {
//				return fmt.Errorf("create shared archive pvc failed %v. ", err)
//			}
//
//		}
//
//		return nil
//	}
//}

func CreateZookeeper(cluster *zookeeperv1.ZookeeperCluster, c *corev1.Container) options.StatefulSetResourcesOpts {
	return func(set *apsv1.StatefulSet) error {
		c.VolumeMounts = []corev1.VolumeMount{
			{
				MountPath: "/conf/log4j-quiet.properties",
				Name:      "log4j-quiet",
				SubPath:   "log4j-quiet.properties",
				ReadOnly:  true,
			},
			{
				MountPath: "/conf/log4j.properties",
				Name:      "log4j",
				SubPath:   "log4j.properties",
				ReadOnly:  true,
			},
			{
				MountPath: "/conf/zoo.cfg.dynamic",
				Name:      "zoo-dynamic",
				SubPath:   "zoo.cfg.dynamic",
				ReadOnly:  true,
			},
			{
				MountPath: "/data",
				Name:      "data",
			},
			{
				MountPath: "/conf",
				Name:      "conf",
			},
			{
				MountPath: "/etc/localtime",
				Name:      "localtime",
			},
		}
		zookeeperVolume := []corev1.Volume{
			{
				Name: "log4j-quiet",
				VolumeSource: corev1.VolumeSource{
					ConfigMap: &corev1.ConfigMapVolumeSource{
						LocalObjectReference: corev1.LocalObjectReference{Name: options.GetClusterLog4JQuietConfigName(cluster.Name)},
					},
				},
			},
			{
				Name: "log4j",
				VolumeSource: corev1.VolumeSource{
					ConfigMap: &corev1.ConfigMapVolumeSource{
						LocalObjectReference: corev1.LocalObjectReference{Name: options.GetClusterLog4JConfigName(cluster.Name)},
					},
				},
			},
			{
				Name: "zoo-cfg",
				VolumeSource: corev1.VolumeSource{
					ConfigMap: &corev1.ConfigMapVolumeSource{
						LocalObjectReference: corev1.LocalObjectReference{Name: options.GetClusterCustomConfigName(cluster.Name)},
					},
				},
			},
			{
				Name: "zoo-dynamic",
				VolumeSource: corev1.VolumeSource{
					ConfigMap: &corev1.ConfigMapVolumeSource{
						LocalObjectReference: corev1.LocalObjectReference{Name: options.GetClusterDynamicConfigName(cluster.Name)},
					},
				},
			},
			{
				Name: "data",
				VolumeSource: corev1.VolumeSource{
					EmptyDir: &corev1.EmptyDirVolumeSource{},
				},
			},
			{
				Name: "conf",
				VolumeSource: corev1.VolumeSource{
					EmptyDir: &corev1.EmptyDirVolumeSource{},
				},
			},
			{
				Name: "localtime",
				VolumeSource: corev1.VolumeSource{
					HostPath: &corev1.HostPathVolumeSource{
						Path: "/etc/localtime",
					},
				},
			},
		}

		// add volumes
		//c.VolumeMounts = append(c.VolumeMounts, corev1.VolumeMount{
		//	MountPath: apis.BaseDir,
		//	Name:      "data",
		//	ReadOnly:  false,
		//})
		//pvc := corev1.PersistentVolumeClaim{
		//	ObjectMeta: metav1.ObjectMeta{
		//		Name:      "data",
		//		Namespace: set.Namespace,
		//	},
		//	Spec: corev1.PersistentVolumeClaimSpec{
		//		AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
		//		Resources: corev1.ResourceRequirements{
		//			Requests: corev1.ResourceList{
		//				corev1.ResourceStorage: *resource.NewQuantity(*cluster.Spec.Storage.Size, resource.BinarySI),
		//			},
		//		},
		//		StorageClassName: cluster.Spec.Storage.StorageClass,
		//	},
		//}
		//set.Spec.VolumeClaimTemplates = append(set.Spec.VolumeClaimTemplates, pvc)

		set.Spec.Template.Spec.EnableServiceLinks = pointer.BoolPtr(false)
		set.Spec.Template.Spec.Containers = append(set.Spec.Template.Spec.Containers, *c)
		set.Spec.Template.Spec.Volumes = append(set.Spec.Template.Spec.Volumes, zookeeperVolume...)
		return nil
	}
}

func NewZookeeperContainerEnv(cluster *zookeeperv1.ZookeeperCluster, index int) []corev1.EnvVar {
	env := make([]corev1.EnvVar, 0)
	env = append(env, []corev1.EnvVar{
		{
			Name:  "DOMAIN",
			Value: options.GetHeadlessDomain(cluster),
		},
		{
			Name:  "QUORUM_PORT",
			Value: "2888",
		},
		{
			Name:  "LEADER_PORT",
			Value: "3888",
		},
		{
			Name:  "CLIENT_HOST",
			Value: options.GetClusterHeadlessServiceName(cluster.Name),
		},
		{
			Name:  "CLIENT_PORT",
			Value: "2181",
		},
		{
			Name:  "ADMIN_SERVER_HOST",
			Value: options.GetClusterHeadlessServiceName(cluster.Name),
		},
		{
			Name:  "ADMIN_SERVER_PORT",
			Value: "8080",
		},
		{
			Name:  "CLUSTER_NAME",
			Value: cluster.Name,
		},
		{
			Name:  "CLUSTER_SIZE",
			Value: fmt.Sprint(cluster.Spec.Replicas),
		},
		{
			Name:  "MY_ID",
			Value: fmt.Sprintf("%d", index),
		},
	}...)

	return env
}

func NewInitContainerEnv(cluster *zookeeperv1.ZookeeperCluster, index int) []corev1.EnvVar {
	env := make([]corev1.EnvVar, 0)
	env = append(env, []corev1.EnvVar{
		{
			Name:  "MY_ID",
			Value: fmt.Sprintf("%d", index),
		},
	}...)

	return env
}
