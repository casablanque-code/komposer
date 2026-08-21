package composer

import (
	"fmt"
	"strings"
)

type Preset struct {
	Name        string
	Description string
	Config      *ServiceConfig
}

var Presets = []Preset{
	{
		Name:        "PostgreSQL",
		Description: "PostgreSQL database with persistent volume and healthcheck",
		Config: &ServiceConfig{
			Image: "postgres:15-alpine",
			Ports: []string{"5432:5432"},
			Environment: []string{
				"POSTGRES_USER=postgres",
				"POSTGRES_PASSWORD=postgres",
				"POSTGRES_DB=mydb",
			},
			Volumes: []string{"postgres_data:/var/lib/postgresql/data"},
			HealthCheck: &HealthCheck{
				Test:     []string{"CMD-SHELL", "pg_isready -U postgres"},
				Interval: "10s",
				Timeout:  "5s",
				Retries:  5,
			},
			Restart: "unless-stopped",
		},
	},
	{
		Name:        "Redis",
		Description: "Redis cache with persistent volume",
		Config: &ServiceConfig{
			Image:   "redis:7-alpine",
			Ports:   []string{"6379:6379"},
			Volumes: []string{"redis_data:/data"},
			HealthCheck: &HealthCheck{
				Test:     []string{"CMD", "redis-cli", "ping"},
				Interval: "5s",
				Timeout:  "3s",
				Retries:  3,
			},
			Restart: "unless-stopped",
		},
	},
	{
		Name:        "Nginx",
		Description: "Nginx web server with HTTP/HTTPS ports",
		Config: &ServiceConfig{
			Image: "nginx:alpine",
			Ports: []string{"80:80", "443:443"},
			Volumes: []string{
				"./nginx.conf:/etc/nginx/nginx.conf:ro",
				"./html:/usr/share/nginx/html:ro",
			},
			Restart: "unless-stopped",
		},
	},
	{
		Name:        "MySQL",
		Description: "MySQL database with persistent volume",
		Config: &ServiceConfig{
			Image: "mysql:8",
			Ports: []string{"3306:3306"},
			Environment: []string{
				"MYSQL_ROOT_PASSWORD=root",
				"MYSQL_DATABASE=mydb",
				"MYSQL_USER=user",
				"MYSQL_PASSWORD=password",
			},
			Volumes: []string{"mysql_data:/var/lib/mysql"},
			HealthCheck: &HealthCheck{
				Test:     []string{"CMD", "mysqladmin", "ping", "-h", "localhost"},
				Interval: "10s",
				Timeout:  "5s",
				Retries:  5,
			},
			Restart: "unless-stopped",
		},
	},
	{
		Name:        "MongoDB",
		Description: "MongoDB NoSQL database",
		Config: &ServiceConfig{
			Image: "mongo:7",
			Ports: []string{"27017:27017"},
			Environment: []string{
				"MONGO_INITDB_ROOT_USERNAME=root",
				"MONGO_INITDB_ROOT_PASSWORD=root",
			},
			Volumes: []string{"mongo_data:/data/db"},
			Restart: "unless-stopped",
		},
	},
}

func (c *ComposeConfig) AddPreset(preset Preset, serviceName string) *ServiceConfig {
	svc := c.AddService(serviceName)
	*svc = *preset.Config
	return svc
}

// StackService is one service within a Stack: a suggested name plus the
// config to give it. The name is a starting point, not a guarantee —
// AddStack uniquifies it if something in the config already uses it.
type StackService struct {
	Name   string
	Config *ServiceConfig
}

// Stack is a ready-made group of services that are commonly used
// together, with the dependencies and shared identifiers between them
// already wired up (depends_on, one service's env pointing at another
// by name, etc.) — unlike a single Preset, which only ever adds one
// isolated service with nothing else to configure. This is the
// "1-click Postgres + PgAdmin" idea: pick the stack, get a working
// multi-service setup in one step instead of building it by hand.
type Stack struct {
	Name        string
	Description string
	Services    []StackService
}

