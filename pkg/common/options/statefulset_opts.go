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

package options

import (
	"context"
	"fmt"
	zookeeperv1 "github.com/qilitang/zookeeper-operator/api/v1"
	utils2 "github.com/qilitang/zookeeper-operator/pkg/utils"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"strings"
)

/*
statefulSet创建闭包函数
*/
type StatefulSetResourcesOpts func(set *appsv1.StatefulSet) error

var (
	terminationGracePeriodSeconds int64 = 0
	fluentdImage                        = "registry.woqutech.com/google_containers/fluent-bit:0.12.9"
)

const (
	calcPhyMemoryRatio  = 0.65 // no need to change
	calcForBlockAlign   = 8
	calcForPoolInstance = 4 * 1024 * 1024 * 1024
)

func NewStatefulSet(opts ...StatefulSetResourcesOpts) (*appsv1.StatefulSet, error) {

	set := appsv1.StatefulSet{}

	for _, opt := range opts {
		err := opt(&set)
		if err != nil {
			return nil, err
		}
	}

	return &set, nil
}

func UpdateStatefulSet(set *appsv1.StatefulSet, opts ...StatefulSetResourcesOpts) error {

	for _, opt := range opts {
		err := opt(set)
		if err != nil {
			return err
		}
	}

	return nil
}

func WithParallel() StatefulSetResourcesOpts {
	return func(set *appsv1.StatefulSet) error {
		set.Spec.PodManagementPolicy = appsv1.ParallelPodManagement
		return nil
	}
}

/*
创建statefulSet所需要的基本spec配置信息
*/
func WithStatefulSetCommon(name, nameSpace string, replica int32, labels, annotations map[string]string) StatefulSetResourcesOpts {
	return func(set *appsv1.StatefulSet) error {

		setLabels := utils2.CopyMap(labels)
		setLabels[utils2.SetName] = name
		// pod 上区分出主备从
		podLabels := utils2.CopyMap(setLabels)

		*set = appsv1.StatefulSet{
			TypeMeta: metav1.TypeMeta{
				Kind:       "StatefulSet",
				APIVersion: "apps/v1",
			},
			ObjectMeta: metav1.ObjectMeta{
				Namespace:   nameSpace,
				Name:        name,
				Labels:      setLabels,
				Annotations: annotations,
			},
			Spec: appsv1.StatefulSetSpec{
				Replicas: &replica,
				UpdateStrategy: appsv1.StatefulSetUpdateStrategy{
					Type: appsv1.RollingUpdateStatefulSetStrategyType,
				},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Name:   name,
						Labels: podLabels,
					},
					Spec: corev1.PodSpec{
						SecurityContext: &corev1.PodSecurityContext{
							Sysctls: []corev1.Sysctl{},
						},
						TerminationGracePeriodSeconds: &terminationGracePeriodSeconds,
					},
				},
				Selector:             &metav1.LabelSelector{MatchLabels: setLabels},
				VolumeClaimTemplates: []corev1.PersistentVolumeClaim{},
			},
		}

		return nil
	}
}

// 设置pod优先级
// 当设置为 scheduling.SystemCriticalPriority， kubelet会设置 OOMScore， 同时在pod无资源可分配时，会发起抢占： 删除其它低优先级pod来获取分配资源
func WithStatefulSetPodPriority(priority int32) StatefulSetResourcesOpts {
	return func(set *appsv1.StatefulSet) error {
		set.Spec.Template.Spec.Priority = &priority
		return nil
	}
}

func WithStatefulSetPvc(cluster *zookeeperv1.ZookeeperCluster) StatefulSetResourcesOpts {
	return func(set *appsv1.StatefulSet) error {
		if *cluster.Spec.ZookeeperResources.Storage.StorageClass == "" {

		}
		pvc := corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "data",
				Namespace: set.Namespace,
			},
			Spec: corev1.PersistentVolumeClaimSpec{
				AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
				Resources: corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceStorage: cluster.Spec.ZookeeperResources.Storage.Size,
					},
				},
				StorageClassName: cluster.Spec.ZookeeperResources.Storage.StorageClass,
			},
		}
		set.Spec.VolumeClaimTemplates = append(set.Spec.VolumeClaimTemplates, pvc)
		return nil
	}
}

func WithResourceSet(cluster *zookeeperv1.ZookeeperCluster) StatefulSetResourcesOpts {
	return func(set *appsv1.StatefulSet) error {

		return nil
	}
}

