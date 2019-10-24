package server

import (
	"net/http"

	"github.com/labstack/gommon/log"

	"github.com/labstack/echo/v4"
	"github.com/xtfly/gofd/common"
	"github.com/xtfly/gofd/p2p"
	"github.com/xtfly/gokits/gcache"
)

// CreateTask POST /api/v1/server/tasks
func (s *Server) CreateTask(c echo.Context) (err error) {
	//  获取Body
	t := new(CreateTask)
	if err = c.Bind(t); err != nil {
		common.LOG.Errorf("Recv [%s] request, decode body failed. %v", c.Request().URL, err)
		return
	}

	// 检查任务是否存在
	v, ok := s.cache.Get(t.ID)
	if ok {
		cti := v.(*CachedTaskInfo)
		if cti.EqualCmp(t) {
			return c.String(http.StatusAccepted, "")
		}
		common.LOG.Debugf("[%s] Recv task, task is existed", t.ID)
		return c.String(http.StatusBadRequest, TaskExist.String())
	}

	common.LOG.Infof("[%s] Recv task, file=%v, ips=%v, fileType=%v, gid=%v, version=%v, storage_dir=%v", t.ID, t.DispatchFiles, t.DestIPs, t.FileType, t.Gid, t.Version, t.StorageDir)

	cti := NewCachedTaskInfo(s, t)
	s.cache.Set(t.ID, cti, gcache.NoExpiration)
	s.cache.OnEvicted(func(id string, v interface{}) {
		common.LOG.Infof("[%s] Remove task cache", t.ID)
		cti := v.(*CachedTaskInfo)
		cti.quitChan <- struct{}{}
	})
	go cti.Start()

	return c.String(http.StatusAccepted, "")
}

// CancelTask DELETE /api/v1/server/tasks/:id
func (s *Server) CancelTask(c echo.Context) error {
	id := c.Param("id")
	common.LOG.Infof("[%s] Recv cancel task", id)
	v, ok := s.cache.Get(id)
	if !ok {
		return c.String(http.StatusBadRequest, TaskNotExist.String())
	}
	cti := v.(*CachedTaskInfo)
	cti.stopChan <- struct{}{}
	return c.JSON(http.StatusAccepted, "")
}

// QueryTask GET /api/v1/server/tasks/:id
func (s *Server) QueryTask(c echo.Context) error {
	id := c.Param("id")
	log.Infof("[%s] Recv query task", id)
	v, ok := s.cache.Get(id)
	if !ok {
		return c.String(http.StatusBadRequest, TaskNotExist.String())
	}
	cti := v.(*CachedTaskInfo)
	return c.JSON(http.StatusOK, cti.Query())

}

// ReportTask POST /api/v1/server/tasks/status
func (s *Server) ReportTask(c echo.Context) (err error) {
	//  获取Body
	csr := new(p2p.StatusReport)
	if err = c.Bind(csr); err != nil {
		common.LOG.Errorf("Recv [%s] request, decode body failed. %v", c.Request().URL, err)
		return
	}

	common.LOG.Debugf("[%s] Recv task report, ip=%v, fileType=%v, gid=%v, version=%v, percent=%v", csr.TaskID, csr.IP, csr.FileType, csr.GID, csr.Version, csr.PercentComplete)
	if v, ok := s.cache.Get(csr.TaskID); ok {
		cti := v.(*CachedTaskInfo)
		cti.reportChan <- csr
	}

	return c.String(http.StatusOK, "")
}

// StartArchive POST /api/v1/server/archive
func (s *Server) StartArchive(c echo.Context) (err error) {
	//  获取Body
	a := new(StartArchive)

	if err = c.Bind(a); err != nil {
		common.LOG.Errorf("Recv [%s] request, decode body failed. %v", c.Request().URL, err)
		return
	}

	// 检查任务是否存在
	v, ok := s.cache.Get(a.ID)
	if ok {
		cai := v.(*CachedArchiveInfo)
		if cai.EqualCmp(a) {
			return c.String(http.StatusAccepted, "")
		}
		common.LOG.Debugf("[%s] Recv archive, archive is existed", a.ID)
		return c.String(http.StatusBadRequest, TaskExist.String())
	}

	common.LOG.Infof("[%s] Recv archive, dir_path=%v", a.ID, a.ArchiveDirPath)

	cai := NewCachedArchiveInfo(s, a)
	s.cache.Set(a.ID, cai, gcache.NoExpiration)
	s.cache.OnEvicted(func(id string, v interface{}) {
		common.LOG.Infof("[%s] Remove archive cache", a.ID)
		cai := v.(*CachedArchiveInfo)
		cai.quitChan <- struct{}{}
	})
	go cai.Start()

	return c.String(http.StatusOK, "")
}

func (s *Server) QueryArchiveTask(c echo.Context) error {
	id := c.Param("id")
	log.Infof("[%s] Recv query archive task", id)
	v, ok := s.cache.Get(id)
	if !ok {
		return c.String(http.StatusBadRequest, TaskNotExist.String())
	}
	cti := v.(*CachedArchiveInfo)
	return c.JSON(http.StatusOK, cti.Query())

}
