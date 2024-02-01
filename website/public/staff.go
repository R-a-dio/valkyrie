package public

import (
	"log"
	"net/http"
)

type User = struct {
	DjName string
	DjImage string
	Role string
	Visible int
}

func (s State) GetStaff(w http.ResponseWriter, r *http.Request) {

	users := []User{
		{DjName:"benisman", 	DjImage:"1-59b10a1565ef8119.png", Role:"staff", Visible:1},
		{DjName:"fug", 			DjImage:"2-1a704457d3834f41.png", Role:"staff", Visible:1},
		{DjName:"testuser9000", DjImage:"3-8c035a1bcbcde125.png", Role:"dev", 	Visible:1},
		{DjName:"testuser9001", DjImage:"2-1a704457d3834f41.png", Role:"dev", 	Visible:1},
		{DjName:"gooddj", 		DjImage:"3-8c035a1bcbcde125.png", Role:"dj", 	Visible:1},
		{DjName:"gooddj2", 		DjImage:"3-8c035a1bcbcde125.png", Role:"dj", 	Visible:1},
		{DjName:"gooddj3", 		DjImage:"2-1a704457d3834f41.png", Role:"dj", 	Visible:1},
		{DjName:"gooddj4", 		DjImage:"1-59b10a1565ef8119.png", Role:"dj", 	Visible:1},
		{DjName:"gooddj5", 		DjImage:"3-8c035a1bcbcde125.png", Role:"dj", 	Visible:1},
		{DjName:"baddj", 		DjImage:"2-1a704457d3834f41.png", Role:"dj", 	Visible:1},
		{DjName:"gooddj6", 		DjImage:"3-8c035a1bcbcde125.png", Role:"dj", 	Visible:1},
	}

	roles := []string{
		"staff",
		"dev",
		"dj",
	}
	
	staffInput := struct {
		shared
		Users []User
		Roles []string
	}{
		shared: s.shared(r),
		Users: users,
		Roles: roles,
	}

	err := s.TemplateExecutor.ExecuteFull(theme, "staff", w, staffInput)
	if err != nil {
		log.Println(err)
		return
	}
}
