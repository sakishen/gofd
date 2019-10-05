package server

import (
	"encoding/json"
	"fmt"
	"github.com/labstack/gommon/log"
	"github.com/xtfly/gofd/common"
	"github.com/xtfly/gofd/p2p"
	"github.com/xtfly/gokits/gcache"
	"strings"
	"time"
)

type cmpDeployTask struct {
	t   *CreateDeployTask
	out chan bool
}

type queryDeployTask struct {
	out chan *DeployTaskInfo
}

// CachedTaskInfo 每一个Task，对应一个缓存对象，所有与它关联的操作都由一个Goroutine来处理
type CachedDeployTaskInfo struct {
	s *Server

	id            string
	dispatchFiles []string
	destIPs       []string
	ti            *DeployTaskInfo

	succCount int
	failCount int
	allCount  int

	stopChan     chan struct{}
	quitChan     chan struct{}
	reportChan   chan *p2p.StatusReport
	agentRspChan chan *clientRsp
	cmpChan      chan *cmpDeployTask
	queryChan    chan *queryDeployTask
}

// NewCachedTaskInfo ...
func NewCachedDeployTaskInfo(s *Server, t *CreateDeployTask) *CachedDeployTaskInfo {
	return &CachedDeployTaskInfo{
		s:             s,
		id:            t.ID,
		dispatchFiles: t.DispatchFiles,
		destIPs:       t.DestIPs,
		ti:            newDeployTaskInfo(t),

		stopChan:     make(chan struct{}),
		quitChan:     make(chan struct{}),
		reportChan:   make(chan *p2p.StatusReport, 10),
		agentRspChan: make(chan *clientRsp, 10),
		cmpChan:      make(chan *cmpDeployTask, 2),
		queryChan:    make(chan *queryDeployTask, 2),
	}
}

func newDeployTaskInfo(t *CreateDeployTask) *DeployTaskInfo {
	init := TaskInit.String()
	ti := &DeployTaskInfo{ID: t.ID, Status: init, StartedAt: time.Now()}
	ti.DispatchInfos = make(map[string]*DispatchInfo, len(t.DestIPs))
	for _, ip := range t.DestIPs {
		di := &DispatchInfo{Status: init, StartedAt: time.Now()}
		di.DispatchFiles = make([]*DispatchFile, len(t.DispatchFiles))
		ti.DispatchInfos[ip] = di
		for j, fn := range t.DispatchFiles {
			di.DispatchFiles[j] = &DispatchFile{FileName: fn}
		}
	}
	return ti
}

// Start 使用一个 Goroutine 来启动任务操作
func (ct *CachedDeployTaskInfo) Start() {
	if ts := ct.createDeployTask(); ts != TaskInProgress {
		ct.endDeployTask(ts)
	}

	for {
		select {
		case <-ct.quitChan:
			log.Infof("[%s] Quit task goroutine", ct.id)
			return
		case <-ct.stopChan:
			ct.endDeployTask(TaskFailed)
			ct.stopAllClientDeployTask()
		case c := <-ct.cmpChan:
			// 内容不相同
			if !equalSlice(c.t.DestIPs, ct.destIPs) || !equalSlice(c.t.DispatchFiles, ct.dispatchFiles) {
				c.out <- false
			}
			// 内容相同，如果失败了，则重新启动
			c.out <- true
			if ct.ti.Status == TaskFailed.String() {
				_ = ct.s.cache.Replace(ct.id, ct, gcache.NoExpiration)
				log.Infof("[%s] Task status is FAILED, will start task try again", ct.id)
				if ts := ct.createDeployTask(); ts != TaskInProgress {
					ct.endDeployTask(ts)
				}
			}
		case q := <-ct.queryChan:
			q.out <- ct.ti
		case csr := <-ct.reportChan:
			ct.reportDeployStatus(csr)
			if ts, ok := checkDeployFinished(ct.ti); ok {
				ct.endDeployTask(ts)
				ct.stopAllClientDeployTask()
			}
		}
	}
}

func (ct *CachedDeployTaskInfo) endDeployTask(ts TaskStatus) {
	log.Errorf("[%s] Task status changed, status=%v", ct.id, ts)
	ct.ti.Status = ts.String()
	ct.ti.FinishedAt = time.Now()
	log.Infof("[%s] Task elapsed time: (%.2f seconds)", ct.id, ct.ti.FinishedAt.Sub(ct.ti.StartedAt).Seconds())
	_ = ct.s.cache.Replace(ct.id, ct, 5*time.Minute)
	ct.s.sessionMgnt.StopTask(ct.id)
}