func WithAntiAffinity(weight int32, matchLabels map[string]string, clusterForcedAntiAffinity bool) StatefulSetResourcesOpts {
	return func(set *appsv1.StatefulSet) error {
		if set.Spec.Template.Spec.Affinity == nil {
			set.Spec.Template.Spec.Affinity = &corev1.Affinity{}
		}
		affinity := set.Spec.Template.Spec.Affinity
		if affinity.PodAntiAffinity == nil {
			affinity.PodAntiAffinity = &corev1.PodAntiAffinity{}
		}
		if clusterForcedAntiAffinity {
			affinity.PodAntiAffinity.RequiredDuringSchedulingIgnoredDuringExecution =
				append(affinity.PodAntiAffinity.RequiredDuringSchedulingIgnoredDuringExecution,
					corev1.PodAffinityTerm{
						TopologyKey: "kubernetes.io/hostname", // node scope
						LabelSelector: &metav1.LabelSelector{
							MatchLabels: matchLabels,
						},
					},
				)
		} else {
			affinity.PodAntiAffinity.PreferredDuringSchedulingIgnoredDuringExecution =
				append(affinity.PodAntiAffinity.PreferredDuringSchedulingIgnoredDuringExecution,
					corev1.WeightedPodAffinityTerm{
						Weight: weight,
						PodAffinityTerm: corev1.PodAffinityTerm{
							TopologyKey: "kubernetes.io/hostname", // node scope
							LabelSelector: &metav1.LabelSelector{
								MatchLabels: matchLabels,
							},
						},
					},
				)
		}

		return nil
	}
}

/*
1.为statefulSet添加OwnerReferences
2.遍历statefulSet的所有pvc，添加OwnerReferences
*/
func WithOwnerReference(reference metav1.OwnerReference) StatefulSetResourcesOpts {
	return func(set *appsv1.StatefulSet) error {
		set.SetOwnerReferences([]metav1.OwnerReference{reference})

		// 检查是否有pvc，也设置OwnerReference
		for i := range set.Spec.VolumeClaimTemplates {
			set.Spec.VolumeClaimTemplates[i].SetOwnerReferences([]metav1.OwnerReference{reference})
		}

		return nil
	}
}

// 开源是否适用？ TODO
// localpv-controller 管理的pvc，全部加上默认的selecotr
func WithPVCSelector(client client.Client) StatefulSetResourcesOpts {
	nop := func(set *appsv1.StatefulSet) error {
		return nil
	}

	scList := &v1.StorageClassList{}
	err := client.List(context.TODO(), scList)
	if err != nil {
		return nop
	}

	var localSc []string
	// creater: localpv-creator
	for _, sc := range scList.Items {
		if sc.Annotations == nil {
			continue
		}
		if sc.Annotations[utils2.LabelKey] == utils2.LabelValue {
			localSc = append(localSc, sc.Name)
		}
	}
	if len(localSc) == 0 {
		return nop
	}
	localScStr := fmt.Sprintf(",%s,", strings.Join(localSc, ","))

	return func(set *appsv1.StatefulSet) error {
		for i, pvc := range set.Spec.VolumeClaimTemplates {
			if pvc.Spec.StorageClassName == nil {
				continue
			}
			name := fmt.Sprintf(",%s,", *pvc.Spec.StorageClassName)
			if !strings.Contains(localScStr, name) {
				continue
			}
			if set.Spec.VolumeClaimTemplates[i].Spec.Selector == nil {
				set.Spec.VolumeClaimTemplates[i].Spec.Selector = &metav1.LabelSelector{}
			}
			if set.Spec.VolumeClaimTemplates[i].Spec.Selector.MatchLabels == nil {
				set.Spec.VolumeClaimTemplates[i].Spec.Selector.MatchLabels = make(map[string]string, 0)
			}
			// 确保所有的pvc都有一个selector，并且这个selector不对应任何pv。等待localpv-controller来重建
			set.Spec.VolumeClaimTemplates[i].Spec.Selector.MatchLabels[utils2.LabelKey] = "wait-to-recreate"
		}
		return nil
	}
}