// Stacks uses "{{localname}}" inside environment values to reference
// another service in the same stack by hostname (e.g. "{{db}}" for the
// service named "db" below) — see AddStack for why a literal name like
// "db" can't be hardcoded there.
var Stacks = []Stack{
	{
		Name:        "Postgres + PgAdmin",
		Description: "Postgres database with a web UI to browse and query it",
		Services: []StackService{
			{
				Name: "db",
				Config: &ServiceConfig{
					Image: "postgres:15-alpine",
					Environment: []string{
						"POSTGRES_USER=postgres",
						"POSTGRES_PASSWORD=postgres",
						"POSTGRES_DB=app",
					},
					Volumes: []string{"postgres_data:/var/lib/postgresql/data"},
					HealthCheck: &HealthCheck{
						Test:     []string{"CMD-SHELL", "pg_isready -U postgres"},
						Interval: "10s",
						Timeout:  "5s",
						Retries:  5,
					},
					Restart: "unless-stopped",
				},
			},
			{
				Name: "pgadmin",
				Config: &ServiceConfig{
					Image: "dpage/pgadmin4:latest",
					Ports: []string{"5050:80"},
					Environment: []string{
						"PGADMIN_DEFAULT_EMAIL=admin@admin.com",
						"PGADMIN_DEFAULT_PASSWORD=admin",
					},
					DependsOn: []DependsOnEntry{{Service: "db", Condition: CondServiceHealthy}},
					Restart:   "unless-stopped",
				},
			},
		},
	},
	{
		Name:        "Postgres + Redis",
		Description: "Postgres for storage plus Redis for caching or sessions",
		Services: []StackService{
			{
				Name: "db",
				Config: &ServiceConfig{
					Image: "postgres:15-alpine",
					Environment: []string{
						"POSTGRES_USER=postgres",
						"POSTGRES_PASSWORD=postgres",
						"POSTGRES_DB=app",
					},
					Volumes: []string{"postgres_data:/var/lib/postgresql/data"},
					HealthCheck: &HealthCheck{
						Test:     []string{"CMD-SHELL", "pg_isready -U postgres"},
						Interval: "10s",
						Timeout:  "5s",
						Retries:  5,
					},
					Restart: "unless-stopped",
				},
			},
			{
				Name: "cache",
				Config: &ServiceConfig{
					Image:   "redis:7-alpine",
					Volumes: []string{"redis_data:/data"},
					HealthCheck: &HealthCheck{
						Test:     []string{"CMD", "redis-cli", "ping"},
						Interval: "5s",
						Timeout:  "3s",
						Retries:  3,
					},
					Restart: "unless-stopped",
				},
			},
		},
	},
	{
		Name:        "WordPress + MySQL",
		Description: "WordPress site backed by its own MySQL database",
		Services: []StackService{
			{
				Name: "db",
				Config: &ServiceConfig{
					Image: "mysql:8",
					Environment: []string{
						"MYSQL_ROOT_PASSWORD=wordpress",
						"MYSQL_DATABASE=wordpress",
						"MYSQL_USER=wordpress",
						"MYSQL_PASSWORD=wordpress",
					},
					Volumes: []string{"wordpress_db_data:/var/lib/mysql"},
					HealthCheck: &HealthCheck{
						Test:     []string{"CMD", "mysqladmin", "ping", "-h", "localhost"},
						Interval: "10s",
						Timeout:  "5s",
						Retries:  5,
					},
					Restart: "unless-stopped",
				},
			},
			{
				Name: "wordpress",
				Config: &ServiceConfig{
					Image: "wordpress:latest",
					Ports: []string{"8080:80"},
					Environment: []string{
						"WORDPRESS_DB_HOST={{db}}:3306",
						"WORDPRESS_DB_USER=wordpress",
						"WORDPRESS_DB_PASSWORD=wordpress",
						"WORDPRESS_DB_NAME=wordpress",
					},
					Volumes:   []string{"wordpress_data:/var/www/html"},
					DependsOn: []DependsOnEntry{{Service: "db", Condition: CondServiceHealthy}},
					Restart:   "unless-stopped",
				},
			},
		},
	},
	{
		Name:        "Mongo + Mongo Express",
		Description: "MongoDB with a web UI to browse collections",
		Services: []StackService{
			{
				Name: "db",
				Config: &ServiceConfig{
					Image: "mongo:7",
					Environment: []string{
						"MONGO_INITDB_ROOT_USERNAME=root",
						"MONGO_INITDB_ROOT_PASSWORD=root",
					},
					Volumes: []string{"mongo_data:/data/db"},
					Restart: "unless-stopped",
				},
			},
			{
				Name: "mongo-express",
				Config: &ServiceConfig{
					Image: "mongo-express:latest",
					Ports: []string{"8081:8081"},
					Environment: []string{
						"ME_CONFIG_MONGODB_ADMINUSERNAME=root",
						"ME_CONFIG_MONGODB_ADMINPASSWORD=root",
						"ME_CONFIG_MONGODB_SERVER={{db}}",
					},
					DependsOn: []DependsOnEntry{{Service: "db", Condition: CondServiceStarted}},
					Restart:   "unless-stopped",
				},
			},
		},
	},
	{
		Name:        "MySQL + phpMyAdmin",
		Description: "MySQL database with a web UI to browse and query it",
		Services: []StackService{
			{
				Name: "db",
				Config: &ServiceConfig{
					Image: "mysql:8",
					Environment: []string{
						"MYSQL_ROOT_PASSWORD=root",
						"MYSQL_DATABASE=app",
					},
					Volumes: []string{"mysql_data:/var/lib/mysql"},
					HealthCheck: &HealthCheck{
						Test:     []string{"CMD", "mysqladmin", "ping", "-h", "localhost"},
						Interval: "10s",
						Timeout:  "5s",
						Retries:  5,
					},
					Restart: "unless-stopped",
				},
			},
			{
				Name: "phpmyadmin",
				Config: &ServiceConfig{
					Image: "phpmyadmin:latest",
					Ports: []string{"8082:80"},
					Environment: []string{
						"PMA_HOST={{db}}",
					},
					DependsOn: []DependsOnEntry{{Service: "db", Condition: CondServiceHealthy}},
					Restart:   "unless-stopped",
				},
			},
		},
	},
	{
		Name:        "Nextcloud + Postgres",
		Description: "Self-hosted file storage/sync backed by Postgres",
		Services: []StackService{
			{
				Name: "db",
				Config: &ServiceConfig{
					Image: "postgres:15-alpine",
					Environment: []string{
						"POSTGRES_DB=nextcloud",
						"POSTGRES_USER=nextcloud",
						"POSTGRES_PASSWORD=nextcloud",
					},
					Volumes: []string{"nextcloud_db_data:/var/lib/postgresql/data"},
					HealthCheck: &HealthCheck{
						Test:     []string{"CMD-SHELL", "pg_isready -U nextcloud"},
						Interval: "10s",
						Timeout:  "5s",
						Retries:  5,
					},
					Restart: "unless-stopped",
				},
			},
			{
				Name: "app",
				Config: &ServiceConfig{
					Image: "nextcloud:latest",
					Ports: []string{"8080:80"},
					Environment: []string{
						"POSTGRES_HOST={{db}}",
						"POSTGRES_DB=nextcloud",
						"POSTGRES_USER=nextcloud",
						"POSTGRES_PASSWORD=nextcloud",
					},
					Volumes:   []string{"nextcloud_data:/var/www/html"},
					DependsOn: []DependsOnEntry{{Service: "db", Condition: CondServiceHealthy}},
					Restart:   "unless-stopped",
				},
			},
		},
	},
	{
		Name:        "Nextcloud + Redis + MariaDB",
		Description: "Nextcloud with MariaDB for storage and Redis for caching/locking",
		Services: []StackService{
			{
				Name: "db",
				Config: &ServiceConfig{
					Image: "mariadb:11",
					Environment: []string{
						"MYSQL_ROOT_PASSWORD=nextcloud",
						"MYSQL_DATABASE=nextcloud",
						"MYSQL_USER=nextcloud",
						"MYSQL_PASSWORD=nextcloud",
					},
					Volumes: []string{"nextcloud_db_data:/var/lib/mysql"},
					HealthCheck: &HealthCheck{
						Test:     []string{"CMD", "mysqladmin", "ping", "-h", "localhost"},
						Interval: "10s",
						Timeout:  "5s",
						Retries:  5,
					},
					Restart: "unless-stopped",
				},
			},
			{
				Name: "cache",
				Config: &ServiceConfig{
					Image:   "redis:7-alpine",
					Volumes: []string{"nextcloud_redis_data:/data"},
					HealthCheck: &HealthCheck{
						Test:     []string{"CMD", "redis-cli", "ping"},
						Interval: "5s",
						Timeout:  "3s",
						Retries:  3,
					},
					Restart: "unless-stopped",
				},
			},
			{
				Name: "app",
				Config: &ServiceConfig{
					Image: "nextcloud:latest",
					Ports: []string{"8080:80"},
					Environment: []string{
						"MYSQL_HOST={{db}}",
						"MYSQL_DATABASE=nextcloud",
						"MYSQL_USER=nextcloud",
						"MYSQL_PASSWORD=nextcloud",
						"REDIS_HOST={{cache}}",
					},
					Volumes: []string{"nextcloud_data:/var/www/html"},
					DependsOn: []DependsOnEntry{
						{Service: "db", Condition: CondServiceHealthy},
						{Service: "cache", Condition: CondServiceHealthy},
					},
					Restart: "unless-stopped",
				},
			},
		},
	},
	{
		Name:        "Gitea + Postgres",
		Description: "Self-hosted Git server backed by Postgres",
		Services: []StackService{
			{
				Name: "db",
				Config: &ServiceConfig{
					Image: "postgres:15-alpine",
					Environment: []string{
						"POSTGRES_DB=gitea",
						"POSTGRES_USER=gitea",
						"POSTGRES_PASSWORD=gitea",
					},
					Volumes: []string{"gitea_db_data:/var/lib/postgresql/data"},
					HealthCheck: &HealthCheck{
						Test:     []string{"CMD-SHELL", "pg_isready -U gitea"},
						Interval: "10s",
						Timeout:  "5s",
						Retries:  5,
					},
					Restart: "unless-stopped",
				},
			},
			{
				Name: "gitea",
				Config: &ServiceConfig{
					Image: "gitea/gitea:latest",
					Ports: []string{"3000:3000", "2222:22"},
					Environment: []string{
						"GITEA__database__DB_TYPE=postgres",
						"GITEA__database__HOST={{db}}:5432",
						"GITEA__database__NAME=gitea",
						"GITEA__database__USER=gitea",
						"GITEA__database__PASSWD=gitea",
					},
					Volumes:   []string{"gitea_data:/data"},
					DependsOn: []DependsOnEntry{{Service: "db", Condition: CondServiceHealthy}},
					Restart:   "unless-stopped",
				},
			},
		},
	},
	{
		Name:        "Prometheus + Grafana",
		Description: "Metrics collection with Prometheus and dashboards in Grafana",
		Services: []StackService{
			{
				Name: "prometheus",
				Config: &ServiceConfig{
					Image:   "prom/prometheus:latest",
					Ports:   []string{"9090:9090"},
					Volumes: []string{"prometheus_data:/prometheus"},
					Restart: "unless-stopped",
				},
			},
			{
				Name: "grafana",
				Config: &ServiceConfig{
					Image: "grafana/grafana:latest",
					Ports: []string{"3000:3000"},
					Environment: []string{
						"GF_SECURITY_ADMIN_PASSWORD=admin",
					},
					Volumes:   []string{"grafana_data:/var/lib/grafana"},
					DependsOn: []DependsOnEntry{{Service: "prometheus", Condition: CondServiceStarted}},
					Restart:   "unless-stopped",
				},
			},
		},
	},
	{
		Name:        "Elasticsearch + Logstash + Kibana",
		Description: "The classic ELK stack for log collection and search",
		Services: []StackService{
			{
				Name: "elasticsearch",
				Config: &ServiceConfig{
					Image: "docker.elastic.co/elasticsearch/elasticsearch:8.15.0",
					Environment: []string{
						"discovery.type=single-node",
						"xpack.security.enabled=false",
					},
					Ports:   []string{"9200:9200"},
					Volumes: []string{"es_data:/usr/share/elasticsearch/data"},
					Restart: "unless-stopped",
				},
			},
			{
				Name: "logstash",
				Config: &ServiceConfig{
					Image:     "docker.elastic.co/logstash/logstash:8.15.0",
					Ports:     []string{"5044:5044"},
					DependsOn: []DependsOnEntry{{Service: "elasticsearch", Condition: CondServiceStarted}},
					Restart:   "unless-stopped",
				},
			},
			{
				Name: "kibana",
				Config: &ServiceConfig{
					Image: "docker.elastic.co/kibana/kibana:8.15.0",
					Ports: []string{"5601:5601"},
					Environment: []string{
						"ELASTICSEARCH_HOSTS=http://{{elasticsearch}}:9200",
					},
					DependsOn: []DependsOnEntry{{Service: "elasticsearch", Condition: CondServiceStarted}},
					Restart:   "unless-stopped",
				},
			},
		},
	},
	{
		Name:        "Redis + RedisInsight",
		Description: "Redis with a web UI to browse keys and run commands",
		Services: []StackService{
			{
				Name: "cache",
				Config: &ServiceConfig{
					Image:   "redis:7-alpine",
					Volumes: []string{"redis_data:/data"},
					HealthCheck: &HealthCheck{
						Test:     []string{"CMD", "redis-cli", "ping"},
						Interval: "5s",
						Timeout:  "3s",
						Retries:  3,
					},
					Restart: "unless-stopped",
				},
			},
			{
				Name: "redisinsight",
				Config: &ServiceConfig{
					Image:     "redis/redisinsight:latest",
					Ports:     []string{"5540:5540"},
					Volumes:   []string{"redisinsight_data:/data"},
					DependsOn: []DependsOnEntry{{Service: "cache", Condition: CondServiceHealthy}},
					Restart:   "unless-stopped",
				},
			},
		},
	},
	{
		Name:        "Ghost + MySQL",
		Description: "Ghost blogging platform backed by its own MySQL database",
		Services: []StackService{
			{
				Name: "db",
				Config: &ServiceConfig{
					Image: "mysql:8",
					Environment: []string{
						"MYSQL_ROOT_PASSWORD=ghost",
						"MYSQL_DATABASE=ghost",
						"MYSQL_USER=ghost",
						"MYSQL_PASSWORD=ghost",
					},
					Volumes: []string{"ghost_db_data:/var/lib/mysql"},
					HealthCheck: &HealthCheck{
						Test:     []string{"CMD", "mysqladmin", "ping", "-h", "localhost"},
						Interval: "10s",
						Timeout:  "5s",
						Retries:  5,
					},
					Restart: "unless-stopped",
				},
			},
			{
				Name: "ghost",
				Config: &ServiceConfig{
					Image: "ghost:5-alpine",
					Ports: []string{"2368:2368"},
					Environment: []string{
						"database__client=mysql",
						"database__connection__host={{db}}",
						"database__connection__user=ghost",
						"database__connection__password=ghost",
						"database__connection__database=ghost",
					},
					Volumes:   []string{"ghost_data:/var/lib/ghost/content"},
					DependsOn: []DependsOnEntry{{Service: "db", Condition: CondServiceHealthy}},
					Restart:   "unless-stopped",
				},
			},
		},
	},
	{
		Name:        "Metabase + Postgres",
		Description: "Self-hosted BI/analytics dashboards over a Postgres warehouse",
		Services: []StackService{
			{
				Name: "db",
				Config: &ServiceConfig{
					Image: "postgres:15-alpine",
					Environment: []string{
						"POSTGRES_DB=metabase",
						"POSTGRES_USER=metabase",
						"POSTGRES_PASSWORD=metabase",
					},
					Volumes: []string{"metabase_db_data:/var/lib/postgresql/data"},
					HealthCheck: &HealthCheck{
						Test:     []string{"CMD-SHELL", "pg_isready -U metabase"},
						Interval: "10s",
						Timeout:  "5s",
						Retries:  5,
					},
					Restart: "unless-stopped",
				},
			},
			{
				Name: "metabase",
				Config: &ServiceConfig{
					Image: "metabase/metabase:latest",
					Ports: []string{"3000:3000"},
					Environment: []string{
						"MB_DB_TYPE=postgres",
						"MB_DB_DBNAME=metabase",
						"MB_DB_PORT=5432",
						"MB_DB_USER=metabase",
						"MB_DB_PASS=metabase",
						"MB_DB_HOST={{db}}",
					},
					DependsOn: []DependsOnEntry{{Service: "db", Condition: CondServiceHealthy}},
					Restart:   "unless-stopped",
				},
			},
		},
	},
	{
		Name:        "n8n + Postgres",
		Description: "Self-hosted workflow automation backed by Postgres",
		Services: []StackService{
			{
				Name: "db",
				Config: &ServiceConfig{
					Image: "postgres:15-alpine",
					Environment: []string{
						"POSTGRES_DB=n8n",
						"POSTGRES_USER=n8n",
						"POSTGRES_PASSWORD=n8n",
					},
					Volumes: []string{"n8n_db_data:/var/lib/postgresql/data"},
					HealthCheck: &HealthCheck{
						Test:     []string{"CMD-SHELL", "pg_isready -U n8n"},
						Interval: "10s",
						Timeout:  "5s",
						Retries:  5,
					},
					Restart: "unless-stopped",
				},
			},
			{
				Name: "n8n",
				Config: &ServiceConfig{
					Image: "n8nio/n8n:latest",
					Ports: []string{"5678:5678"},
					Environment: []string{
						"DB_TYPE=postgresdb",
						"DB_POSTGRESDB_HOST={{db}}",
						"DB_POSTGRESDB_DATABASE=n8n",
						"DB_POSTGRESDB_USER=n8n",
						"DB_POSTGRESDB_PASSWORD=n8n",
					},
					Volumes:   []string{"n8n_data:/home/node/.n8n"},
					DependsOn: []DependsOnEntry{{Service: "db", Condition: CondServiceHealthy}},
					Restart:   "unless-stopped",
				},
			},
		},
	},
	{
		Name:        "Directus + Postgres",
		Description: "Self-hosted headless CMS / data platform backed by Postgres",
		Services: []StackService{
			{
				Name: "db",
				Config: &ServiceConfig{
					Image: "postgres:15-alpine",
					Environment: []string{
						"POSTGRES_DB=directus",
						"POSTGRES_USER=directus",
						"POSTGRES_PASSWORD=directus",
					},
					Volumes: []string{"directus_db_data:/var/lib/postgresql/data"},
					HealthCheck: &HealthCheck{
						Test:     []string{"CMD-SHELL", "pg_isready -U directus"},
						Interval: "10s",
						Timeout:  "5s",
						Retries:  5,
					},
					Restart: "unless-stopped",
				},
			},
			{
				Name: "directus",
				Config: &ServiceConfig{
					Image: "directus/directus:latest",
					Ports: []string{"8055:8055"},
					Environment: []string{
						"KEY=replace-with-a-random-value",
						"SECRET=replace-with-a-random-value",
						"DB_CLIENT=pg",
						"DB_HOST={{db}}",
						"DB_PORT=5432",
						"DB_DATABASE=directus",
						"DB_USER=directus",
						"DB_PASSWORD=directus",
						"ADMIN_EMAIL=admin@admin.com",
						"ADMIN_PASSWORD=admin",
					},
					Volumes:   []string{"directus_uploads:/directus/uploads"},
					DependsOn: []DependsOnEntry{{Service: "db", Condition: CondServiceHealthy}},
					Restart:   "unless-stopped",
				},
			},
		},
	},
	{
		Name:        "Adminer + Postgres",
		Description: "Postgres database with Adminer, a lightweight single-file DB admin UI",
		Services: []StackService{
			{
				Name: "db",
				Config: &ServiceConfig{
					Image: "postgres:15-alpine",
					Environment: []string{
						"POSTGRES_USER=postgres",
						"POSTGRES_PASSWORD=postgres",
						"POSTGRES_DB=app",
					},
					Volumes: []string{"postgres_data:/var/lib/postgresql/data"},
					HealthCheck: &HealthCheck{
						Test:     []string{"CMD-SHELL", "pg_isready -U postgres"},
						Interval: "10s",
						Timeout:  "5s",
						Retries:  5,
					},
					Restart: "unless-stopped",
				},
			},
			{
				Name: "adminer",
				Config: &ServiceConfig{
					Image: "adminer:latest",
					Ports: []string{"8083:8080"},
					Environment: []string{
						"ADMINER_DEFAULT_SERVER={{db}}",
					},
					DependsOn: []DependsOnEntry{{Service: "db", Condition: CondServiceHealthy}},
					Restart:   "unless-stopped",
				},
			},
		},
	},
	{
		Name:        "Portainer + Watchtower",
		Description: "Docker management UI plus automatic container updates",
		Services: []StackService{
			{
				Name: "portainer",
				Config: &ServiceConfig{
					Image: "portainer/portainer-ce:latest",
					Ports: []string{"9443:9443"},
					Volumes: []string{
						"/var/run/docker.sock:/var/run/docker.sock",
						"portainer_data:/data",
					},
					Restart: "unless-stopped",
				},
			},
			{
				Name: "watchtower",
				Config: &ServiceConfig{
					Image: "containrrr/watchtower:latest",
					Volumes: []string{
						"/var/run/docker.sock:/var/run/docker.sock",
					},
					Restart: "unless-stopped",
				},
			},
		},
	},
}

