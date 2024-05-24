module github.com/R-a-dio/valkyrie

go 1.22.1

replace github.com/gorilla/csrf => github.com/Wessie/csrf v0.0.0-20240430222011-49a3bdf737c0

require (
	github.com/BurntSushi/toml v1.3.2
	github.com/DATA-DOG/go-sqlmock v1.5.2
	github.com/Wessie/fdstore v1.2.1
	github.com/XSAM/otelsql v0.29.0
	github.com/agoda-com/opentelemetry-go/otelzerolog v0.0.1
	github.com/agoda-com/opentelemetry-logs-go v0.4.3
	github.com/alevinval/sse v1.0.2
	github.com/alexedwards/scs/v2 v2.8.0
	github.com/cenkalti/backoff v2.2.1+incompatible
	github.com/cenkalti/backoff/v4 v4.3.0
	github.com/davecgh/go-spew v1.1.1
	github.com/go-chi/httprate v0.9.0
	github.com/go-sql-driver/mysql v1.8.1
	github.com/golang-migrate/migrate/v4 v4.17.0
	github.com/google/subcommands v1.2.0
	github.com/gorilla/csrf v1.7.2
	github.com/jmoiron/sqlx v1.3.5
	github.com/jxskiss/base62 v1.1.0
	github.com/leanovate/gopter v0.2.10
	github.com/lrstanley/girc v0.0.0-20240317214256-a4a3d96369cb
	github.com/prometheus/client_golang v1.19.0
	github.com/rs/xid v1.5.0
	github.com/rs/zerolog v1.32.0
	github.com/spf13/afero v1.11.0
	github.com/stretchr/testify v1.9.0
	github.com/tcolgate/mp3 v0.0.0-20170426193717-e79c5a46d300
	github.com/testcontainers/testcontainers-go v0.29.1
	github.com/testcontainers/testcontainers-go/modules/mariadb v0.29.1
	github.com/yuin/goldmark v1.7.0
	go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc v0.49.0
	go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp v0.49.0
	go.opentelemetry.io/otel v1.24.0
	go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc v1.24.0
	go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc v1.24.0
	go.opentelemetry.io/otel/sdk v1.24.0
	go.opentelemetry.io/otel/sdk/metric v1.24.0
	go.opentelemetry.io/otel/trace v1.24.0
	golang.org/x/crypto v0.22.0
	golang.org/x/exp v0.0.0-20240325151524-a685a6edb6d8
	golang.org/x/term v0.19.0
	golang.org/x/text v0.14.0
	golang.org/x/tools v0.19.0
	google.golang.org/grpc v1.63.2
)

require (
	dario.cat/mergo v1.0.0 // indirect
	filippo.io/edwards25519 v1.1.0 // indirect
	github.com/Azure/go-ansiterm v0.0.0-20230124172434-306776ec8161 // indirect
	github.com/Microsoft/go-winio v0.6.1 // indirect
	github.com/Microsoft/hcsshim v0.12.2 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/cespare/xxhash/v2 v2.2.0 // indirect
	github.com/containerd/containerd v1.7.14 // indirect
	github.com/containerd/log v0.1.0 // indirect
	github.com/cpuguy83/dockercfg v0.3.1 // indirect
	github.com/distribution/reference v0.5.0 // indirect
	github.com/docker/docker v26.1.0+incompatible // indirect
	github.com/docker/go-connections v0.5.0 // indirect
	github.com/docker/go-units v0.5.0 // indirect
	github.com/felixge/httpsnoop v1.0.4 // indirect
	github.com/go-logr/logr v1.4.1 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/go-ole/go-ole v1.3.0 // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/gorilla/securecookie v1.1.2 // indirect
	github.com/grpc-ecosystem/grpc-gateway/v2 v2.19.1 // indirect
	github.com/justincormack/go-memfd v0.0.0-20170219213707-6e4af0518993 // indirect
	github.com/klauspost/compress v1.17.7 // indirect
	github.com/lufia/plan9stats v0.0.0-20240226150601-1dcf7310316a // indirect
	github.com/magiconair/properties v1.8.7 // indirect
	github.com/mattn/go-colorable v0.1.13 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/moby/docker-image-spec v1.3.1 // indirect
	github.com/moby/patternmatcher v0.6.0 // indirect
	github.com/moby/sys/sequential v0.5.0 // indirect
	github.com/moby/sys/user v0.1.0 // indirect
	github.com/moby/term v0.5.0 // indirect
	github.com/morikuni/aec v1.0.0 // indirect
	github.com/opencontainers/go-digest v1.0.0 // indirect
	github.com/opencontainers/image-spec v1.1.0 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/power-devops/perfstat v0.0.0-20240221224432-82ca36839d55 // indirect
	github.com/prometheus/client_model v0.6.0 // indirect
	github.com/prometheus/common v0.51.1 // indirect
	github.com/prometheus/procfs v0.13.0 // indirect
	github.com/shirou/gopsutil/v3 v3.24.3 // indirect
	github.com/shoenig/go-m1cpu v0.1.6 // indirect
	github.com/sirupsen/logrus v1.9.3 // indirect
	github.com/tklauser/go-sysconf v0.3.13 // indirect
	github.com/tklauser/numcpus v0.7.0 // indirect
	github.com/yusufpapurcu/wmi v1.2.4 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlptrace v1.24.0 // indirect
	go.opentelemetry.io/otel/metric v1.24.0 // indirect
	go.opentelemetry.io/proto/otlp v1.1.0 // indirect
	golang.org/x/mod v0.16.0 // indirect
	golang.org/x/net v0.24.0 // indirect
	golang.org/x/sys v0.20.0 // indirect
	google.golang.org/genproto/googleapis/api v0.0.0-20240401170217-c3f982113cda // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20240401170217-c3f982113cda // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

require (
	github.com/go-chi/chi/v5 v5.0.12
	github.com/hashicorp/errwrap v1.1.0 // indirect
	github.com/hashicorp/go-multierror v1.1.1 // indirect
	go.uber.org/atomic v1.11.0 // indirect
	google.golang.org/protobuf v1.33.0
)
