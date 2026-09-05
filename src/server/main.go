package main

import (
	"fmt"
	"os"

	"github.com/kwhitestone/prism-fusion/core"

	// 导入插件包，触发所有插件的 init() 自动注册
	_ "github.com/kwhitestone/prism-fusion/addons"
)

//go:generate go env -w GO111MODULE=on
//go:generate go mod tidy
//go:generate go mod download

func main() {
	if err := core.RunApplication(core.ApplicationOptions{}); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
