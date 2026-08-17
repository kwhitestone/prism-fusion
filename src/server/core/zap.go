package core

import (
	"fmt"
	"os"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"github.com/kwhitestone/prism-fusion/global"
	"github.com/kwhitestone/prism-fusion/utils"
)

// Zap 获取 zap.Logger
func Zap() (logger *zap.Logger) {
	if ok, _ := utils.PathExists(global.PRISM_CONFIG.Zap.Director); !ok { // 判断是否有Director文件夹
		fmt.Printf("create %v directory\n", global.PRISM_CONFIG.Zap.Director)
		_ = os.Mkdir(global.PRISM_CONFIG.Zap.Director, os.ModePerm)
	}

	cores := getZapCores()
	logger = zap.New(zapcore.NewTee(cores...))

	if global.PRISM_CONFIG.Zap.ShowLine {
		logger = logger.WithOptions(zap.AddCaller())
	}
	return logger
}

// getZapCores 根据配置文件的Level获取 []zapcore.Core
func getZapCores() []zapcore.Core {
	cores := make([]zapcore.Core, 0, 7)
	for level := global.PRISM_CONFIG.Zap.TransportLevel(); level <= zapcore.FatalLevel; level++ {
		cores = append(cores, getZapCore(level, getZapLogWriter(level)))
	}
	return cores
}

// getZapCore 获取 zapcore.Core
func getZapCore(level zapcore.Level, ws zapcore.WriteSyncer) zapcore.Core {
	var encoder zapcore.Encoder
	if global.PRISM_CONFIG.Zap.Format == "json" {
		encoder = getJsonEncoder()
	} else {
		encoder = getConsoleEncoder()
	}
	return zapcore.NewCore(encoder, ws, level)
}

// getConsoleEncoder 获取zapcore.Encoder
func getConsoleEncoder() zapcore.Encoder {
	if global.PRISM_CONFIG.Zap.EncodeLevel == "LowercaseColorLevelEncoder" { // 自定义日志级别显示
		global.PRISM_CONFIG.Zap.EncodeLevel = "LowercaseLevelEncoder"
	}

	return zapcore.NewConsoleEncoder(getEncoderConfig())
}

// getJsonEncoder 获取zapcore.Encoder
func getJsonEncoder() zapcore.Encoder {
	return zapcore.NewJSONEncoder(getEncoderConfig())
}

// getEncoderConfig 获取zapcore.EncoderConfig
func getEncoderConfig() zapcore.EncoderConfig {
	config := zapcore.EncoderConfig{
		MessageKey:     "message",
		LevelKey:       "level",
		TimeKey:        "time",
		NameKey:        "logger",
		CallerKey:      "caller",
		StacktraceKey:  global.PRISM_CONFIG.Zap.StacktraceKey,
		LineEnding:     zapcore.DefaultLineEnding,
		EncodeLevel:    zapcore.LowercaseLevelEncoder,
		EncodeTime:     zapcore.TimeEncoderOfLayout("2006-01-02 15:04:05.000"),
		EncodeDuration: zapcore.SecondsDurationEncoder,
		EncodeCaller:   zapcore.FullCallerEncoder,
	}
	switch {
	case global.PRISM_CONFIG.Zap.EncodeLevel == "LowercaseLevelEncoder": // 小写编码器(默认)
		config.EncodeLevel = zapcore.LowercaseLevelEncoder
	case global.PRISM_CONFIG.Zap.EncodeLevel == "LowercaseColorLevelEncoder": // 小写编码器带颜色
		config.EncodeLevel = zapcore.LowercaseColorLevelEncoder
	case global.PRISM_CONFIG.Zap.EncodeLevel == "CapitalLevelEncoder": // 大写编码器
		config.EncodeLevel = zapcore.CapitalLevelEncoder
	case global.PRISM_CONFIG.Zap.EncodeLevel == "CapitalColorLevelEncoder": // 大写编码器带颜色
		config.EncodeLevel = zapcore.CapitalColorLevelEncoder
	default:
		config.EncodeLevel = zapcore.LowercaseLevelEncoder
	}
	return config
}

// getZapLogWriter 获取zapcore.WriteSyncer
func getZapLogWriter(level zapcore.Level) zapcore.WriteSyncer {
	fileWriter := utils.GetWriteSyncer(level.String())
	if global.PRISM_CONFIG.Zap.LogInConsole {
		return zapcore.NewMultiWriteSyncer(zapcore.AddSync(os.Stdout), fileWriter)
	}
	return fileWriter
}