func (ct *CachedDeployTaskInfo) createDeployTask() TaskStatus {
	// 先产生任务元数据信息
	start := time.Now()
	mi, err := p2p.CreateFileMeta(ct.dispatchFiles, FixedBlockLen) // 块大小
	end := time.Now()
	if err != nil {
		log.Errorf("[%s] Create file meta failed, error=%v", ct.id, err)
		return TaskFileNotExist
	}
	log.Infof("[%s] Create metainfo: (%.2f seconds)", ct.id, end.Sub(start).Seconds())

	dt := &p2p.DispatchTask{
		TaskID:   ct.id,
		MetaInfo: mi,
		Speed:    int64(ct.s.Cfg.Control.Speed * FixedBlockLen),
	}
	dt.LinkChain = createDeployLinkChain(ct.s.Cfg, []string{}, ct.ti) //

	dtbytes, err1 := json.Marshal(dt)
	if err1 != nil {
		return TaskFailed
	}
	log.Debugf("[%s] Create dispatch task, task=%v", ct.id, string(dtbytes))

	ct.allCount = len(ct.destIPs)
	ct.succCount, ct.failCount = 0, 0
	ct.ti.Status = TaskInProgress.String()
	// 提交到session管理中运行
	ct.s.sessionMgnt.CreateTask(dt)
	// 给各节点发送创建分发任务的Rest消息
	ct.sendDeployReqToClients(ct.destIPs, "/api/v1/agent/tasks", dtbytes)

	for {
		select {
		case tdr := <-ct.agentRspChan:
			ct.checkAgentRsp(tdr)
			if ct.failCount == ct.allCount {
				return TaskFailed
			}
			if ct.succCount+ct.failCount == ct.allCount {
				if ts := ct.startDeployTask(); ts != TaskInProgress {
					return ts
				}
				// 部分节点响应，则也继续
				return TaskInProgress
			}
		case <-time.After(5 * time.Second): // 等超时
			if ct.succCount == 0 {
				common.LOG.Errorf("[%s] Wait client response timeout.", ct.id)
				return TaskFailed
			}
		}
	}
}


func (ct *CachedDeployTaskInfo) checkAgentRsp(tcr *clientRsp) {
	if di, ok := ct.ti.DispatchInfos[tcr.IP]; ok {
		di.StartedAt = time.Now()
		if tcr.Success {
			di.Status = TaskInProgress.String()
			ct.succCount++
		} else {
			di.Status = TaskFailed.String()
			di.FinishedAt = time.Now()
			ct.failCount++
		}
	}
}

// Query ...
func (ct *CachedDeployTaskInfo) Query() *DeployTaskInfo {
	qchan := make(chan *DeployTaskInfo, 2)
	ct.queryChan <- &queryDeployTask{out: qchan}
	defer close(qchan)
	return <-qchan
}

func createDeployLinkChain(cfg *common.Config, ips []string, ti *DeployTaskInfo) *p2p.LinkChain {
	lc := new(p2p.LinkChain)
	lc.ServerAddr = fmt.Sprintf("%s:%v", cfg.Net.Host, cfg.Net.MgntPort)
	//lc.ServerAddr = fmt.Sprintf("%s:%v", "10.86.1.21", cfg.Net.MgntPort)
	lc.DispatchAddrs = make([]string, 1+len(ips))
	// 第一个节点为服务端
	lc.DispatchAddrs[0] = fmt.Sprintf("%s:%v", cfg.Net.Host, cfg.Net.DataPort)
	//lc.DispatchAddrs[0] = fmt.Sprintf("%s:%v", "10.86.1.21", cfg.Net.DataPort)

	idx := 1
	for _, ip := range ips {
		if di, ok := ti.DispatchInfos[ip]; ok && di.Status == TaskInProgress.String() {
			lc.DispatchAddrs[idx] = fmt.Sprintf("%s:%v", ip, cfg.Net.AgentDataPort)
			idx++
		}
	}
	lc.DispatchAddrs = lc.DispatchAddrs[:idx]

	return lc
}

func (ct *CachedDeployTaskInfo) startDeployTask() TaskStatus {
	log.Infof("[%s] Recv all client response, will send start command to clients", ct.id)
	st := &p2p.StartTask{TaskID: ct.id}
	st.LinkChain = createDeployLinkChain(ct.s.Cfg, ct.destIPs, ct.ti)

	stbytes, err1 := json.Marshal(st)
	if err1 != nil {
		return TaskFailed
	}
	log.Debugf("[%s] Create start task, task=%v", ct.id, string(stbytes))

	// 第一个是Server，不用发送启动
	ct.allCount = len(st.LinkChain.DispatchAddrs) - 1
	ct.succCount, ct.failCount = 0, 0
	ct.s.sessionMgnt.StartTask(st)

	// 给其它各节点发送启支分发任务的Rest消息
	ct.sendDeployReqToClients(st.LinkChain.DispatchAddrs[1:], "/api/v1/agent/tasks/start", stbytes)
	for {
		select {
		case tdr := <-ct.agentRspChan:
			ct.checkAgentRsp(tdr)
			if ct.failCount == ct.allCount {
				return TaskFailed
			}
			if ct.succCount+ct.failCount == ct.allCount {
				return TaskInProgress
			}
		case <-time.After(5 * time.Second): // 等超时
			if ct.succCount == 0 {
				log.Errorf("[%s] Wait client response timeout.", ct.id)
				return TaskFailed
			}
		}
	}
}

