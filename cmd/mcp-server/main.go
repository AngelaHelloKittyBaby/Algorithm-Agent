package main

import (
	"agentOpenai/tools"
	"context"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"log"
)

// GetProblemInput 定义模型调用读题工具时需要提供的参数。
type GetProblemInput struct {
	Reference string `json:"reference" jsonschema:"题号或题目链接，例如 P1001、4A"`
}

// SearchProblemsInput 定义搜题参数。
type SearchProblemsInput struct {
	Tag      string `json:"tag" jsonschema:"算法标签或搜索关键词"`
	Limit    int    `json:"limit,omitempty" jsonschema:"返回数量，默认 5"`
	Platform string `json:"platform,omitempty" jsonschema:"平台：codeforces 或 luogu，默认 codeforces"`
}

// SearchProblemsOutput 定义搜题结果。
type SearchProblemsOutput struct {
	Problems []tools.Problem `json:"problems"`
}

// SearchKnowledgeInput 定义联网搜索参数。
type SearchKnowledgeInput struct {
	Query string `json:"query" jsonschema:"需要联网搜索的问题或关键词"`
	Limit int    `json:"limit,omitempty" jsonschema:"返回数量，默认 5，最多 10"`
}

// SearchKnowledgeOutput 定义联网搜索结果。
type SearchKnowledgeOutput struct {
	Results []tools.KnowledgeHit `json:"results"`
}

func main() {
	// 创建 MCP 服务，名称和版本用于标识这个服务。
	server := mcp.NewServer(
		&mcp.Implementation{
			Name:    "algorithm-tools",
			Version: "1.0.0",
		},
		nil,
	)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_problem",
		Description: "读取洛谷或 Codeforces 题面、输入输出要求、限制和样例",
	}, func(
		ctx context.Context,
		req *mcp.CallToolRequest,
		input GetProblemInput,
	) (*mcp.CallToolResult, tools.ProblemDetail, error) {
		problem, err := tools.GetProblem(ctx, input.Reference)
		return nil, problem, err
	})
	// 注册搜题工具。
	mcp.AddTool(server, &mcp.Tool{
		Name:        "search_problems",
		Description: "按标签搜索 Codeforces 题目，或按关键词搜索洛谷题目",
	}, func(
		ctx context.Context,
		req *mcp.CallToolRequest,
		input SearchProblemsInput,
	) (*mcp.CallToolResult, SearchProblemsOutput, error) {
		problems, err := tools.SearchProblems(
			ctx, input.Tag, input.Limit, input.Platform,
		)
		return nil, SearchProblemsOutput{Problems: problems}, err
	})

	// 注册联网搜索工具。
	mcp.AddTool(server, &mcp.Tool{
		Name:        "search_knowledge",
		Description: "联网搜索算法资料，返回标题、来源链接和摘要",
	}, // SearchKnowledge 处理 MCP 联网搜索请求。
		func(
			ctx context.Context,
			req *mcp.CallToolRequest,
			input SearchKnowledgeInput,
		) (*mcp.CallToolResult, SearchKnowledgeOutput, error) {
			results, err := tools.SearchKnowledge(ctx, input.Query, input.Limit)
			return nil, SearchKnowledgeOutput{Results: results}, err
		})

	//通过标准输入输出接收 MCP 客户端的请求。
	if err := server.Run(context.Background(), &mcp.StdioTransport{}); err != nil {
		log.Fatal(err)
	}
}
