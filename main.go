package main

import (
	"fmt"
	"os"

	"agentOpenai/tui"
	tea "charm.land/bubbletea/v2"
	"github.com/joho/godotenv"
)

func main() {
	// 必须先加载配置，再创建 AI 客户端。
	if err := godotenv.Load(); err != nil {
		fmt.Println("加载 .env 失败:", err)
		return
	}

	model := tui.NewModel()
	p := tea.NewProgram(model)
	_, runErr := p.Run()

	// 界面退出后再关闭 MCP。
	if err := model.Close(); err != nil {
		fmt.Println("关闭 MCP 失败:", err)
	}
	if runErr != nil {
		fmt.Println("程序退出:", runErr)
		os.Exit(1)
	}
}
