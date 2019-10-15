package server

//----------------------------------------
import "time"

// CreateTask 创建分发任务
type CreateTask struct {
	ID            string   `json:"id"`
	Gid           int      `json:"gid"`
	Version       int      `json:"version"`
	DispatchFiles []string `json:"dispatchFiles"`
	DestIPs       []string `json:"destIPs"`
}

// StartArchive 创建归档任务
type StartArchive struct {
	ID             string `json:"id"`
	Gid            int    `json:"gid"`
	Version        int    `json:"version"`
	ArchiveDirPath string `json:"archiveDirPath"`
	DestFilePath   string `json:"destFilePath"`
}

// CreateDeployTask 创建部署任务
type CreateDeployTask struct {
	ID            string   `json:"id"`            // 任务 ID
	DispatchFiles []string `json:"dispatchFiles"` // 部署任务相关列表
	DestIPs       []string `json:"destIPs"`       // 参与部署任务的机器 IP
}

// TaskInfo 查询分发任务
type TaskInfo struct {
	ID     string `json:"id"`
	Status string `json:"status"`

	StartedAt  time.Time `json:"startedAt"`
	FinishedAt time.Time `json:"finishedAt"`

	DispatchInfos map[string]*DispatchInfo `json:"dispatchInfos,omitempty"`
}

// ArchiveInfo 查询归档任务
type ArchiveInfo struct {
	ID     string `json:"id"`
	Status string `json:"status"`

	StartedAt  time.Time `json:"startedAt"`
	FinishedAt time.Time `json:"finishedAt"`
}

// DeployTaskInfo 查询分发任务
type DeployTaskInfo struct {
	ID     string `json:"id"`
	Status string `json:"status"`

	StartedAt  time.Time `json:"startedAt"`
	FinishedAt time.Time `json:"finishedAt"`

	DispatchInfos map[string]*DispatchInfo `json:"dispatchInfos,omitempty"`
}

// DispatchInfo 单个IP的分发信息
type DispatchInfo struct {
	Status          string  `json:"status"`
	PercentComplete float32 `json:"percentComplete"`

	StartedAt  time.Time `json:"startedAt"`
	FinishedAt time.Time `json:"finishedAt"`

	DispatchFiles []*DispatchFile `json:"dispatchFiles"`
}

// DispatchFile 单个文件分发状态
type DispatchFile struct {
	FileName        string  `json:"filename"`
	PercentComplete float32 `json:"-"`
}

// TaskStatus 任务状态
type TaskStatus int

// the enum of TaskStatus
const (
	TaskNotExist TaskStatus = iota
	TaskExist
	TaskInit
	TaskFailed
	TaskCompleted
	TaskInProgress
	TaskFileNotExist
)

// convert task status to a string
func (ts TaskStatus) String() string {
	switch ts {
	case TaskNotExist:
		return "TASK_NOT_EXISTED"
	case TaskExist:
		return "TASK_EXISTED"
	case TaskInit:
		return "INIT"
	case TaskFailed:
		return "FAILED"
	case TaskCompleted:
		return "COMPLETED"
	case TaskInProgress:
		return "INPROGESS"
	case TaskFileNotExist:
		return "FILE_NOT_EXISTED"
	default:
		return "TASK_NOT_EXISTED"
	}
}
