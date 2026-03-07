package core

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"whitestone.top/prism-fusion/global"
)

func initServer(address string, router *gin.Engine, readTimeout, writeTimeout time.Duration) {
	s := &http.Server{
		Addr:           address,
		Handler:        router,
		ReadTimeout:    readTimeout,
		WriteTimeout:   writeTimeout,
		MaxHeaderBytes: 1 << 20,
	}

	// 启动服务器
	go func() {
		if err := s.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			global.PRISM_LOG.Fatal("listen: ", zap.Error(err))
		}
	}()

	// 等待中断信号来优雅地关闭服务器
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	global.PRISM_LOG.Info("Shutdown Server ...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := s.Shutdown(ctx); err != nil {
		global.PRISM_LOG.Fatal("Server Shutdown: ", zap.Error(err))
	}

	global.PRISM_LOG.Info("Server exiting")
}
