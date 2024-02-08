package radio

//go:generate go generate ./rpc/generate.go
//go:generate moq -out mocks/radio.gen.go -pkg mocks . StorageService StorageTx TrackStorage SubmissionStorage
//go:generate moq -out mocks/templates.gen.go -pkg mocks ./templates/ Executor
