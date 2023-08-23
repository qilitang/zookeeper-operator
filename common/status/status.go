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
	"fmt"

	ctrl "sigs.k8s.io/controller-runtime"
)

/*
获取资源的所有状态
1. 判断ready时，不深究子资源状态
2. 判断fail时，需要遍历所有子资源，除了返回失败标志，也返回子资源详细信息
*/
type ResourcesStatus interface {
	IsReady() bool
	/*
		判断当前资源状态是否失败，并收集进度条信息。
		如果当前资源已经ready，则不再收集子资源信息。
	*/
	IsFailed() (bool, *ProgressStep)
}

const (
	STATUS_INIT        = " " // 初始状态
	STATUS_CREATING    = "C" // 创建中
	STATUS_AVAILABLE   = "A" // 可用
	STATUS_WARNING     = "W" // 可用,带警告
	STATUS_PAUSING     = "P" // 暂停中
	STATUS_PAUSED      = "D" // 已停用
	STATUS_MAINTAINING = "I" // 进入维护模式中
	STATUS_MAINTAINED  = "M" // 已进入维护模式
	STATUS_RECOVERING  = "R" // 集群自动恢复中
	STATUS_UPDATING    = "U" // 更新中
	STATUS_UNAVAILABLE = "E" // 不可用
	STATUS_FAILED      = "F" // 失败（失败会将pod副本数设置为0，避免资源浪费）
	STATUS_READONLY    = "O" // 集群因为业务限制成为只读状态
	STATUS_DELETING    = "T" // 集群删除中
)

var StatusDescription = map[string]string{
	STATUS_INIT:        "init",
	STATUS_CREATING:    "creating",
	STATUS_AVAILABLE:   "available",
	STATUS_WARNING:     "warning",
	STATUS_PAUSING:     "pausing",
	STATUS_PAUSED:      "paused",
	STATUS_MAINTAINING: "maintaining",
	STATUS_MAINTAINED:  "maintained",
	STATUS_RECOVERING:  "recovering",
	STATUS_UPDATING:    "updating",
	STATUS_UNAVAILABLE: "unavailable",
	STATUS_FAILED:      "failed",
	STATUS_READONLY:    "readonly",
	STATUS_DELETING:    "deleting",
}

var StatusValue = map[string]string{
	"init":        STATUS_INIT,
	"creating":    STATUS_CREATING,
	"available":   STATUS_AVAILABLE,
	"warning":     STATUS_WARNING,
	"pausing":     STATUS_PAUSING,
	"paused":      STATUS_PAUSED,
	"maintaining": STATUS_MAINTAINING,
	"maintained":  STATUS_MAINTAINED,
	"recovering":  STATUS_RECOVERING,
	"updating":    STATUS_UPDATING,
	"unavailable": STATUS_UNAVAILABLE,
	"failed":      STATUS_FAILED,
	"readonly":    STATUS_READONLY,
	"deleting":    STATUS_DELETING,
}

type ActionBranch int

const (
	ActionNone    ActionBranch = iota
	StatusReady                // 当前状态为ready
	StatusWarning              // 当前状态为warning
	StatusFail                 // 当前状态为fail

	StatusEnd // 用于判断当前操作是action还是status

	ActionPause         // 发起暂停动作
	ActionUnPause       // 发起暂停恢复动作
	ActionUpdate        // 发起更新动作
	ActionEnterMaintain // 进入维护模式
	ActionExitMaintain  // 退出维护模式
)

/*
FSM状态信息表
From 开始状态
To 目的状态
Action 状态切换动作 找不到对应动作的切换时，ActionNone为默认动作。
*/
type StatusFSM struct {
	From   string
	To     string
	Action ActionBranch
}

