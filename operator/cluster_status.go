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
	"fmt"
	"github.com/go-logr/logr"
	zookeeperv1 "github.com/qilitang/zookeeper-operator/api/v1"
	"github.com/qilitang/zookeeper-operator/utils"
	appsv1 "k8s.io/api/apps/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	commonstatus "github.com/qilitang/zookeeper-operator/common/status"
)

type ZookeeperClusterResourcesStatus struct {
	client.Client
	Log     logr.Logger
	Cluster *zookeeperv1.ZookeeperCluster
}

func (t *ZookeeperClusterResourcesStatus) IsReady() bool {

	if t.Cluster.Status.ReadyReplicas == t.Cluster.Spec.Replicas {
		return true
	}

	return false
}

func (t *ZookeeperClusterResourcesStatus) UpdateStatus() error {

	status := &t.Cluster.Status

	status.Replicas = 0
	status.ReadyReplicas = 0
	status.CurrentReplicas = 0
	status.UpdatedReplicas = 0
	matchLabels := utils.CopyMap(NewDatabaseLabel(t.Cluster))
	setList := &appsv1.StatefulSetList{}
	setOpts := []client.ListOption{
		client.InNamespace(t.Cluster.Namespace),
		client.MatchingLabels(matchLabels),
	}
	if err := t.Client.List(context.TODO(), setList, setOpts...); err != nil {
		t.Log.Error(err, "Failed to list statefulsets",
			"ZookeeperCluster.Namespace", t.Cluster.GetNamespace(), "MysqlCluster.Name", t.Cluster.GetName())
		return err
	}

	for i := range setList.Items {
		status.Replicas++
		setStatus := commonstatus.StatefulSetResourcesStatus{
			StatefulSet: &setList.Items[i],
			Client:      t.Client,
			Log:         t.Log.WithName("StatefulSetResourcesStatus"),
		}

		if setStatus.IsReady() {
			status.ReadyReplicas++
		}
	}

	var flag = commonstatus.ActionNone
	if t.IsReady() {
		flag = commonstatus.StatusReady
		t.Cluster.Status.StatusDetails = ""
	} else if ok, step := t.IsWarning(); ok {
		flag = commonstatus.StatusWarning
		if step != nil {
			t.Cluster.Status.StatusDetails = step.ToString("", 0)
		}
	} else {
		isFail, step := t.IsFailed()
		if isFail {
			flag = commonstatus.StatusFail
		}
		if step != nil {
			t.Cluster.Status.StatusDetails = step.ToString("", 0)
		}
	}

	err := commonstatus.ChangeClusterStatus(&t.Cluster.Status.ClusterStatus, flag)
	if err != nil {
		return err
	}

	return nil
}

func (t *ZookeeperClusterResourcesStatus) IsWarning() (bool, *commonstatus.ProgressStep) {
	score := 0 // 主库正常加60分，从库正常加1分，总分大于60表示warning
	if t.IsReady() {
		return false, commonstatus.NewProgress(t.Cluster.Name, "cluster", "ready. ")
	}

	currentMaster := ""

	setList := &appsv1.StatefulSetList{}
	setOpts := []client.ListOption{
		client.InNamespace(t.Cluster.Namespace),
		client.MatchingLabels(NewDatabaseLabel(t.Cluster)),
	}
	if err := t.Client.List(context.TODO(), setList, setOpts...); err != nil {
		t.Log.Error(err, "Failed to list statefulsets",
			"MysqlCluster.Namespace", t.Cluster.GetNamespace(), "MysqlCluster.Name", t.Cluster.GetName())
		return false, commonstatus.NewProgress(t.Cluster.Name, "cluster", "wait to create database. ")
	}
	step := commonstatus.NewProgress(t.Cluster.Name, "cluster", "wait database to ready. ")
	for _, set := range setList.Items {
		setStatus := commonstatus.StatefulSetResourcesStatus{
			StatefulSet: &set,
			Client:      t.Client,
			Log:         t.Log.WithName("StatefulSetResourcesStatus"),
		}

		if setStatus.IsReady() && set.Name == currentMaster {
			score += 60
		} else if setStatus.IsReady() {
			score += 1
		}
	}

	if score >= 60 && t.Cluster.Status.FSMStatus != "" && t.Cluster.Status.FSMStatus[:1] != commonstatus.STATUS_CREATING { // creating without warning
		t.Log.Info(fmt.Sprintf("cluster is warning with score %d. ", score))
		return true, step
	}

	t.Log.Info(fmt.Sprintf("cluster's score is %d. ", score))
	return false, step
}

func (t *ZookeeperClusterResourcesStatus) IsFailed() (bool, *commonstatus.ProgressStep) {

	if t.IsReady() {
		return false, commonstatus.NewProgress(t.Cluster.Name, "cluster", "ready. ")
	}

	if t.Cluster.Status.CustomStatus == commonstatus.StatusDescription[commonstatus.STATUS_FAILED] {
		return true, nil
	}

	setList := &appsv1.StatefulSetList{}
	setOpts := []client.ListOption{
		client.InNamespace(t.Cluster.Namespace),
		client.MatchingLabels(NewDatabaseLabel(t.Cluster)),
	}
	if err := t.Client.List(context.TODO(), setList, setOpts...); err != nil {
		t.Log.Error(err, "Failed to list statefulsets",
			"MysqlCluster.Namespace", t.Cluster.GetNamespace(), "MysqlCluster.Name", t.Cluster.GetName())
		return false, commonstatus.NewProgress(t.Cluster.Name, "cluster", "wait to create database. ")
	}

	step := commonstatus.NewProgress(t.Cluster.Name, "cluster", "wait database to ready. ")
	for _, set := range setList.Items {
		setStatus := commonstatus.StatefulSetResourcesStatus{
			StatefulSet: &set,
			Client:      t.Client,
			Log:         t.Log.WithName("StatefulSetResourcesStatus"),
		}
		isFailed, subStep := setStatus.IsFailed()
		step.AddChild(subStep)
		if isFailed {
			return isFailed, step
		}
	}

	return false, step
}