/*
由于fluentbit比较通用，所以独立出来作为statefulSet组件。
传入fluentbit需要挂载的卷名
*/
func WithFluentbitContainer(logVolumeName string) StatefulSetResourcesOpts {
	return func(set *appsv1.StatefulSet) error {
		c := corev1.Container{
			Name:  "fluentbit",
			Image: fluentdImage,
			VolumeMounts: []corev1.VolumeMount{
				{
					MountPath: "/tmp/log",
					Name:      logVolumeName,
					ReadOnly:  true,
				},
				{
					MountPath: "/fluent-bit/etc",
					Name:      "fluentbit-config",
					ReadOnly:  true,
				},
			},
		}

		fluentVolume := corev1.Volume{
			Name: "fluentbit-config",
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					//TODO: Unified configuration name
					LocalObjectReference: corev1.LocalObjectReference{Name: fmt.Sprintf("%s-fluentbit", set.Name)},
					Items: []corev1.KeyToPath{
						{
							Key:  "fluent-bit.conf",
							Path: "fluent-bit.conf",
						},
						{
							Key:  "parsers.conf",
							Path: "parsers.conf",
						},
					},
				},
			},
		}

		set.Spec.Template.Spec.Containers = append(set.Spec.Template.Spec.Containers, c)
		set.Spec.Template.Spec.Volumes = append(set.Spec.Template.Spec.Volumes, fluentVolume)
		return nil
	}
}

func WithShareProcess() StatefulSetResourcesOpts {
	return func(set *appsv1.StatefulSet) error {
		var share bool = true
		set.Spec.Template.Spec.ShareProcessNamespace = &share
		return nil
	}
}

//func WithCustomerAnnotation(annotation map[string]string) StatefulSetResourcesOpts {
//	return func(set *appsv1.StatefulSet) error {
//		if annotation == nil {
//			return nil
//		}
//		nodeSelect := annotation[set.Name+apis.StsNodeSelector]
//		if nodeSelect == "" {
//			return nil
//		}
//		nodeList := strings.Split(nodeSelect, ",")
//		if len(nodeList) == 0 {
//			return nil
//		}
//		if set.Spec.Template.Spec.Affinity == nil {
//			set.Spec.Template.Spec.Affinity = &corev1.Affinity{}
//		}
//		if set.Spec.Template.Spec.Affinity.NodeAffinity == nil {
//			set.Spec.Template.Spec.Affinity.NodeAffinity = &corev1.NodeAffinity{}
//		}
//		nodeAffinity := set.Spec.Template.Spec.Affinity.NodeAffinity
//		if nodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution == nil {
//			nodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution = &corev1.NodeSelector{}
//		}
//
//		nodeSelector := nodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution
//
//		if len(nodeSelector.NodeSelectorTerms) == 0 || nodeSelector.NodeSelectorTerms[0].MatchExpressions == nil {
//			nodeSelector.NodeSelectorTerms = []corev1.NodeSelectorTerm{
//				{
//					MatchExpressions: []corev1.NodeSelectorRequirement{
//						{
//							Key:      "kubernetes.io/hostname",
//							Operator: corev1.NodeSelectorOpIn,
//							Values:   nodeList,
//						},
//					},
//				},
//			}
//			return nil
//		}
//
//		nodeSelector.NodeSelectorTerms[0].MatchExpressions = append(nodeSelector.NodeSelectorTerms[0].MatchExpressions,
//			corev1.NodeSelectorRequirement{
//				Key:      "kubernetes.io/hostname",
//				Operator: corev1.NodeSelectorOpIn,
//				Values:   nodeList,
//			})
//
//		return nil
//	}
//}

//func WithCustomerNodeLabelAnnotation(annotation map[string]string) StatefulSetResourcesOpts {
//	return func(set *appsv1.StatefulSet) error {
//		if annotation == nil {
//			return nil
//		}
//		nodeSelect := annotation[set.Name+apis.StsNodeLabelSelector]
//		nodeSelectAll := annotation[apis.StsAll+apis.StsNodeLabelSelector]
//		if nodeSelect == "" {
//			nodeSelect = nodeSelectAll
//			if nodeSelect == "" {
//				return nil
//			}
//		} else {
//			// merge
//			nodeSelect = nodeSelectAll + "," + nodeSelect // 如果有相同的key，后面的会覆盖前面的
//		}
//		nodeList := strings.Split(nodeSelect, ",")
//		if len(nodeList) == 0 {
//			return nil
//		}
//		if set.Spec.Template.Spec.NodeSelector == nil {
//			set.Spec.Template.Spec.NodeSelector = make(map[string]string, 0)
//		}
//		for _, sel := range nodeList {
//			if sel == "" {
//				continue
//			}
//			labs := strings.Split(sel, ":")
//			if len(labs) != 2 {
//				ctrl.Log.Info(fmt.Sprintf("get invalid nodeLabelAnnotation %s. ", sel))
//				continue
//			}
//			set.Spec.Template.Spec.NodeSelector[labs[0]] = labs[1]
//		}
//
//		return nil
//	}
//}
