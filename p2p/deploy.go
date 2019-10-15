package p2p

import (
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/go-cmd/cmd"
	"github.com/xtfly/gofd/common"
)

type deployInfo struct {
	serverAddr string
	gid        int
	version    int
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
func (r *deployer) DoDeploy(serverAddr string, gid int, version int, files []*FileDict) {
	r.deployChan <- &deployInfo{serverAddr: serverAddr, gid: gid, version: version, Files: files}
}

func (r *deployer) Close() {
	close(r.deployChan)
}

func (r *deployer) deployImp(ri *deployInfo) {
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
	common.LOG.Errorf("[%s] Deploy Done, gid=%v, version=%v.", r.taskID, ri.gid, ri.version)
	// TODO: 增加解压及创建目录的方法
	// 1. 检查指定目录是否存在，存在就判断是否是 subvolume, 不存在就创建 （stat -f --format=%T /path）
	// 2. 解压归档文件到指定目录

	path := fmt.Sprintf("/opt/area_game_data/%v_%v", ri.gid, ri.version)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		// path/to/whatever does not exist
		common.LOG.Errorf("[%s] %v_%v not exists.", r.taskID, ri.gid, ri.version)
	} else {
		// 判断是否 btrfs subvolume
		if path_type, err := r.isBtrfsSubvolum(path); err != nil {
			common.LOG.Error("error:", err)
		} else {
			common.LOG.Error("path_type:", path_type)
			common.LOG.Error(strings.Compare(path_type, "\"btrfs\""))
		}
	}

	return
}

// isBtrfsSubvolum call change model command
func (r *deployer) isBtrfsSubvolum(path string) (string, error) {
	common.LOG.Error("Stat File:", path)
	result := ""
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		// path/to/whatever exists
		createCmd := cmd.NewCmd("stat", "-f", "--format=\"%T\"", path)
		stateChan := createCmd.Start()
		finalStatus := <-stateChan
		if finalStatus.Error != nil {
			common.LOG.Error(finalStatus.Error)
			return "", finalStatus.Error
		}

		n := len(finalStatus.Stdout)
		common.LOG.Error("n:", n)
		if n > 1 {
			result = strings.TrimRight(finalStatus.Stdout[n-1], "\n")
		} else {
			result = strings.TrimRight(finalStatus.Stdout[0], "\n")
		}

	} else {
		return "", err
	}

	return result, nil
}
