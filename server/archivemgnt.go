package server

import (
	"fmt"
	"os"
	"time"

	"github.com/labstack/gommon/log"
	"github.com/xtfly/gofd/common"
	"github.com/xtfly/gokits/gcache"
)

type cmpArchive struct {
	t   *StartArchive
	out chan bool
}

type queryArchive struct {
	out chan *ArchiveInfo
}

// CachedArchiveInfo 每一次 Archive ，对应一个缓存对象，所有与它关联的操作都由一个 Goroutine 来处理
type CachedArchiveInfo struct {
	s *Server

	id             string
	archiveDirPath string
	destFilePath   string
	ti             *ArchiveInfo

	succCount int
	failCount int
	allCount  int

	stopChan chan struct{}
	quitChan chan struct{}
	//reportChan   chan *p2p.StatusReport
	//agentRspChan chan *clientRsp
	cmpChan   chan *cmpArchive
	queryChan chan *queryArchive
}

// NewCachedArchiveInfo ...
func NewCachedArchiveInfo(s *Server, t *StartArchive) *CachedArchiveInfo {
	return &CachedArchiveInfo{
		s:              s,
		id:             t.ID,
		archiveDirPath: t.ArchiveDirPath,
		destFilePath:   t.DestFilePath,
		ti:             newArchiveInfo(t),

		stopChan: make(chan struct{}),
		quitChan: make(chan struct{}),
		//reportChan:   make(chan *p2p.StatusReport, 10),
		//agentRspChan: make(chan *clientRsp, 10),
		cmpChan:   make(chan *cmpArchive, 2),
		queryChan: make(chan *queryArchive, 2),
	}
}

// EqualCmp ...
func (ct *CachedArchiveInfo) EqualCmp(t *StartArchive) bool {
	cchan := make(chan bool, 2)
	ct.cmpChan <- &cmpArchive{t: t, out: cchan}
	defer close(cchan)
	return <-cchan
}

func newArchiveInfo(t *StartArchive) *ArchiveInfo {
	init := TaskInit.String()
	ti := &ArchiveInfo{ID: t.ID, Status: init, StartedAt: time.Now()}
	return ti
}

func (ct *CachedArchiveInfo) endArchive(ts TaskStatus) {
	log.Errorf("[%s] Archive status changed, status=%v", ct.id, ts)
	ct.ti.Status = ts.String()
	ct.ti.FinishedAt = time.Now()
	log.Infof("[%s] Archive elapsed time: (%.2f seconds)", ct.id, ct.ti.FinishedAt.Sub(ct.ti.StartedAt).Seconds())
	_ = ct.s.cache.Replace(ct.id, ct, 5*time.Minute)
	//ct.s.sessionMgnt.StopTask(ct.id)
}

// Start 使用一个 Goroutine 来启动任务操作
func (ct *CachedArchiveInfo) Start() {
	if ts := ct.createTask(); ts != TaskInProgress {
		ct.endArchive(ts)
	}

	for {
		select {
		case <-ct.quitChan:
			log.Infof("[%s] Quit archive goroutine", ct.id)
			return
		case <-ct.stopChan:
			ct.endArchive(TaskFailed)
		case c := <-ct.cmpChan:
			// 内容不相同
			if c.t.ArchiveDirPath != ct.archiveDirPath || c.t.DestFilePath != ct.destFilePath {
				c.out <- false
			}
			// 内容相同, 如果失败了, 则重新启动
			c.out <- true
			if ct.ti.Status == TaskFailed.String() {
				_ = ct.s.cache.Replace(ct.id, ct, gcache.NoExpiration)
				log.Infof("[%s] Archive status is FAILED, will start task try again", ct.id)
				if ts := ct.createTask(); ts != TaskInProgress {
					ct.endArchive(ts)
				}
			}
		case q := <-ct.queryChan:
			q.out <- ct.ti
		}
	}
}

func (ct *CachedArchiveInfo) createTask() TaskStatus {
	// 先产生任务元数据信息
	start := time.Now()
	log.Infof("[%s] archive info: archiveDirPath: %s, destFilePath: %s", ct.id, ct.archiveDirPath, ct.destFilePath)
	// 目标文件如果存在也强制创建
	if _, err := os.Stat(ct.archiveDirPath); !os.IsNotExist(err) {
		common.TarGz(ct.archiveDirPath, ct.destFilePath) // 压缩
	} else {
		fmt.Printf("Error, File/Dir not exists.")
		return TaskFileNotExist
	}

	end := time.Now()
	log.Infof("[%s] Start archive: (%.2f seconds)", ct.id, end.Sub(start).Seconds())

	ct.allCount = 1
	ct.succCount, ct.failCount = 1, 0
	ct.ti.Status = TaskInProgress.String()
	return TaskCompleted
}

// Query ...
func (ct *CachedArchiveInfo) Query() *ArchiveInfo {
	qchan := make(chan *ArchiveInfo, 2)
	ct.queryChan <- &queryArchive{out: qchan}
	defer close(qchan)
	return <-qchan
}
