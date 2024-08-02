package utils

const (
	AppNameLabelKey          = "app-name"
	CreatedByLabelKey        = "created-by"
	ZookeeperContainerName   = "zookeeper"
	SetName                  = "set-name"
	ZookeeperJVMFLAGSEnvName = "SERVER_JVMFLAGS"
	ZookeeperHeapEnvName     = "ZK_SERVER_HEAP"
)

const (
	// localpv管理的pv和pvc都会加上这个标签，表示由localpv管理
	LabelKey   = "creater"
	LabelValue = "localpv-creator"
)

// Ratio
const JVMRatio = 0.75

// annotations

const (
	AnnotationsRoleKey      = "role"
	AnnotationsRoleFollower = "follower"
	AnnotationsRoleLeader   = "leader"
	AnnotationsRoleNotReady = "not-ready"
)
