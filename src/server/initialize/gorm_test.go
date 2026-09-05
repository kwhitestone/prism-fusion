package initialize

import (
	"path/filepath"
	"testing"

	"github.com/kwhitestone/prism-fusion/global"
	"go.uber.org/zap"
)

func TestGormFailsClosedForUnavailableSQLite(t *testing.T) {
	previousConfig, previousLogger := global.PRISM_CONFIG, global.PRISM_LOG
	global.PRISM_LOG = zap.NewNop()
	global.PRISM_CONFIG.Mysql.Host = ""
	global.PRISM_CONFIG.Sqlite.Path = filepath.Join(t.TempDir(), "missing", "db.sqlite")
	t.Cleanup(func() { global.PRISM_CONFIG, global.PRISM_LOG = previousConfig, previousLogger })
	defer func() {
		if recover() == nil {
			t.Fatal("selected SQLite failure was silently ignored")
		}
	}()
	_ = Gorm()
}

func TestGormFailsClosedForInvalidMySQLConfiguration(t *testing.T) {
	previousConfig := global.PRISM_CONFIG
	previousLogger := global.PRISM_LOG
	global.PRISM_LOG = zap.NewNop()
	global.PRISM_CONFIG.Mysql.Host = "mysql.example.invalid"
	global.PRISM_CONFIG.Mysql.Dbname = ""
	global.PRISM_CONFIG.Sqlite.Path = "file:must-not-fallback?mode=memory&cache=shared"
	t.Cleanup(func() {
		global.PRISM_CONFIG = previousConfig
		global.PRISM_LOG = previousLogger
	})

	defer func() {
		if recover() == nil {
			t.Fatal("Gorm() did not fail closed for configured but invalid MySQL")
		}
	}()
	_ = Gorm()
}

func TestGormUsesSQLiteOnlyWhenMySQLIsNotSelected(t *testing.T) {
	previousConfig := global.PRISM_CONFIG
	previousLogger := global.PRISM_LOG
	global.PRISM_LOG = zap.NewNop()
	global.PRISM_CONFIG.Mysql.Host = ""
	global.PRISM_CONFIG.Sqlite.Path = "file:explicit-sqlite?mode=memory&cache=shared"
	global.PRISM_CONFIG.Sqlite.MaxIdleConns = 1
	global.PRISM_CONFIG.Sqlite.MaxOpenConns = 1
	t.Cleanup(func() {
		global.PRISM_CONFIG = previousConfig
		global.PRISM_LOG = previousLogger
	})

	db := Gorm()
	if db == nil {
		t.Fatal("Gorm() returned nil for explicit SQLite configuration")
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("resolve sql database: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
}
