package tui

import (
	"context"
	"fmt"
	"strings"

	"agentOpenai/ai"
	"agentOpenai/core"
	"agentOpenai/tools"
	"charm.land/bubbles/v2/list"
	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

var docStyle = lipgloss.NewStyle().Margin(1, 2)

type item struct{ title, desc string }

func (i item) Title() string       { return i.title }
func (i item) Description() string { return i.desc }
func (i item) FilterValue() string { return i.title }

type screen int

const (
	homeScreen screen = iota
	searchScreen
	questionScreen
)

type answerMsg struct {
	text string
	err  error
}
type answerChunk string
type thinkingChunk string
type problemsMsg struct {
	problems []tools.Problem
	err      error
}

type Model struct {
	list        list.Model
	input       textinput.Model
	searchInput textinput.Model
	chat        viewport.Model
	screen      screen
	question    string
	answer      string
	thinking    string
	loading     bool
	client      *ai.Client
	session     *core.Session
	clientErr   error
	problems    []tools.Problem
	searching   bool
	searchErr   error
	askEvents   chan tea.Msg
}

func NewModel() Model {
	items := []list.Item{
		item{title: "算法问题问答", desc: "和 AI 多轮讨论算法问题"},
	}
	m := list.New(items, list.NewDefaultDelegate(), 0, 0)
	m.Title = "Algorithm Agent"
	input := textinput.New()
	input.Prompt = "> "
	input.Placeholder = "输入你的算法问题"
	searchInput := textinput.New()
	searchInput.Prompt = "搜索 > "
	searchInput.Placeholder = "例如 dp，或 洛谷 P1001"
	client, clientErr := ai.NewClientFromEnv()
	// AI 客户端创建成功后，连接 MCP 工具服务。
	if clientErr == nil {
		clientErr = client.ConnectMCP(context.Background())
	}
	return Model{list: m, input: input, searchInput: searchInput, chat: viewport.New(), screen: homeScreen, client: client, session: core.NewSession(), clientErr: clientErr}
}

// Close 释放 MCP 会话。
func (m Model) Close() error {
	if m.client == nil {
		return nil
	}
	return m.client.CloseMCP()
}
func searchCmd(tag string) tea.Cmd {
	return func() tea.Msg {
		platform := "codeforces"
		for _, prefix := range []string{"洛谷 ", "洛谷：", "洛谷:", "luogu ", "luogu:"} {
			if strings.HasPrefix(strings.ToLower(tag), prefix) {
				platform = "luogu"
				tag = strings.TrimSpace(tag[len(prefix):])
				break
			}
		}
		p, err := tools.SearchProblems(context.Background(), tag, 5, platform)
		return problemsMsg{problems: p, err: err}
	}
}

func askCmd(client *ai.Client, session *core.Session, question string, events chan tea.Msg) tea.Cmd {
	return func() tea.Msg {
		// 后台执行问答，通过消息通道按顺序刷新界面，避免跨协程修改界面状态。
		go func() {
			answer, err := client.Ask(context.Background(), session, question,
				func(text string) { events <- answerChunk(text) },
				func(text string) { events <- thinkingChunk(text) })
			events <- answerMsg{text: answer, err: err}
		}()
		return <-events
	}
}

func waitAskEvent(events chan tea.Msg) tea.Cmd {
	return func() tea.Msg { return <-events }
}

func (m Model) Init() tea.Cmd { return nil }

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case thinkingChunk:
		m.thinking += string(msg)
		m.updateChat()
		m.chat.GotoBottom()
		return m, waitAskEvent(m.askEvents)
	case answerChunk:
		m.answer = string(msg)
		m.updateChat()
		m.chat.GotoBottom()
		return m, waitAskEvent(m.askEvents)
	case answerMsg:
		m.loading = false
		m.askEvents = nil
		if msg.err != nil {
			if m.answer != "" {
				m.answer += "\n\n"
			}
			m.answer += "请求失败：" + msg.err.Error()
		} else {
			m.answer = msg.text
		}
		m.updateChat()
		m.chat.GotoBottom()
		return m, nil
	case problemsMsg:
		m.searching = false
		m.problems = msg.problems
		m.searchErr = msg.err
		if msg.err != nil {
			m.problems = nil
		}
		return m, nil
	case tea.KeyPressMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit
		case "esc":
			m.screen = homeScreen
			m.input.Blur()
			m.searchInput.Blur()
		case "enter":
			if m.screen == homeScreen {
				m.screen = questionScreen
				m.input.Focus()
			} else if m.screen == searchScreen && !m.searching && m.searchInput.Value() != "" {
				tag := m.searchInput.Value()
				m.searchInput.SetValue("")
				m.searching = true
				return m, searchCmd(tag)
			} else if m.screen == questionScreen && !m.loading && m.input.Value() != "" && m.client != nil {
				question := m.input.Value()
				m.input.SetValue("")
				m.question = question
				m.answer = ""
				m.thinking = ""
				m.askEvents = make(chan tea.Msg, 16)
				m.loading = true
				m.updateChat()
				return m, askCmd(m.client, m.session, question, m.askEvents)
			}
		}
	case tea.WindowSizeMsg:
		h, v := docStyle.GetFrameSize()
		m.list.SetSize(msg.Width-h, msg.Height-v)
		m.input.SetWidth(msg.Width - h - 4)
		m.searchInput.SetWidth(msg.Width - h - 4)
		m.chat.SetWidth(msg.Width - h)
		m.chat.SetHeight(msg.Height - v - 7)
	}
	if m.screen == homeScreen {
		var cmd tea.Cmd
		m.list, cmd = m.list.Update(msg)
		return m, cmd
	}
	if m.screen == questionScreen {
		var cmd tea.Cmd
		m.chat, _ = m.chat.Update(msg)
		m.input, cmd = m.input.Update(msg)
		return m, cmd
	}
	if m.screen == searchScreen {
		var cmd tea.Cmd
		m.searchInput, cmd = m.searchInput.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m *Model) updateChat() {
	content := ""
	if m.question != "" {
		content += "问题：" + m.question + "\n"
	}
	if m.thinking != "" {
		content += "\n思考过程（模型返回）：\n" + plainAnswer(m.thinking) + "\n"
	}
	if m.answer != "" {
		content += "\nAI：" + plainAnswer(m.answer) + "\n"
	}
	if m.loading {
		content += "\nAI 正在处理，请稍候……\n"
	}
	m.chat.SetContent(content)
}