// AddStack adds every service in a stack to the config in one step,
// starting from each service's suggested name and uniquifying it
// against whatever's already there — a stack can be applied more than
// once, or alongside services that happen to already use the same
// name.
//
// Two things need fixing up afterward, both because a name can change:
//
//  1. depends_on entries reference another service by its stack-local
//     name (e.g. "db") — those get re-pointed at whatever name that
//     service actually ended up with.
//  2. Environment values that need to reference another service as a
//     hostname (e.g. WORDPRESS_DB_HOST) can't just hardcode "db" for
//     the same reason — see the {{name}} placeholder convention used
//     in the Stacks definitions below. Every "{{localname}}" in an
//     environment value gets substituted with the final name here.
//
// It returns the final names actually used, in the same order as
// stack.Services.
func (c *ComposeConfig) AddStack(stack Stack) []string {
	finalNames := make([]string, len(stack.Services))
	nameMap := make(map[string]string, len(stack.Services))
	for i, s := range stack.Services {
		finalNames[i] = c.uniqueServiceName(s.Name)
		nameMap[s.Name] = finalNames[i]
	}

	for i, s := range stack.Services {
		svc := c.AddService(finalNames[i])

		// A plain `*svc = *s.Config` copies the struct's slice fields
		// as shared headers pointing at the SAME backing arrays as the
		// static Stacks definition below. Mutating any element of
		// those slices afterward (the DependsOn rewrite two lines down,
		// for instance) would silently corrupt the shared template for
		// every future use of this stack in the same program run.
		// Copying each slice into a fresh one keeps this call's changes
		// local to the service that was actually just added.
		cfg := *s.Config
		cfg.Ports = append([]string(nil), s.Config.Ports...)
		cfg.Environment = append([]string(nil), s.Config.Environment...)
		cfg.Volumes = append([]string(nil), s.Config.Volumes...)
		cfg.DependsOn = append([]DependsOnEntry(nil), s.Config.DependsOn...)
		if s.Config.HealthCheck != nil {
			hc := *s.Config.HealthCheck
			hc.Test = append([]string(nil), s.Config.HealthCheck.Test...)
			cfg.HealthCheck = &hc
		}

		for j, dep := range cfg.DependsOn {
			if mapped, ok := nameMap[dep.Service]; ok {
				cfg.DependsOn[j].Service = mapped
			}
		}

		for j, env := range cfg.Environment {
			for local, final := range nameMap {
				env = strings.ReplaceAll(env, "{{"+local+"}}", final)
			}
			cfg.Environment[j] = env
		}

		*svc = cfg
	}

	return finalNames
}

// uniqueServiceName returns name unchanged if nothing in the config
// already uses it, otherwise appends -2, -3, ... until it finds one
// that's free.
func (c *ComposeConfig) uniqueServiceName(name string) string {
	if c.GetService(name) == nil {
		return name
	}
	for i := 2; ; i++ {
		candidate := fmt.Sprintf("%s-%d", name, i)
		if c.GetService(candidate) == nil {
			return candidate
		}
	}
}
