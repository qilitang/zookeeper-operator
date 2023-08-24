package utils

const (
	AppNameLabelKey   = "app-name"
	CreatedByLabelKey = "created-by"
	DBVersionLabelKey = "zk-version"

	ZookeeperContainerName = "zookeeper"
	SetName                = "set-name"
)

const (
	// localpv管理的pv和pvc都会加上这个标签，表示由localpv管理
	LabelKey   = "creater"
	LabelValue = "localpv-creator"
	KeyStorage = "storage"
)