func (ct *CachedDeployTaskInfo) sendDeployReqToClients(ips []string, url string, body []byte) {
	for _, ip := range ips {
		if idx := strings.Index(ip, ":"); idx > 0 {
			ip = ip[:idx]
		}

		go func(ip string) {
			if _, err2 := ct.s.HTTPPost(ip, url, body); err2 != nil {
				log.Errorf("[%s] Send http request failed. POST, ip=%s, url=%s, error=%v", ct.id, ip, url, err2)
				ct.agentRspChan <- &clientRsp{IP: ip, Success: false}
			} else {
				log.Debugf("[%s] Send http request success. POST, ip=%s, url=%s", ct.id, ip, url)
				ct.agentRspChan <- &clientRsp{IP: ip, Success: true}
			}
		}(ip)
	}
}

// 给所有客户端发送停止命令
func (ct *CachedDeployTaskInfo) stopAllClientDeployTask() {
	url := "/api/v1/agent/tasks/" + ct.id
	ct.s.sessionMgnt.StopTask(ct.id)
	for _, ip := range ct.destIPs {
		go func(ip string) {
			if err2 := ct.s.HTTPDelete(ip, url); err2 != nil {
				log.Errorf("[%s] Send http request failed. DELETE, ip=%s, url=%s, error=%v", ct.id, ip, url, err2)
			} else {
				log.Debugf("[%s] Send http request success. DELETE, ip=%s, url=%s", ct.id, ip, url)
			}
		}(ip)
	}
}

func (ct *CachedTaskInfo) reportDeployStatus(csr *p2p.StatusReport) {
	if di, ok := ct.ti.DispatchInfos[csr.IP]; ok {
		if int(csr.PercentComplete) == 100 {
			di.Status = TaskCompleted.String()
			di.FinishedAt = time.Now()
			log.Infof("[%s] Recv report task status is completed, ip=%s", ct.id, csr.IP)
		} else if int(csr.PercentComplete) == -1 {
			di.Status = TaskFailed.String()
			di.FinishedAt = time.Now()
			log.Infof("[%s] Recv report task status is failed, ip=%s", ct.id, csr.IP)
		}
		di.PercentComplete = csr.PercentComplete
	}
}

// Query ...
func (ct *CachedDeployTaskInfo) QueryDeploy() *DeployTaskInfo {
	qchan := make(chan *DeployTaskInfo, 2)
	ct.queryChan <- &queryDeployTask{out: qchan}
	defer close(qchan)
	return <-qchan
}

// EqualCmp ...
func (ct *CachedDeployTaskInfo) EqualDeployCmp(t *CreateDeployTask) bool {
	cchan := make(chan bool, 2)
	ct.cmpChan <- &cmpDeployTask{t: t, out: cchan}
	defer close(cchan)
	return <-cchan
}

func (ct *CachedDeployTaskInfo) reportDeployStatus(csr *p2p.StatusReport) {
	if di, ok := ct.ti.DispatchInfos[csr.IP]; ok {
		if int(csr.PercentComplete) == 100 {
			di.Status = TaskCompleted.String()
			di.FinishedAt = time.Now()
			log.Infof("[%s] Recv report task status is completed, ip=%s", ct.id, csr.IP)
		} else if int(csr.PercentComplete) == -1 {
			di.Status = TaskFailed.String()
			di.FinishedAt = time.Now()
			log.Infof("[%s] Recv report task status is failed, ip=%s", ct.id, csr.IP)
		}
		di.PercentComplete = csr.PercentComplete
	}
}

func checkDeployFinished(ti *DeployTaskInfo) (TaskStatus, bool) {
	completed := 0
	failed := 0
	for _, v := range ti.DispatchInfos {
		if v.Status == TaskCompleted.String() {
			completed++
		}
		if v.Status == TaskFailed.String() {
			failed++
		}
	}

	count := len(ti.DispatchInfos)
	if completed == count {
		return TaskCompleted, true
	}

	if completed+failed == count {
		return TaskCompleted, true
	}

	return TaskInProgress, false
}


