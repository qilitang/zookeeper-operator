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

package status

import (
	"context"
	"fmt"
	"github.com/go-logr/logr"
	"github.com/qilitang/zookeeper-operator/utils"
	"k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"strings"
	"time"
)

type StatefulSetResourcesStatus struct {
	RemoteRequest *utils.RemoteRequest
	StatefulSet   *v1.StatefulSet
	client.Client
	Log logr.Logger
}

const runExecTimeout = 10

func (t StatefulSetResourcesStatus) IsReady() bool {

	if *t.StatefulSet.Spec.Replicas == 0 { // return ready if replicas is 0
		return true
	}

	if *t.StatefulSet.Spec.Replicas != t.StatefulSet.Status.Replicas {
		return false
	}

	if t.StatefulSet.Status.Replicas != t.StatefulSet.Status.ReadyReplicas {
		return false
	}

	if t.StatefulSet.Status.CurrentReplicas != t.StatefulSet.Status.Replicas {
		return false
	}

	if !t.IsPvcReady() {
		return false
	}

	return true
}

func (t StatefulSetResourcesStatus) IsRoleReady() (string, bool) {
	ctx, cancel := context.WithTimeout(context.Background(), runExecTimeout*time.Second)
	defer cancel()
	cmd := []string{
		"sh",
		"-c",
		"echo stat | nc localhost 2181",
	}
	stdout, stderr, err := t.RemoteRequest.Exec(ctx, t.StatefulSet.Namespace, t.StatefulSet.Name+"-0", utils.ZookeeperContainerName, cmd)
	if err != nil {
		return "", false
	}
	if stderr != "" {
		return "", false
	}
	if strings.Contains(stdout, "Mode: leader") {
		return utils.AnnotationsRoleLeader, true

	} else if strings.Contains(stdout, "Mode: follower") {
		return utils.AnnotationsRoleFollower, true
	}
	return "", false
}

func (t StatefulSetResourcesStatus) IsPvcReady() bool {
	pvcSize := make(map[string]int64, 0)
	var i int32
	//for pvcPrefix, reqPvcSize := range pvcSize {
	for pvcPrefix, _ := range pvcSize {
		for i = 0; i < *t.StatefulSet.Spec.Replicas; i++ {
			pvcName := GetPVCName(pvcPrefix, t.StatefulSet.Name, i)
			pvc := &corev1.PersistentVolumeClaim{}
			err := t.Get(context.TODO(), types.NamespacedName{Name: pvcName, Namespace: t.StatefulSet.Namespace}, pvc)
			if err != nil {
				t.Log.Error(err, fmt.Sprintf("get pvc %s/%s failed", t.StatefulSet.Namespace, pvcName))
				return false
			}

			sc := &storagev1.StorageClass{}
			err = t.Get(context.TODO(), types.NamespacedName{Name: *pvc.Spec.StorageClassName}, sc)
			if err != nil {
				t.Log.Error(err, fmt.Sprintf("get pvc %s/%s failed", t.StatefulSet.Namespace, pvcName))
				return false
			}
			//// 本地卷不检查pvc size
			//if sc.Provisioner != apis.LocalPVProvisioner {
			//	requestStorage := resource.NewQuantity(reqPvcSize, resource.BinarySI)
			//	// get current pvc size
			//	curStorage := pvc.Status.Capacity["storage"]
			//	if curStorage.Cmp(*requestStorage) != 0 {
			//		return false
			//	}
			//}
		}
	}
	return true
}

func (t StatefulSetResourcesStatus) IsFailed() (bool, *ProgressStep) {
	if t.IsReady() {
		return false, NewProgress(t.StatefulSet.Name, "statefulSet", "ready. ")
	}

	podList := &corev1.PodList{}
	listOpts := []client.ListOption{
		client.InNamespace(t.StatefulSet.Namespace),
		client.MatchingLabels(t.StatefulSet.Labels),
	}
	if err := t.List(context.TODO(), podList, listOpts...); err != nil {
		t.Log.Error(err, "Failed to list statefulset's pods", "Statefulset.Name", t.StatefulSet.Name)
		return false, NewProgress(t.StatefulSet.Name, "statefulSet", "wait pod to create.")
	}

	step := NewProgress(t.StatefulSet.Name, "statefulSet", "wait pod to ready. ")
	for _, pod := range podList.Items {
		podRes := PodResourcesStatus{
			Pod:    &pod,
			Client: t.Client,
			Log:    t.Log.WithName("PodResourcesStatus"),
		}
		isFail, subStep := podRes.IsFailed()
		t.Log.V(3).Info(fmt.Sprintf("check pod status [%s] get step: [%v] ", pod.Status.Phase, subStep.ToString("", 0)))
		step.AddChild(subStep)
		if isFail {
			t.Log.Info(fmt.Sprintf("pod [%s] is failed status [%s]. ", pod.Name, step.ToString("", 0)))
			return true, step
		}
	}

	return false, step
}

func GetPVCName(storageName string, statefulSetName string, ordinal int32) string {
	return fmt.Sprintf("%s-%s-%d", storageName, statefulSetName, ordinal)
}
