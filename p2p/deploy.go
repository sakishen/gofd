package p2p

import (
	"fmt"
	"github.com/go-cmd/cmd"
	"github.com/xtfly/gofd/common"
	"net/http"
	"os"
	"strconv"
)

const (
	DeployEmpty = iota
	DeployGameData
	DeployUserData
	DeployOtherData
	DeployDataEnd
)

type deployInfo struct {
	serverAddr string
	fileType   int
	gid        int
	version    int
	storageDir string
	Files      []*FileDict
}

type deployer struct {
	taskID string
	cfg    *common.Config
	client *http.Client

	deployChan chan *deployInfo
}

func newDeployer(taskID string, cfg *common.Config) *deployer {
	d := &deployer{
		taskID:     taskID,
		cfg:        cfg,
		client:     common.CreateHTTPClient(cfg),
		deployChan: make(chan *deployInfo, 20),
	}

	go d.run()
	return d
}

func (r *deployer) run() {
	for rc := range r.deployChan {
		r.deployImp(rc)
	}
}

// DoDeploy 分发完毕后会调用此函数用于加压归档数据
func (r *deployer) DoDeploy(serverAddr string, gid int, version int, fileType int, storageDir string, files []*FileDict) {
	r.deployChan <- &deployInfo{serverAddr: serverAddr, gid: gid, version: version, fileType: fileType, storageDir: storageDir, Files: files}
}

func (r *deployer) Close() {
	close(r.deployChan)
}

func (r *deployer) deployImp(ri *deployInfo) {
	if ri.fileType > DeployDataEnd {
		common.LOG.Error("deploy file type illegal")
		return
	}

	common.LOG.Errorf("[%s] Deploy Done, fileType:%v, gid=%v, version=%v, storageDir:%v.", ri.fileType, r.taskID, ri.gid, ri.version, ri.storageDir)
	// TODO: 增加解压及创建目录的方法
	// 1. 检查指定目录是否存在，存在就判断是否是 subvolume, 不存在就创建 （stat -f --format=%T /path）
	// 2. 解压归档文件到指定目录

	// 根据任务文件类型设置解压路径
	switch ri.fileType {
	case DeployGameData:
		common.LOG.Error("Deploy Game data")
		path := fmt.Sprintf("%s/%v_%v", r.cfg.GameDir, ri.gid, ri.version)
		target := fmt.Sprintf("%s/%v_%v.tar.gz", r.cfg.DownDir, ri.gid, ri.version)
		r.decompression(target, path)
	case DeployUserData:
		common.LOG.Error("Deploy User data")
		path := fmt.Sprintf("%s/%s", r.cfg.UserDateDir, ri.storageDir)
		target := fmt.Sprintf("%s/%v_%v_user.tar.gz", r.cfg.DownDir, ri.gid, ri.version)
		r.decompression(target, path)
	case DeployOtherData:
		common.LOG.Error("Deploy Other data")
	}

	/*csr := &StatusReport{
		TaskID: r.taskID,
		IP:     r.cfg.Net.Host, // 上报进度使用
		//PercentComplete: ri.percentComplete,
	}
	bs, err := json.Marshal(csr)
	if err != nil {
		common.LOG.Errorf("[%s] Report session status failed. error=%v", r.taskID, err)
		return
	}

	_, err = common.SendHTTPReq(r.cfg, "POST",
		ri.serverAddr, "/api/v1/server/tasks/status", bs)
	if err != nil {
		common.LOG.Errorf("[%s] Report session status failed. error=%v", r.taskID, err)
	}*/

	return
}

func (r *deployer) decompression(target, path string) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		// path/to/whatever does not exist
		if err := r.createBtrfsSubvolum(path); err != nil {
			common.LOG.Errorf("[%s] btrfs subvolume create dir failed, err: %v", r.taskID, err)
		}

		if err := r.ChownUserGroupDir(path, r.cfg.UserID, r.cfg.GroupID); err != nil {
			common.LOG.Errorf("[%s] chown %v:%v failed, err: %v", r.taskID, r.cfg.UserID, r.cfg.GroupID, err)
		}
	}

	// 解压数据, 并同意设置文件所属用户及所属工作组
	if err := common.UnTarGz(target, path, r.cfg.UserID, r.cfg.GroupID); err != nil {
		common.LOG.Error("UnTarGz fail")
	}
}

func (r *deployer) createBtrfsSubvolum(path string) error {
	common.LOG.Error("btrfs subvolume create:", path)
	createCmd := cmd.NewCmd("btrfs", "subvolume", "create", path)
	stateChan := createCmd.Start()
	finalStatus := <-stateChan
	if finalStatus.Error != nil {
		common.LOG.Error(finalStatus.Error)
		return finalStatus.Error
	}

	for _, info := range finalStatus.Stdout {
		common.LOG.Error(info)
	}

	return nil
}

// ChownUserGroupDir call change owner command
func (r *deployer) ChownUserGroupDir(path string, user int, group int) error {
	// chown -R owner:group path
	common.LOG.Debug("chownUserGroupDir user:", user, ", group:", group, ", path:", path)
	chownCmd := cmd.NewCmd("chown", "-R", strconv.Itoa(user)+":"+strconv.Itoa(group), path)
	stateChan := chownCmd.Start()
	finalStatus := <-stateChan

	if finalStatus.Error != nil {
		common.LOG.Error(finalStatus.Error)
		return finalStatus.Error
	}

	for _, info := range finalStatus.Stdout {
		common.LOG.Error(info)
	}

	return nil
}
