package global

import (
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/spf13/viper"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"whitestone.top/prism-fusion/config"
)

var (
	PRISM_DB      *gorm.DB
	PRISM_CONFIG  config.Server
	PRISM_VP      *viper.Viper
	PRISM_LOG     *zap.Logger
	PRISM_ROUTERS gin.RoutesInfo
	lock        sync.RWMutex
)
