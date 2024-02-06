package public

import (
	"net/http"
)

type ChatInput struct {
	SharedInput
}

func (ChatInput) TemplateBundle() string {
	return "chat"
}

func NewChatInput(r *http.Request) ChatInput {
	return ChatInput{
		SharedInput: NewSharedInput(r),
	}
}

func (s State) GetChat(w http.ResponseWriter, r *http.Request) {
	input := NewChatInput(r)

	err := s.Templates.Execute(w, r, input)
	if err != nil {
		s.errorHandler(w, r, err)
		return
	}
}
