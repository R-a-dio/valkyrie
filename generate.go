package radio

//go:generate go generate ./rpc/generate.go
//go:generate moq -out mocks/radio.gen.go -pkg mocks . SearchService ManagerService StreamerService QueueService AnnounceService StorageTx StorageService SessionStorageService SessionStorage QueueStorageService QueueStorage SongStorageService SongStorage TrackStorageService TrackStorage RequestStorageService RequestStorage UserStorageService UserStorage StatusStorageService StatusStorage NewsStorageService NewsStorage SubmissionStorageService SubmissionStorage RelayStorage RelayStorageService ScheduleStorageService ScheduleStorage
//go:generate moq -out mocks/templates.gen.go -pkg mocks ./templates/ Executor TemplateSelectable
//go:generate moq -out mocks/streamer.gen.go -pkg mocks ./streamer/audio/ Reader
//go:generate moq -out mocks/util.gen.go -pkg mocks ./mocks/ FS File FileInfo
//go:generate moq -out mocks/eventstream.gen.go -pkg mocks ./util/eventstream/ Stream
