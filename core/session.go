package core

import "github.com/openai/openai-go/v3"

// Session 保存 Chat Completions API 所需的完整多轮对话消息。
type Session struct {
	Messages []openai.ChatCompletionMessageParamUnion
}

func NewSession() *Session {
	return &Session{
		Messages: make([]openai.ChatCompletionMessageParamUnion, 0),
	}
}

func (s *Session) GetMessages() []openai.ChatCompletionMessageParamUnion {
	return s.Messages
}

func (s *Session) AddMessage(message openai.ChatCompletionMessageParamUnion) {
	s.Messages = append(s.Messages, message)
}

func (s *Session) Len() int {
	return len(s.Messages)
}
