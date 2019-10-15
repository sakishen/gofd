package p2p

import (
	"encoding/json"
	"net/http"

	"github.com/xtfly/gofd/common"
)

type reportInfo struct {
	serverAddr      string
	gid             int
	version         int
	percentComplete float32
}

type reporter struct {
	taskID string
	cfg    *common.Config
	client *http.Client

	reportChan chan *reportInfo
}

func newReporter(taskID string, cfg *common.Config) *reporter {
	r := &reporter{
		taskID:     taskID,
		cfg:        cfg,
		client:     common.CreateHTTPClient(cfg),
		reportChan: make(chan *reportInfo, 20),
	}

	go r.run()
	return r
}

func (r *reporter) run() {
	for rc := range r.reportChan {
		r.reportImp(rc)
	}
}

func (r *reporter) DoReport(serverAddr string, gid int, version int, pecent float32) {
	r.reportChan <- &reportInfo{serverAddr: serverAddr, gid: gid, version: version, percentComplete: pecent}
}

func (r *reporter) Close() {
	close(r.reportChan)
}

func (r *reporter) reportImp(ri *reportInfo) {
	if int(ri.percentComplete) == 100 {
		common.LOG.Infof("[%s] Report session status... completed", r.taskID)
	}
	csr := &StatusReport{
		TaskID: r.taskID,
		//IP:              r.cfg.Net.IP,
		IP:              r.cfg.Net.Host, // 上报进度使用
		GID:             ri.gid,         // 上报游戏 ID
		Version:         ri.version,     // 上报游戏版本
		PercentComplete: ri.percentComplete,
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
	}
	return
}
