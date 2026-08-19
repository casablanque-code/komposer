package composer

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
			Image:   "postgres:15-alpine",
			Ports:   []string{"5432:5432"},
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