var clusterFSM = []StatusFSM{
	{STATUS_INIT, STATUS_CREATING, ActionNone},
	// 创建中
	{STATUS_CREATING, STATUS_CREATING, ActionNone},
	// C->A：集群创建成功
	{STATUS_CREATING, STATUS_AVAILABLE, StatusReady},
	// C->C: 创建时变更
	{STATUS_CREATING, STATUS_CREATING, ActionUpdate},
	// C->FC:集群创建失败
	{STATUS_CREATING, STATUS_FAILED, StatusFail},
	// A->P：集群暂停
	{STATUS_AVAILABLE, STATUS_PAUSING, ActionPause},
	{STATUS_WARNING, STATUS_PAUSING, ActionPause},
	// A->RA：集群异常，进入自动恢复
	{STATUS_AVAILABLE, STATUS_RECOVERING, ActionNone},
	// A->WA: 集群出现警告
	{STATUS_AVAILABLE, STATUS_WARNING, StatusWarning},
	// A->U：集群更新
	{STATUS_AVAILABLE, STATUS_UPDATING, ActionUpdate},
	{STATUS_WARNING, STATUS_UPDATING, ActionUpdate},
	// 集群保持ready
	{STATUS_AVAILABLE, STATUS_AVAILABLE, StatusReady},
	// A->M: 进入维护模式
	{STATUS_AVAILABLE, STATUS_MAINTAINING, ActionEnterMaintain},
	{STATUS_UNAVAILABLE, STATUS_MAINTAINING, ActionEnterMaintain},
	{STATUS_WARNING, STATUS_MAINTAINING, ActionEnterMaintain},
	// W->A: 从警告中恢复
	{STATUS_WARNING, STATUS_AVAILABLE, StatusReady},
	// stay
	{STATUS_WARNING, STATUS_WARNING, StatusWarning},
	{STATUS_WARNING, STATUS_UNAVAILABLE, ActionNone},
	// 暂停中
	{STATUS_PAUSING, STATUS_PAUSING, ActionNone},
	// P->D：集群暂停完成
	{STATUS_PAUSING, STATUS_PAUSED, StatusReady},
	// D->U：集群暂停恢复
	{STATUS_PAUSED, STATUS_UPDATING, ActionUnPause},
	// stay
	{STATUS_PAUSED, STATUS_PAUSED, ActionNone},
	// U->A：集群更新成功
	{STATUS_UPDATING, STATUS_AVAILABLE, StatusReady},
	// 更新中
	{STATUS_UPDATING, STATUS_UPDATING, ActionNone},
	// U->U： 集群更新过程中重新发起更新
	{STATUS_UPDATING, STATUS_UPDATING, ActionUpdate},
	// RA->A:集群自动恢复成功
	{STATUS_RECOVERING, STATUS_AVAILABLE, StatusReady},
	// 恢复中
	{STATUS_RECOVERING, STATUS_RECOVERING, StatusWarning},
	// 异常中更新来恢复
	{STATUS_RECOVERING, STATUS_UPDATING, ActionUpdate},
	// R->ER:集群自动恢复失败
	{STATUS_RECOVERING, STATUS_UNAVAILABLE, ActionNone},
	// EA->A：集群从异常中恢复
	{STATUS_UNAVAILABLE, STATUS_AVAILABLE, StatusReady},
	{STATUS_UNAVAILABLE, STATUS_WARNING, StatusWarning},
	// stay
	{STATUS_UNAVAILABLE, STATUS_UNAVAILABLE, ActionNone},
	// EU->UE：集群回滚重新更新
	{STATUS_UNAVAILABLE, STATUS_UPDATING, ActionUpdate},
	// stay fail
	{STATUS_FAILED, STATUS_FAILED, ActionNone},
	// 进入维护模式过程中
	{STATUS_MAINTAINING, STATUS_MAINTAINING, ActionNone},
	// 已进入维护模式
	{STATUS_MAINTAINING, STATUS_MAINTAINED, StatusReady},
	// stay
	{STATUS_MAINTAINED, STATUS_MAINTAINED, ActionNone},
	// 退出维护模式
	{STATUS_MAINTAINED, STATUS_UPDATING, ActionExitMaintain},
	// 集群保持只读状态
	{STATUS_READONLY, STATUS_READONLY, ActionNone},
	{STATUS_READONLY, STATUS_READONLY, ActionUpdate},
}

/*
状态转换
maybe为flag无法精确匹配时的默认转换规则
*/
func StatusConvert(from string, flag ActionBranch) (string, error) {
	log := ctrl.Log.WithName("StatusConvert")
	if from == "" {
		from = STATUS_INIT // INIT
	}
	var maybe StatusFSM
	for _, status := range clusterFSM {
		if status.From == from || status.From[0] == from[0] {
			if status.Action == flag {
				log.Info(fmt.Sprintf("convert status from [%s:%v] to [%s]. ", from, flag, status.To))
				return status.To, nil
			} else if status.Action == ActionNone {
				maybe = status
			}
		}
	}
	if flag > StatusEnd { // action切换，不能走默认配置
		return "", fmt.Errorf("cann't convert  %s with action %v. ", from, flag)
	}
	if maybe.To != "" {
		log.Info(fmt.Sprintf("convert status from [%s:%v] to maybe [%s]. ", from, flag, maybe.To))
		return maybe.To, nil
	}
	return "", fmt.Errorf("cann't convert status from [%s] [%v]. ", from, flag)
}
