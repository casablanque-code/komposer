package composer

import "fmt"

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
						"WORDPRESS_DB_HOST=db:3306",
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
						"ME_CONFIG_MONGODB_SERVER=db",
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
						"PMA_HOST=db",
					},
					DependsOn: []DependsOnEntry{{Service: "db", Condition: CondServiceHealthy}},
					Restart:   "unless-stopped",
				},
			},
		},
	},
}

// AddStack adds every service in a stack to the config in one step,
// starting from each service's suggested name and uniquifying it
// against whatever's already there — a stack can be applied more than
// once, or alongside services that happen to already use the same
// name. depends_on entries inside the stack get re-pointed at whatever
// name each dependency actually ended up with, so "db" still resolves
// correctly even if it had to become "db-2".
//
// It returns the final names actually used, in the same order as
// stack.Services.
func (c *ComposeConfig) AddStack(stack Stack) []string {
	finalNames := make([]string, len(stack.Services))
	nameMap := make(map[string]string, len(stack.Services))

	for i, s := range stack.Services {
		finalName := c.uniqueServiceName(s.Name)
		nameMap[s.Name] = finalName
		finalNames[i] = finalName

		svc := c.AddService(finalName)
		*svc = *s.Config
	}

	for _, finalName := range finalNames {
		svc := c.GetService(finalName)
		if svc == nil {
			continue
		}
		for i, dep := range svc.DependsOn {
			if mapped, ok := nameMap[dep.Service]; ok {
				svc.DependsOn[i].Service = mapped
			}
		}
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
