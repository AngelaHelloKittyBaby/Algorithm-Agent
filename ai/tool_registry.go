package ai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/shared"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func stringProperty(description string) map[string]any {
	return map[string]any{"type": "string", "description": description}
}
func intProperty(description string, min, max int) map[string]any {
	return map[string]any{"type": "integer", "description": description, "minimum": min, "maximum": max}
}
func toolDefinition(name, description string, properties map[string]any, required ...string) openai.ChatCompletionToolUnionParam {
	if required == nil {
		required = []string{}
	}
	return openai.ChatCompletionFunctionTool(shared.FunctionDefinitionParam{Name: name, Description: openai.String(description), Parameters: shared.FunctionParameters{"type": "object", "properties": properties, "required": required, "additionalProperties": false}})
}

func algorithmTools() []openai.ChatCompletionToolUnionParam {
	return []openai.ChatCompletionToolUnionParam{
		toolDefinition("search_problems", "Search Codeforces by algorithm tag or Luogu by problem title/ID keywords. Choose platform=luogu when the user requests Luogu. Returns real problem names and URLs. No difficulty filtering; Luogu keywords are not algorithm tag filters.", map[string]any{"tag": stringProperty("Codeforces algorithm tag, or Luogu title/ID search keywords; empty string means any"), "platform": map[string]any{"type": "string", "enum": []string{"codeforces", "luogu"}, "description": "Problem platform, default codeforces"}, "limit": intProperty("Number of problems, default 5", 1, 20)}, "tag"),
		toolDefinition("get_problem", "Read a Codeforces or Luogu statement, constraints and samples before explaining or solving it. Returns original statement data, not community editorials. Use it instead of guessing a problem from its ID.", map[string]any{"reference": stringProperty("Codeforces ID such as 4A, Luogu ID such as P1001, or the platform's full HTTPS problem URL")}, "reference"),
		toolDefinition("search_knowledge", "Search the Internet using Tavily (requires TAVILY_API_KEY) for algorithm references and programming documentation. Returns titles, source URLs and search snippets, not full webpages. Prefer primary sources and cite URLs. Refine queries if results are irrelevant.", map[string]any{"query": stringProperty("Web search query, Chinese or English; prefer official documentation and authoritative algorithm sources"), "limit": intProperty("Maximum results, default 5", 1, 10)}, "query"),
	}
}

func decodeArguments(arguments string, target any) error {
	if strings.TrimSpace(arguments) == "null" {
		return errors.New("arguments must be an object")
	}
	decoder := json.NewDecoder(strings.NewReader(arguments))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("invalid tool arguments: %w", err)
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return errors.New("unexpected trailing tool arguments")
	}
	return nil
}

func (c *Client) executeTool(ctx context.Context, name, arguments string) string {
	toolCtx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	value, err := c.dispatchTool(toolCtx, name, arguments)
	if err != nil {
		value = map[string]string{"error": err.Error()}
	}
	data, err := json.Marshal(value)
	if err != nil {
		return `{"error":"cannot encode tool result"}`
	}
	return string(data)
}

// dispatchTool 将模型生成的工具调用交给 MCP 服务执行。
func (c *Client) dispatchTool(
	ctx context.Context,
	name, arguments string,
) (any, error) {
	if c.mcpSession == nil {
		return nil, errors.New("MCP 尚未连接")
	}

	// 将模型返回的 JSON 参数解析为对象。
	var args map[string]any
	if err := decodeArguments(arguments, &args); err != nil {
		return nil, err
	}

	result, err := c.mcpSession.CallTool(ctx, &mcp.CallToolParams{
		Name:      name,
		Arguments: args,
	})
	if err != nil {
		return nil, err
	}

	// 提取服务端返回的文本内容。
	var texts []string
	for _, content := range result.Content {
		if text, ok := content.(*mcp.TextContent); ok {
			texts = append(texts, text.Text)
		}
	}
	text := strings.Join(texts, "\n")

	if result.IsError {
		return nil, fmt.Errorf("工具执行失败：%s", text)
	}

	// 将 JSON 结果还原为对象，交给原来的结果编码逻辑。
	var value any
	if err := json.Unmarshal([]byte(text), &value); err == nil {
		return value, nil
	}
	return text, nil
}
