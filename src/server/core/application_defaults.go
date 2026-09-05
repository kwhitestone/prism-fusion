package core

import (
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"

	"github.com/kwhitestone/prism-fusion/global"
	"github.com/kwhitestone/prism-fusion/initialize"
	"github.com/kwhitestone/prism-fusion/plugin"
	"go.uber.org/zap"
)

func defaultApplicationOperations(options ApplicationOptions) applicationOperations {
	return applicationOperations{
		configure: func() error {
			if options.ConfigPath == "" {
				global.PRISM_VP = Viper()
			} else {
				global.PRISM_VP = Viper(options.ConfigPath)
			}
			global.PRISM_LOG = Zap()
			zap.ReplaceGlobals(global.PRISM_LOG)
			global.PRISM_DB = nil
			return nil
		},
		openDatabase: func() (io.Closer, error) {
			global.PRISM_DB = initialize.Gorm()
			if global.PRISM_DB == nil {
				return nil, errors.New("database is required but no datastore is configured")
			}
			return global.PRISM_DB.DB()
		},
		migrate:   func() error { initialize.InitTables(); return nil },
		router:    func() (http.Handler, error) { return initialize.Routers(), nil },
		address:   func() string { return fmt.Sprintf(":%d", global.PRISM_CONFIG.System.Addr) },
		listen:    net.Listen,
		lifecycle: plugin.NewLifecycle(nil, plugin.LifecycleOptions{StopTimeout: shutdownTimeout(options)}),
	}
}
