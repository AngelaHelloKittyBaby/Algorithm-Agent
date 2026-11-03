package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

type Problem struct {
	Name, URL string
	Tags      []string
}

type cfResponse struct {
	Status string `json:"status"`
	Result struct {
		Problems []struct {
			ContestID int      `json:"contestId"`
			Index     string   `json:"index"`
			Name      string   `json:"name"`
			Tags      []string `json:"tags"`
		} `json:"problems"`
	} `json:"result"`
}

func SearchProblems(ctx context.Context, tag string, limit int, platform string) ([]Problem, error) {
	if limit <= 0 {
		limit = 5
	}
	if limit > 20 {
		return nil, errors.New("最多返回 20 道题")
	}
	switch strings.ToLower(strings.TrimSpace(platform)) {
	case "luogu":
		return searchLuogu(ctx, tag, limit)
	case "", "codeforces":
	default:
		return nil, errors.New("platform 仅支持 codeforces 或 luogu")
	}
	u := "https://codeforces.com/api/problemset.problems?" + url.Values{"lang": {"en"}}.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("题目接口返回 HTTP %d", resp.StatusCode)
	}
	var data cfResponse
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, err
	}
	if data.Status != "OK" {
		return nil, fmt.Errorf("题目接口状态异常：%s", data.Status)
	}
	want := strings.ToLower(strings.TrimSpace(tag))
	aliases := map[string]string{"贪心": "greedy", "动态规划": "dp", "数组": "implementation", "图": "graphs", "字符串": "strings", "二分": "binary search", "双指针": "two pointers"}
	if mapped, ok := aliases[want]; ok {
		want = mapped
	}
	result := make([]Problem, 0, limit)
	for _, p := range data.Result.Problems {
		matched := want == ""
		for _, t := range p.Tags {
			if strings.Contains(strings.ToLower(t), want) {
				matched = true
			}
		}
		if !matched || p.ContestID == 0 {
			continue
		}
		result = append(result, Problem{Name: p.Name, URL: fmt.Sprintf("https://codeforces.com/problemset/problem/%d/%s", p.ContestID, p.Index), Tags: p.Tags})
		if len(result) >= limit {
			break
		}
	}
	return result, nil
}

func searchLuogu(ctx context.Context, keyword string, limit int) ([]Problem, error) {
	var data struct {
		Problems *struct {
			Result []struct {
				PID  string `json:"pid"`
				Name string `json:"name"`
			} `json:"result"`
		} `json:"problems"`
	}
	if err := fetchLuogu(ctx, "/problem/list?"+url.Values{"keyword": {keyword}, "page": {"1"}}.Encode(), &data); err != nil {
		return nil, err
	}
	if data.Problems == nil {
		return nil, errors.New("洛谷搜索结果缺少题目列表")
	}
	result := []Problem{}
	for _, p := range data.Problems.Result {
		if !luoguID.MatchString(p.PID) || p.Name == "" {
			continue
		}
		result = append(result, Problem{Name: p.PID + " " + p.Name, URL: "https://www.luogu.com.cn/problem/" + p.PID, Tags: []string{}})
		if len(result) >= limit {
			break
		}
	}
	return result, nil
}
