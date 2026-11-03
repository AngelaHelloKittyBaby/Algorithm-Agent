package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

type ProblemDetail struct {
	URL         string          `json:"url"`
	Title       string          `json:"title"`
	Statement   string          `json:"statement"`
	Input       string          `json:"input"`
	Output      string          `json:"output"`
	TimeLimit   string          `json:"time_limit"`
	MemoryLimit string          `json:"memory_limit"`
	Samples     []ProblemSample `json:"samples"`
}
type ProblemSample struct {
	Input  string `json:"input"`
	Output string `json:"output"`
}

var problemIDPattern = regexp.MustCompile(`^([1-9][0-9]{0,7})([A-Za-z][A-Za-z0-9]{0,3})$`)
var problemURLPattern = regexp.MustCompile(`^https://codeforces\.com/(?:problemset/problem/([1-9][0-9]{0,7})/([A-Za-z][A-Za-z0-9]{0,3})|contest/([1-9][0-9]{0,7})/problem/([A-Za-z][A-Za-z0-9]{0,3}))/?$`)

func GetProblem(ctx context.Context, reference string) (ProblemDetail, error) {
	if id := luoguReference(reference); id != "" {
		return getLuoguProblem(ctx, id)
	}
	var contest, index string
	ref := strings.TrimSpace(reference)
	if m := problemIDPattern.FindStringSubmatch(ref); m != nil {
		contest, index = m[1], m[2]
	} else if m := problemURLPattern.FindStringSubmatch(ref); m != nil {
		contest, index = m[1], m[2]
		if contest == "" {
			contest, index = m[3], m[4]
		}
	} else {
		return ProblemDetail{}, errors.New("请输入 Codeforces 题号（如 4A）、洛谷题号（如 P1001）或对应平台的 HTTPS 题目链接")
	}
	u := "https://codeforces.com/problemset/problem/" + contest + "/" + strings.ToUpper(index)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u+"?locale=en", nil)
	if err != nil {
		return ProblemDetail{}, err
	}
	req.Header.Set("User-Agent", "AlgorithmAgent/1.0")
	// 限制重定向目标，避免远程响应将请求转向其他主机。
	client := &http.Client{CheckRedirect: func(r *http.Request, via []*http.Request) error {
		if len(via) >= 5 || r.URL.Scheme != "https" || r.URL.Host != "codeforces.com" {
			return errors.New("unexpected problem redirect")
		}
		return nil
	}}
	resp, err := client.Do(req)
	if err != nil {
		return ProblemDetail{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return ProblemDetail{}, fmt.Errorf("Codeforces returned HTTP %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 2*1024*1024+1))
	if err != nil {
		return ProblemDetail{}, err
	}
	if len(data) > 2*1024*1024 {
		return ProblemDetail{}, errors.New("problem page is too large")
	}
	return parseProblem(string(data), u)
}

var divTokens = regexp.MustCompile(`(?is)<div\b[^>]*>|</div\s*>`)
var classAttribute = regexp.MustCompile(`(?i)\bclass\s*=\s*["']([^"']*)["']`)
var tagsPattern = regexp.MustCompile(`(?s)<[^>]*>`)
var lineTags = regexp.MustCompile(`(?i)<br\s*/?>|</(?:p|div|pre|li|h[1-6])\s*>`)
var prePattern = regexp.MustCompile(`(?is)<pre\b[^>]*>(.*?)</pre\s*>`)

// 按 div 标签的嵌套层级提取内容，保留题面和样例中的嵌套结构。
func classSections(page, class string) []string {
	result := []string{}
	tokens := divTokens.FindAllStringIndex(page, -1)
	for i, token := range tokens {
		attr := classAttribute.FindStringSubmatch(page[token[0]:token[1]])
		if len(attr) < 2 {
			continue
		}
		match := false
		for _, c := range strings.Fields(attr[1]) {
			if c == class {
				match = true
			}
		}
		if !match {
			continue
		}
		depth := 1
		for _, next := range tokens[i+1:] {
			if strings.HasPrefix(strings.ToLower(page[next[0]:next[1]]), "</") {
				depth--
			} else {
				depth++
			}
			if depth == 0 {
				result = append(result, page[token[1]:next[0]])
				break
			}
		}
	}
	return result
}
func plainHTML(s string) string {
	return strings.TrimSpace(html.UnescapeString(tagsPattern.ReplaceAllString(lineTags.ReplaceAllString(s, "\n"), "")))
}
func firstSection(page, class string) string {
	sections := classSections(page, class)
	if len(sections) == 0 {
		return ""
	}
	return plainHTML(sections[0])
}
func parseProblem(page, u string) (ProblemDetail, error) {
	sections := classSections(page, "problem-statement")
	if len(sections) == 0 {
		return ProblemDetail{}, errors.New("problem statement missing; page may be blocked by Codeforces")
	}
	s := sections[0]
	result := ProblemDetail{URL: u, Title: firstSection(s, "title"), Statement: plainHTML(s), Input: firstSection(s, "input-specification"), Output: firstSection(s, "output-specification"), TimeLimit: firstSection(s, "time-limit"), MemoryLimit: firstSection(s, "memory-limit"), Samples: []ProblemSample{}}
	inputs, outputs := classSections(s, "input"), classSections(s, "output")
	for i := 0; i < len(inputs) && i < len(outputs); i++ {
		in, out := prePattern.FindStringSubmatch(inputs[i]), prePattern.FindStringSubmatch(outputs[i])
		if len(in) > 1 && len(out) > 1 {
			result.Samples = append(result.Samples, ProblemSample{Input: plainHTML(in[1]), Output: plainHTML(out[1])})
		}
	}
	if result.Title == "" {
		return ProblemDetail{}, errors.New("problem title missing")
	}
	if len([]rune(result.Statement)) > 40000 {
		return ProblemDetail{}, errors.New("problem statement exceeds context size limit")
	}
	return result, nil
}

var luoguID = regexp.MustCompile(`^(?:P[0-9]+|B[0-9]+|U[0-9]+|T[0-9]+|CF[0-9]+[A-Z][0-9]*|AT_[A-Za-z0-9_]+|SP[0-9]+)$`)

// 添加洛谷数据请求头，直接接收 JSON，不解析网页。
func fetchLuogu(ctx context.Context, path string, target any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://www.luogu.com.cn"+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "AlgorithmAgent/1.0")
	req.Header.Set("x-lentille-request", "content-only")
	req.Header.Set("Accept", "application/json")
	client := &http.Client{Timeout: 20 * time.Second, CheckRedirect: func(r *http.Request, via []*http.Request) error {
		if len(via) >= 5 || r.URL.Scheme != "https" || r.URL.Host != "www.luogu.com.cn" {
			return fmt.Errorf("洛谷页面跳转受限：%s（跳转次数 %d）", r.URL.Scheme+"://"+r.URL.Host, len(via))
		}
		return nil
	}}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("洛谷返回 HTTP %d，题目可能不存在、需要登录或访问受限", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 4*1024*1024+1))
	if err != nil {
		return err
	}
	if len(data) > 4*1024*1024 {
		return errors.New("洛谷页面超过大小限制")
	}
	return decodeLuogu(data, target)
}

func decodeLuogu(data []byte, target any) error {
	var envelope struct {
		Status int             `json:"status"`
		Data   json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(data, &envelope); err != nil {
		return fmt.Errorf("洛谷数据解析失败：%w", err)
	}
	if envelope.Status != 200 {
		return fmt.Errorf("洛谷数据状态异常：%d", envelope.Status)
	}
	if len(envelope.Data) == 0 || string(envelope.Data) == "null" {
		return errors.New("洛谷未返回题目数据")
	}
	return json.Unmarshal(envelope.Data, target)
}

type luoguContent struct {
	Name        string `json:"name"`
	Background  string `json:"background"`
	Description string `json:"description"`
	Input       string `json:"formatI"`
	Output      string `json:"formatO"`
	Hint        string `json:"hint"`
}
type luoguProblem struct {
	PID     string       `json:"pid"`
	Name    string       `json:"name"`
	Content luoguContent `json:"content"`
	Contenu luoguContent `json:"contenu"`
	Samples [][]string   `json:"samples"`
	Limits  struct {
		Time   []int `json:"time"`
		Memory []int `json:"memory"`
	} `json:"limits"`
}

func getLuoguProblem(ctx context.Context, id string) (ProblemDetail, error) {
	var data struct {
		Problem *luoguProblem `json:"problem"`
	}
	if err := fetchLuogu(ctx, "/problem/"+id, &data); err != nil {
		return ProblemDetail{}, err
	}
	if data.Problem == nil {
		return ProblemDetail{}, errors.New("洛谷未返回题面")
	}
	p := data.Problem
	c := p.Content
	if c.Description == "" {
		c = p.Contenu
	}
	if p.PID != id || p.Name == "" || c.Description == "" {
		return ProblemDetail{}, errors.New("洛谷题目内容缺失或题号不匹配")
	}
	// 保留 Markdown 和数学公式，避免把题面中的比较符号误当作 HTML 标签删除。
	statement := strings.Join([]string{"## 题目背景", c.Background, "## 题目描述", c.Description, "## 输入格式", c.Input, "## 输出格式", c.Output, "## 说明与数据范围", c.Hint}, "\n\n")
	if len([]rune(statement)) > 40000 {
		return ProblemDetail{}, errors.New("题面超过上下文长度限制")
	}
	result := ProblemDetail{URL: "https://www.luogu.com.cn/problem/" + id, Title: p.PID + " " + p.Name, Statement: statement, Input: c.Input, Output: c.Output, Samples: []ProblemSample{}}
	for _, sample := range p.Samples {
		if len(sample) == 2 {
			result.Samples = append(result.Samples, ProblemSample{Input: sample[0], Output: sample[1]})
		}
	}
	// 各测试点可能使用不同限制，向模型保留完整限制列表。
	if len(p.Limits.Time) > 0 {
		result.TimeLimit = fmt.Sprintf("%v ms", p.Limits.Time)
	}
	if len(p.Limits.Memory) > 0 {
		result.MemoryLimit = fmt.Sprintf("%v KiB", p.Limits.Memory)
	}
	return result, nil
}

func luoguReference(reference string) string {
	ref := strings.TrimSpace(reference)
	if luoguID.MatchString(ref) {
		return ref
	}
	if luoguID.MatchString(strings.ToUpper(ref)) {
		return strings.ToUpper(ref)
	}
	u, err := url.Parse(ref)
	if err != nil || u.Scheme != "https" || u.Host != "www.luogu.com.cn" || u.User != nil {
		return ""
	}
	id := strings.TrimPrefix(u.Path, "/problem/")
	if u.Path != "/problem/"+id || !luoguID.MatchString(id) {
		return ""
	}
	return id
}
