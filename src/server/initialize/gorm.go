package initialize

import (
	"database/sql"

	"github.com/kwhitestone/prism-fusion/global"

	"gorm.io/driver/mysql"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	_ "modernc.org/sqlite" // 纯Go SQLite驱动
)

// GormMysql 初始化MySQL数据库
func GormMysql() *gorm.DB {
	m := global.PRISM_CONFIG.Mysql
	if m.Host == "" || m.Dbname == "" {
		return nil
	}

	dsn := m.Dsn()
	config := &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	}

	if db, err := gorm.Open(mysql.Open(dsn), config); err != nil {
		global.PRISM_LOG.Error("Failed to connect to MySQL database: " + err.Error())
		return nil
	} else {
		sqlDB, _ := db.DB()
		sqlDB.SetMaxIdleConns(m.MaxIdleConns)
		sqlDB.SetMaxOpenConns(m.MaxOpenConns)
		global.PRISM_LOG.Info("Database connected: MySQL, host: " + m.Host + ":" + m.Port + ", database: " + m.Dbname)
		return db
	}
}

// GormSqlite 初始化SQLite数据库
func GormSqlite() *gorm.DB {
	s := global.PRISM_CONFIG.Sqlite
	if s.Path == "" {
		return nil
	}

	// 使用纯Go的SQLite驱动
	sqlDB, err := sql.Open("sqlite", s.Path)
	if err != nil {
		global.PRISM_LOG.Error("Failed to open SQLite database: " + err.Error())
		return nil
	}

	config := &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	}

	if db, err := gorm.Open(sqlite.Dialector{Conn: sqlDB}, config); err != nil {
		global.PRISM_LOG.Error("Failed to connect to SQLite database: " + err.Error())
		return nil
	} else {
		sqlDB.SetMaxIdleConns(s.MaxIdleConns)
		sqlDB.SetMaxOpenConns(s.MaxOpenConns)
		global.PRISM_LOG.Info("Database connected: SQLite, path: " + s.Path)
		return db
	}
}

// Gorm 初始化数据库并产生数据库全局变量
// 优先使用MySQL，如果MySQL未配置则使用SQLite
func Gorm() *gorm.DB {
	// 优先尝试MySQL
	if db := GormMysql(); db != nil {
		return db
	}
	// 回退到SQLite
	return GormSqlite()
}
