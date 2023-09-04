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
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/qilitang/zookeeper-operator/utils"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sort"
	"time"

	"github.com/go-logr/logr"
	"k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	PodTimeout = flag.Duration("podTimeout", time.Minute*10, "pod scheduler failed timeout. ")
)

type PodResourcesStatus struct {
	Pod *v1.Pod
	client.Client
	Log logr.Logger
}

var mustConditions = []v1.PodConditionType{
	v1.PodScheduled,
	v1.PodInitialized,
	v1.PodReady,
}

func (t PodResourcesStatus) IsReady() bool {

	if t.Pod.Status.Phase != v1.PodRunning {
		return false
	}

	// 当Phase为Running时，判断conditions
	// conditions 必须要有三个type：PodScheduled, Initialized, Ready 且状态要全部是True

	if t.Pod.Status.Conditions == nil || len(t.Pod.Status.Conditions) == 0 {
		return false
	}
	statusMap := make(map[v1.PodConditionType]bool, 0)
	for _, condition := range t.Pod.Status.Conditions {
		statusMap[condition.Type] = condition.Status == v1.ConditionTrue
	}
	for _, condition := range mustConditions {
		if !statusMap[condition] {
			return false
		}
	}

	return true
}

/*
失败判断条件：
 1. PodScheduled状态失败  overtime > 5 minute
    时间从event中获取，取最后一个event, 如果是FailedScheduling，并且失败超过10次，则认为资源不足，失败
 2. Initialized失败超过5次
 3. Ready失败超过5次
*/
func (t PodResourcesStatus) IsFailed() (bool, *ProgressStep) {
	if t.IsReady() {
		return false, NewProgress(t.Pod.Name, t.Pod.Kind, "ready. ")
	}
	//eventList := &v1.EventList{}
	//eventOpts := []client.ListOption{
	//	client.MatchingFields(fields.Set{
	//		"involvedObject.name":      t.Pod.Name,
	//		"involvedObject.namespace": t.Pod.Namespace,
	//	}),
	//}
	listOptions := metav1.ListOptions{
		FieldSelector: fmt.Sprintf("involvedObject.name=%s,involvedObject.namespace=%s", t.Pod.Name, t.Pod.Namespace),
	}
	newClient, _ := utils.NewRemoteRequest()
	eventList, err := newClient.ClientSet.CoreV1().Events(t.Pod.Namespace).List(context.Background(), listOptions)
	//err := t.List(context.TODO(), eventList, eventOpts...)
	if err != nil || len(eventList.Items) == 0 {
		t.Log.Info(fmt.Sprintf("get events empty [%v] [%d] [%s/%s]. ", err, len(eventList.Items), t.Pod.Name, t.Pod.Namespace))
	} else {
		sort.Sort(sort.Reverse(SortableEvents(eventList.Items)))
		lastEvent := eventList.Items[0]
		t.Log.Info(fmt.Sprintf("get lastEvent [%s, %s-%s, %s, %s] [%d]", lastEvent.Name,
			lastEvent.FirstTimestamp.Format(time.RFC3339), lastEvent.LastTimestamp.Format(time.RFC3339),
			lastEvent.Reason, lastEvent.Message, lastEvent.Count))
		timeDiff := lastEvent.LastTimestamp.Sub(lastEvent.FirstTimestamp.Time).Minutes()
		if timeDiff > PodTimeout.Minutes() && lastEvent.Reason == "FailedScheduling" {
			m, _ := json.Marshal(lastEvent)
			t.Log.Info("pod has schedule failed", m)
			return true, NewProgress(t.Pod.Name, "pod-schedule", fmt.Sprintf("scheduled fail [%s/%d]. ", lastEvent.Message, lastEvent.Count))
		}
	}

	if t.Pod.Status.Conditions == nil || len(t.Pod.Status.Conditions) == 0 {
		return false, NewProgress(t.Pod.Name, "Pod", "no conditions. Wait for controller manager to manager. ")
	}

	var scheduleCondition, initCondition, readyCondition *v1.PodCondition
	for idx, condition := range t.Pod.Status.Conditions {
		if condition.Type == v1.PodScheduled {
			scheduleCondition = &t.Pod.Status.Conditions[idx]
		}
		if condition.Type == v1.PodInitialized {
			initCondition = &t.Pod.Status.Conditions[idx]
		}
		if condition.Type == v1.PodReady {
			readyCondition = &t.Pod.Status.Conditions[idx]
		}
	}
	if scheduleCondition == nil || scheduleCondition.Status == "" {
		return false, NewProgress(t.Pod.Name, "pod-schedule", "no scheduled. ")
	}
	if scheduleCondition.Reason == v1.PodReasonUnschedulable {
		return false, NewProgress(t.Pod.Name, "pod-schedule", fmt.Sprintf("Unschedulable, please waiting for %s. ", scheduleCondition.Message))
	}
	if scheduleCondition.Status == v1.ConditionFalse {
		return false, NewProgress(t.Pod.Name, "pod-schedule", fmt.Sprintf("pod is scheduling fail [%s]. ", scheduleCondition.Message))
	}
	if scheduleCondition.Status != v1.ConditionTrue {
		t.Log.Info(fmt.Sprintf("pod [%s] schedule fail %s. ", t.Pod.Name, scheduleCondition.Message))
		return false, NewProgress(t.Pod.Name, "pod-schedule", fmt.Sprintf("scheduled fail [%s]. ", scheduleCondition.Message))
	}

	step := NewProgress(t.Pod.Name, "pod-schedule", "scheduled ok. ")

	if initCondition == nil {
		return false, step.AddNext(NewProgress(t.Pod.Name, "Pod init", "wait to init. "))
	}
	if initCondition.Status != v1.ConditionTrue {
		initStep := NewProgress(t.Pod.Name, "pod-init", "Pod initializing. ")
		step.AddNext(initStep)
		isFailed := t.CheckSubStatus(t.Pod.Status.InitContainerStatuses, initStep)
		if isFailed {
			return isFailed, step
		}
		return false, step
	}
	step.AddNext(NewProgress(t.Pod.Name, "pod-init", "init ok. "))

	if readyCondition == nil {
		return false, step.AddNext(NewProgress(t.Pod.Name, "Pod run", "wait to run. "))
	}
	if readyCondition.Status != v1.ConditionTrue {
		runStep := NewProgress(t.Pod.Name, "pod-running", "pod is running, wait to be ready. ")
		step.AddNext(runStep)
		isFailed := t.CheckSubStatus(t.Pod.Status.ContainerStatuses, runStep)
		if isFailed {
			return isFailed, step
		}
		return false, step
	}
	step.AddNext(NewProgress(t.Pod.Name, "pod-running", "run ok. "+string(readyCondition.Status)))

	return false, step
}

