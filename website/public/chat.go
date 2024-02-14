package public

import (
	"net/http"

	"github.com/R-a-dio/valkyrie/website/shared"
)

type ChatInput struct {
	shared.Input
}

func (ChatInput) TemplateBundle() string {
	return "chat"
}

func NewChatInput(f *shared.InputFactory, r *http.Request) ChatInput {
	return ChatInput{
		Input: f.New(r),
	}
}

func (s State) GetChat(w http.ResponseWriter, r *http.Request) {
	input := NewChatInput(s.Shared, r)

	err := s.Templates.Execute(w, r, input)
	if err != nil {
		s.errorHandler(w, r, err)
		return
	}
}
