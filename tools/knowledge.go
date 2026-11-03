package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
	"unicode/utf8"
)

type KnowledgeHit struct {
	Title   string `json:"title"`
	URL     string `json:"url"`
	Snippet string `json:"snippet"`
}

// SearchKnowledge 通过 Tavily 联网搜索资料，返回标题、来源链接和搜索摘要。
func SearchKnowledge(ctx context.Context, query string, limit int) ([]KnowledgeHit, error) {
	return searchKnowledge(ctx, query, limit, &http.Client{Timeout: 20 * time.Second}, "https://api.tavily.com/search")
}

func searchKnowledge(ctx context.Context, query string, limit int, client *http.Client, endpoint string) ([]KnowledgeHit, error) {
	query = strings.TrimSpace(query)
	if query == "" || utf8.RuneCountInString(query) > 300 {
		return nil, errors.New("搜索关键词必须为 1 到 300 个字符")
	}
	if limit == 0 {
		limit = 5
	}
	if limit < 1 || limit > 10 {
		return nil, errors.New("搜索结果数量必须在 1 到 10 之间")
	}
	key := strings.TrimSpace(os.Getenv("TAVILY_API_KEY"))
	if key == "" {
		return nil, errors.New("联网搜索需要配置 TAVILY_API_KEY 环境变量")
	}
	payload, err := json.Marshal(map[string]any{"query": query, "max_results": limit, "search_depth": "basic", "include_answer": false, "include_raw_content": false})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "AlgorithmAgent/1.0")
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("联网搜索失败：%w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
			return nil, errors.New("搜索服务认证失败，请检查 TAVILY_API_KEY 和账户权限")
		}
		if resp.StatusCode == http.StatusTooManyRequests {
			return nil, errors.New("搜索服务限流，请稍后重试")
		}
		return nil, fmt.Errorf("搜索服务返回 HTTP %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 2*1024*1024+1))
	if err != nil {
		return nil, err
	}
	if len(data) > 2*1024*1024 {
		return nil, errors.New("搜索响应超过大小限制")
	}
	var feed struct {
		Results *[]struct {
			Title       string `json:"title"`
			Link        string `json:"url"`
			Description string `json:"content"`
		} `json:"results"`
	}
	if err = json.Unmarshal(data, &feed); err != nil {
		return nil, fmt.Errorf("无法解析搜索结果，搜索服务可能限制了访问：%w", err)
	}
	if feed.Results == nil {
		return nil, errors.New("搜索服务未返回有效 results 字段")
	}
	hits := []KnowledgeHit{}
	seen := map[string]bool{}
	for _, item := range *feed.Results {
		link := strings.TrimSpace(item.Link)
		parsed, err := url.Parse(link)
		if err != nil || parsed.Host == "" || (parsed.Scheme != "https" && parsed.Scheme != "http") || parsed.User != nil || seen[link] {
			continue
		}
		title := plainHTML(item.Title)
		if title == "" {
			continue
		}
		seen[link] = true
		// 搜索摘要不是网页全文，限制长度以控制模型上下文大小。
		snippet := []rune(plainHTML(item.Description))
		if len(snippet) > 1500 {
			snippet = snippet[:1500]
		}
		hits = append(hits, KnowledgeHit{Title: title, URL: link, Snippet: string(snippet)})
		if len(hits) >= limit {
			break
		}
	}
	return hits, nil
}