func (t PodResourcesStatus) CheckSubStatus(containerStatus []v1.ContainerStatus, step *ProgressStep) bool {

	for _, status := range containerStatus {
		if status.Ready {
			step.AddChild(NewProgress(t.Pod.Name, status.Name, "ready. "))
			continue
		}
		if status.RestartCount > 5 {
			step.AddChild(NewProgress(t.Pod.Name, status.Name, GetStatusInfo(t.Pod.Name, status.State)+": fail over 5 times."))
			return true
		} else {
			step.AddChild(NewProgress(t.Pod.Name, status.Name, GetStatusInfo(t.Pod.Name, status.State)))
		}
	}

	return false
}

func GetStatusInfo(podName string, status v1.ContainerState) string {
	buffer := bytes.Buffer{}

	if status.Waiting != nil {
		buffer.WriteString(fmt.Sprintf("pod %s is waiting for [%s/%s]. ", podName,
			status.Waiting.Reason, status.Waiting.Message))
	}
	if status.Running != nil {
		buffer.WriteString(fmt.Sprintf("pod %s is running startedAt [%s]. ", podName,
			status.Running.StartedAt.Format(time.RFC3339)))
	}
	if status.Terminated != nil {
		buffer.WriteString(fmt.Sprintf("pod %s is terminated s/e[%d/%d] r/m[%s/%s] s/e[%s/%s]. ", podName,
			status.Terminated.Signal, status.Terminated.ExitCode,
			status.Terminated.Reason, status.Terminated.Message,
			status.Terminated.StartedAt.Format(time.RFC3339), status.Terminated.FinishedAt.Format(time.RFC3339)))
	}
	return buffer.String()
}

type SortableEvents []v1.Event

func (list SortableEvents) Len() int {
	return len(list)
}

func (list SortableEvents) Swap(i, j int) {
	list[i], list[j] = list[j], list[i]
}

func (list SortableEvents) Less(i, j int) bool {
	return list[i].LastTimestamp.Time.Before(list[j].LastTimestamp.Time)
}
