package radio

//go:generate go generate ./rpc/generate.go
//go:generate moq -out mocks/radio.gen.go -pkg mocks . StorageService StorageTx TrackStorage SubmissionStorage UserStorage ManagerService
//go:generate moq -out mocks/templates.gen.go -pkg mocks ./templates/ Executor TemplateSelectable
//go:generate moq -out mocks/util.gen.go -pkg mocks ./mocks/ FS File FileInfo
