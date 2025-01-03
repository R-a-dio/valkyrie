package public

import (
	"net/http"

	"github.com/R-a-dio/valkyrie/website/middleware"
)

type ChatInput struct {
	middleware.Input
}

func (ChatInput) TemplateBundle() string {
	return "chat"
}

func NewChatInput(r *http.Request) ChatInput {
	return ChatInput{
		Input: middleware.InputFromRequest(r),
	}
}

func (s *State) GetChat(w http.ResponseWriter, r *http.Request) {
	input := NewChatInput(r)

	err := s.Templates.Execute(w, r, input)
	if err != nil {
		s.errorHandler(w, r, err)
		return
	}
}
