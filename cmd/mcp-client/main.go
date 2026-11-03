package main

import (
	"context"
	"log"
	"os/exec"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func main() {
	ctx := context.Background()

	// 创建 MCP 客户端。
	client := mcp.NewClient(&mcp.Implementation{
		Name:    "algorithm-client",
		Version: "1.0.0",
	}, nil)

	// 启动之前编译的服务，通过标准输入输出通信。
	transport := &mcp.CommandTransport{
		Command: exec.Command("./algorithm-mcp.exe"),
	}

	// 与服务建立连接。
	session, err := client.Connect(ctx, transport, nil)
	if err != nil {
		log.Fatal(err)
	}
	defer session.Close()
	// 构造工具名称和调用参数。
	params := &mcp.CallToolParams{
		Name: "get_problem",
		Arguments: map[string]any{
			"reference": "P1001",
		},
	}

	// 通过 MCP 会话调用服务端工具。
	result, err := session.CallTool(ctx, params)
	if err != nil {
		log.Fatalf("调用工具失败：%v", err)
	}

	// 检查工具执行是否失败。
	if result.IsError {
		log.Fatalf("工具执行失败：%v", result.Content)
	}

	// 输出工具返回的文本内容。
	for _, content := range result.Content {
		if text, ok := content.(*mcp.TextContent); ok {
			log.Println(text.Text)
		}
	}
}
