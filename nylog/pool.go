package nylog

import (
	"io"
	"sync"

	"gopkg.in/natefinch/lumberjack.v2"
)

var globalWriterMap sync.Map // map[string]*lumberjack.Logger

// getLumberjackWriter 保证相同日志路径下全局只创建一个 lumberjack 指针，防止句柄泄露与并发锁竞争
func getLumberjackWriter(filePath string, conf FileConfig) io.Writer {
	if actual, ok := globalWriterMap.Load(filePath); ok {
		return actual.(*lumberjack.Logger)
	}
	l := &lumberjack.Logger{
		Filename:   filePath,
		MaxSize:    conf.MaxSizeMB,
		MaxBackups: conf.MaxBackups,
		MaxAge:     conf.MaxAgeDays,
		Compress:   conf.Compress,
		LocalTime:  true,
	}
	actual, _ := globalWriterMap.LoadOrStore(filePath, l)
	return actual.(*lumberjack.Logger)
}

// SyncAll 刷盘关机：刷新并归档目前句柄池中所有的文件日志，防止优雅关机时丢失缓冲区尾部日志
func SyncAll() error {
	globalWriterMap.Range(func(key, value any) bool {
		if l, ok := value.(*lumberjack.Logger); ok {
			_ = l.Close()
		}
		return true
	})
	return nil
}
