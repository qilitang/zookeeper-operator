/*
 Copyright 2017 - 2020 Crunchy Data Solutions, Inc.
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
	zookeeperv1 "github.com/qilitang/zookeeper-operator/api/v1"
	"strings"
)

func ChangeClusterStatus(clusterStatus *zookeeperv1.ClusterStatus, flag ActionBranch) error {

	if flag == ActionUpdate { // 发起更新时，记录标志
		clusterStatus.PermitRollback = true
	}

	newStatus, err := StatusConvert(clusterStatus.FSMStatus, flag)
	if err != nil {
		return err
	}
	clusterStatus.FSMStatus = AppendStatus(newStatus, clusterStatus.FSMStatus)

	clusterStatus.CustomStatus = StatusDescription[newStatus[:1]]

	if clusterStatus.FSMStatus[:1] == STATUS_AVAILABLE { // 更新完成，重置标志
		clusterStatus.PermitRollback = false
	}

	return nil
}

func SetClusterStatus(clusterStatus *zookeeperv1.ClusterStatus, newStatus string) error {
	clusterStatus.FSMStatus = AppendStatus(newStatus, clusterStatus.FSMStatus)
	clusterStatus.CustomStatus = StatusDescription[newStatus[:1]]
	return nil
}

/*
Status update:
The first char represents the current status, the latter represents the historical status
1. If there is the same prefix, it means that the state has not changed, and the state is not updated
2. If the prefix is different, it means that the state is updated, put the new state to the top
3. The length of the status does not exceed 30 characters
*/
func AppendStatus(newStatus, oldStatus string) string {
	if strings.HasPrefix(oldStatus, newStatus) {
		return oldStatus
	}
	newStatus = newStatus + oldStatus
	if len(newStatus) <= 30 {
		return newStatus
	}
	return newStatus[:30]
}