// 将正文中残留的公式标记转成普通文本，代码块和行内代码保持原样。
func plainAnswer(text string) string {
	replacer := strings.NewReplacer(
		"$", "", `\(`, "", `\)`, "", `\[`, "", `\]`, "",
		`\leq`, "<=", `\le`, "<=", `\geq`, ">=", `\ge`, ">=",
		`\neq`, "!=", `\times`, "*", `\cdot`, "*", `\sim`, "~", `\in`, "属于",
	)
	lines := strings.SplitAfter(text, "\n")
	fence := ""
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			if fence == "" {
				fence = trimmed[:3]
			} else if strings.HasPrefix(trimmed, fence) {
				fence = ""
			}
			continue
		}
		if fence != "" {
			continue
		}
		parts := strings.Split(line, "`")
		for j := 0; j < len(parts); j += 2 {
			parts[j] = replacer.Replace(parts[j])
		}
		lines[i] = strings.Join(parts, "`")
	}
	return strings.Join(lines, "")
}

func (m Model) View() tea.View {
	var content string
	switch m.screen {
	case homeScreen:
		content = m.list.View()
	case searchScreen:
		content = "搜索算法题\n\n"
		if m.searching {
			content += "正在搜索……\n"
		}
		if m.searchErr != nil {
			content += "搜索失败：" + m.searchErr.Error() + "\n"
		}
		if !m.searching && m.searchErr == nil && m.searchInput.Value() == "" && len(m.problems) == 0 {
			content += "Codeforces 输入算法标签，例如：贪心、dp；洛谷输入：洛谷 P1001 或 洛谷 背包\n\n"
		}
		if !m.searching && m.searchErr == nil && len(m.problems) == 0 && m.searchInput.Value() == "" {
			content += "没有找到匹配题目。\n"
		}
		for _, p := range m.problems {
			content += fmt.Sprintf("%s\n%s\n\n", p.Name, p.URL)
		}
		content += m.searchInput.View() + "\n\n按 Enter 搜索，Esc 返回主菜单"
	case questionScreen:
		content = "算法问题问答\n\n"
		if m.clientErr != nil {
			content += "配置错误：" + m.clientErr.Error() + "\n\n"
		}
		content += m.chat.View() + "\n" + m.input.View() + "\n\n按 Enter 提问，↑/↓ 滚动，Esc 返回主菜单"
	}
	v := tea.NewView(docStyle.Render(content))
	v.AltScreen = true
	return v
}
