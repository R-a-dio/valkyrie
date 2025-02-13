package config

import "time"

// defaultConfig is the default configuration for this project
var defaultConfig = config{
	DevelopmentMode:  true,
	UserAgent:        "hanyuu/2.0",
	UserRequestDelay: Duration(time.Hour * 1),
	UserUploadDelay:  Duration(time.Hour * 2),
	TemplatePath:     "templates/",
	MusicPath:        "/radio/music",
	AssetsPath:       "./assets/",
	Providers: providers{
		Storage: "mariadb",
		Search:  "storage",
	},
	Database: database{
		DriverName: "mysql",
		DSN:        "radio@unix(/run/mysqld/mysqld.sock)/radio?parseTime=true",
	},
	Website: website{
		WebsiteAddr:               MustParseAddrPort("localhost:3241"),
		DJImageMaxSize:            10 * 1024 * 1024,
		DJImagePath:               "/radio/dj-images",
		PublicStreamURL:           "http://localhost:8000/main.mp3",
		AdminMonitoringURL:        "http://grafana:3000",
		AdminMonitoringUserHeader: "x-proxy-user",
		AdminMonitoringRoleHeader: "x-proxy-role",
	},
	Streamer: streamer{
		RPCAddr:         MustParseAddrPort(":4545"),
		StreamURL:       "http://127.0.0.1:1337/main.mp3",
		RequestsEnabled: true,
		ConnectTimeout:  Duration(time.Second * 30),
	},
	IRC: irc{
		RPCAddr:        MustParseAddrPort(":4444"),
		AllowFlood:     false,
		EnableEcho:     true,
		AnnouncePeriod: Duration(time.Second * 15),
	},
	Manager: manager{
		RPCAddr:         MustParseAddrPort(":4646"),
		FallbackNames:   []string{"fallback"},
		GuestProxyAddr:  "//localhost:9123",
		GuestAuthPeriod: Duration(time.Hour * 24),
	},
	Search: search{
		Endpoint:  "http://127.0.0.1:9200/",
		IndexPath: "/radio/search",
	},
	Balancer: balancer{
		Addr:     "127.0.0.1:4848",
		Fallback: "https://relay0.r-a-d.io/main.mp3",
	},
	Proxy: proxy{
		RPCAddr:            MustParseAddrPort(":5151"),
		ListenAddr:         MustParseAddrPort(":1337"),
		MasterServer:       "http://127.0.0.1:8000",
		MasterUsername:     "source",
		MasterPassword:     "hackme",
		PrimaryMountName:   "/main.mp3",
		IcecastDescription: "a valkyrie in testing (change this in the config file)",
		IcecastName:        "valkyrie-stream",
	},
	Tracker: tracker{
		RPCAddr:          MustParseAddrPort(":4949"),
		ListenAddr:       MustParseAddrPort(":9999"),
		MasterServer:     "http://127.0.0.1:8000",
		MasterUsername:   "admin",
		MasterPassword:   "hackme",
		PrimaryMountName: "/main.mp3",
	},
	Telemetry: telemetry{
		Use:                false,
		Endpoint:           ":4317",
		PrometheusEndpoint: "localhost:9091",
	},
	Tunein: tunein{
		Endpoint: "https://air.radiotime.com/Playing.ashx",
	},
}
